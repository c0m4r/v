package engine

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

// validImageFilename restricts cached image filenames to a safe character class
// so static analyzers can prove the join with ImageDir cannot escape.
var validImageFilename = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,254}$`)

// validImageURL restricts download URLs to a strict http(s) shape so static
// analyzers can prove no scheme or host smuggling.
var validImageURL = regexp.MustCompile(`^https?://[a-zA-Z0-9][a-zA-Z0-9.-]{0,253}(:[0-9]{1,5})?(/[a-zA-Z0-9._~/-]*)?$`)

// invalidNameChars matches every run of characters that validImageFilename
// rejects, so a source file like "Windows 11 (x64).iso" can still be mapped
// onto a usable cache name.
var invalidNameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// importableExts lists the extensions ImportImage accepts. Anything else is
// almost certainly not bootable media, and rejecting it here gives a better
// error than QEMU would at start time.
var importableExts = map[string]bool{
	".iso":   true,
	".img":   true,
	".qcow2": true,
	".raw":   true,
}

// ImageInfo holds metadata about a cached image.
type ImageInfo struct {
	Name string
	Size int64
	Path string
	// Link is the target of an imported image's symlink, empty for images
	// that physically live in the cache. Broken is set when that target is
	// no longer readable.
	Link   string `json:",omitempty"`
	Broken bool   `json:",omitempty"`
}

// maxImageBytes caps image downloads at 16 GiB.
const maxImageBytes = 16 << 30

var privateRanges []*net.IPNet

func init() {
	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
		"100.64.0.0/10",
	} {
		_, block, _ := net.ParseCIDR(cidr)
		if block != nil {
			privateRanges = append(privateRanges, block)
		}
	}
}

func isPrivateIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	for _, block := range privateRanges {
		if block.Contains(ip) {
			return true
		}
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

// safePullClient returns an HTTP client that blocks private/loopback addresses
// and caps response size, preventing SSRF when downloading images.
func safePullClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Minute,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, fmt.Errorf("invalid address %q: %w", addr, err)
				}
				ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, fmt.Errorf("resolve %s: %w", host, err)
				}
				if len(ips) == 0 {
					return nil, fmt.Errorf("no addresses found for %s", host)
				}
				for _, ipAddr := range ips {
					if isPrivateIP(ipAddr.IP) {
						return nil, fmt.Errorf("download blocked: %s resolves to a private/loopback address (%s)", host, ipAddr.IP)
					}
				}
				// Dial by resolved IP to prevent TOCTOU DNS rebinding.
				return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
			},
		},
	}
}

// countingReader wraps an io.Reader and tracks how many bytes have been read.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// progressInterval is the minimum delay between progress callback invocations
// during a download, to keep clients (CLI rendering, web SSE-style streams)
// from being overwhelmed.
const progressInterval = 200 * time.Millisecond

// PullImage downloads a cloud image by name or URL into the image cache.
// progress, if non-nil, is invoked periodically with (bytesSoFar, totalBytes).
// For .xz URLs, both values measure HTTP-level (compressed) bytes against
// Content-Length, so percentage math stays meaningful even though the file on
// disk is the decompressed payload. totalBytes is 0 when unknown.
func (e *Engine) PullImage(nameOrURL string, progress func(bytes, total int64)) (string, error) {
	rawURL, rawFile, err := resolveImage(nameOrURL, e.KnownImages())
	if err != nil {
		return "", err
	}
	fileName := filepath.Base(rawFile)
	compressed := strings.HasSuffix(strings.ToLower(fileName), ".xz")
	storedName := strings.TrimSuffix(fileName, ".xz")
	if !validImageFilename.MatchString(storedName) {
		return "", fmt.Errorf("invalid image filename derived from %q", nameOrURL)
	}
	if !validImageURL.MatchString(rawURL) {
		return "", fmt.Errorf("invalid image URL %q", rawURL)
	}
	destPath := filepath.Join(e.ImageDir, storedName)

	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil // already cached
	}

	client := safePullClient()
	resp, err := client.Get(rawURL)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download image: HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(e.ImageDir, ".download-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // clean up on failure
	}()

	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	counter := &countingReader{r: resp.Body}
	var src io.Reader = counter
	if compressed {
		xzr, xerr := xz.NewReader(counter)
		if xerr != nil {
			return "", fmt.Errorf("decompress xz: %w", xerr)
		}
		src = xzr
	}

	lastTick := time.Now()
	var written int64
	buf := make([]byte, 256*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := tmp.Write(buf[:n]); err != nil {
				return "", fmt.Errorf("write image: %w", err)
			}
			written += int64(n)
			if written > maxImageBytes {
				return "", fmt.Errorf("image exceeds maximum allowed size (%d GiB)", maxImageBytes>>30)
			}
			if progress != nil && time.Since(lastTick) >= progressInterval {
				progress(counter.n, total)
				lastTick = time.Now()
			}
		}
		if readErr == io.EOF {
			if progress != nil {
				progress(counter.n, total)
			}
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("read image: %w", readErr)
		}
	}

	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, destPath); err != nil {
		return "", fmt.Errorf("move image: %w", err)
	}

	return destPath, nil
}

// expandHome expands a leading ~/ to the home directory of the user running
// v. Applied to import paths so the CLI and the web UI accept the same input.
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path[1:], "/"))
		}
	}
	return path
}

// sanitizeImageName maps an arbitrary basename onto the character class
// accepted by validImageFilename, keeping the extension intact so IsISO and
// friends still recognise the media type.
func sanitizeImageName(name string) string {
	name = invalidNameChars.ReplaceAllString(name, "-")
	name = strings.TrimLeft(name, "._-")
	const maxLen = 255
	if len(name) > maxLen {
		ext := filepath.Ext(name)
		name = strings.TrimSuffix(name, ext)[:maxLen-len(ext)] + ext
	}
	return name
}

// ImportImage registers a file that already exists on this host as a cached
// image, by symlinking it into ImageDir under name (defaulting to the
// source's filename). The file itself is not copied: it stays where it is,
// so moving or deleting it later breaks every VM created from it.
//
// It returns the path of the cache entry.
func (e *Engine) ImportImage(srcPath, name string) (string, error) {
	if strings.TrimSpace(srcPath) == "" {
		return "", fmt.Errorf("source path is required")
	}

	// Resolve symlinks in the source so the cache entry points at the real
	// file even if the user's own link is later repointed. This also proves
	// the source exists.
	resolved, err := filepath.EvalSymlinks(expandHome(srcPath))
	if err != nil {
		return "", fmt.Errorf("read %q: %w", srcPath, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", srcPath, err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", srcPath, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%q is not a regular file", srcPath)
	}
	// Fail now rather than at boot if QEMU wouldn't be able to open it.
	f, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", srcPath, err)
	}
	_ = f.Close()

	// A file that already lives in the cache needs no entry of its own.
	// ImageDir is resolved too, so a data dir reached through a symlink
	// (/tmp on some systems) still compares equal.
	if name == "" {
		if imageDir, derr := filepath.EvalSymlinks(e.ImageDir); derr == nil && filepath.Dir(resolved) == imageDir {
			return resolved, nil
		}
	}

	if name == "" {
		name = filepath.Base(resolved)
	}
	storedName := sanitizeImageName(filepath.Base(name))
	if !validImageFilename.MatchString(storedName) {
		return "", fmt.Errorf("cannot derive a valid image name from %q; pass an explicit name", name)
	}
	if ext := strings.ToLower(filepath.Ext(storedName)); !importableExts[ext] {
		return "", fmt.Errorf("unsupported image type %q: expected .iso, .img, .qcow2, or .raw", ext)
	}

	destPath := filepath.Join(e.ImageDir, storedName)
	if existing, err := os.Readlink(destPath); err == nil {
		if existing == resolved {
			return destPath, nil // already imported, same target
		}
		return "", fmt.Errorf("image %q is already in the cache (linked to %s); import it under a different name", storedName, existing)
	}
	if _, err := os.Lstat(destPath); err == nil {
		return "", fmt.Errorf("image %q is already in the cache; import it under a different name", storedName)
	}

	if err := os.Symlink(resolved, destPath); err != nil {
		return "", fmt.Errorf("link image into cache: %w", err)
	}
	return destPath, nil
}

// ListImages returns all cached images. Imported images are symlinks, so
// their size is read from the link target and reported as Broken when that
// target has gone away.
func (e *Engine) ListImages() ([]ImageInfo, error) {
	entries, err := os.ReadDir(e.ImageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var images []ImageInfo
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		imgPath := filepath.Join(e.ImageDir, entry.Name())

		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			link, _ = os.Readlink(imgPath)
			target, terr := os.Stat(imgPath)
			if terr != nil {
				// Still list it: a broken import is easier to fix when visible.
				images = append(images, ImageInfo{Name: entry.Name(), Path: imgPath, Link: link, Broken: true})
				continue
			}
			if !target.Mode().IsRegular() {
				continue
			}
			info = target
		}

		images = append(images, ImageInfo{
			Name: entry.Name(),
			Size: info.Size(),
			Path: imgPath,
			Link: link,
		})
	}
	return images, nil
}

// IsISO returns true if the filename has an .iso extension.
func IsISO(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".iso")
}

func resolveImage(nameOrURL string, knownImages map[string]string) (rawURL, fileName string, err error) {
	candidate := nameOrURL
	if u, ok := knownImages[nameOrURL]; ok {
		candidate = u
	}

	parsed, perr := url.Parse(candidate)
	if perr != nil {
		return "", "", fmt.Errorf("invalid image URL %q: %w", candidate, perr)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("image URL must use http or https, got %q", candidate)
	}
	if parsed.Host == "" {
		return "", "", fmt.Errorf("image URL %q has no host", candidate)
	}
	return parsed.String(), path.Base(parsed.Path), nil
}
