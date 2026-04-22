package engine

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImageInfo holds metadata about a cached image.
type ImageInfo struct {
	Name string
	Size int64
	Path string
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

// PullImage downloads a cloud image by name or URL into the image cache.
// progress receives bytes written so far; pass nil to ignore.
func (e *Engine) PullImage(nameOrURL string, progress func(int64)) (string, error) {
	url, fileName := resolveImage(nameOrURL, e.KnownImages())
	destPath := filepath.Join(e.ImageDir, fileName)

	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil // already cached
	}

	client := safePullClient()
	resp, err := client.Get(url)
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

	var written int64
	buf := make([]byte, 256*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := tmp.Write(buf[:n]); err != nil {
				return "", fmt.Errorf("write image: %w", err)
			}
			written += int64(n)
			if written > maxImageBytes {
				return "", fmt.Errorf("image exceeds maximum allowed size (%d GiB)", maxImageBytes>>30)
			}
			if progress != nil {
				progress(written)
			}
		}
		if readErr == io.EOF {
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

// ListImages returns all cached images.
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
		images = append(images, ImageInfo{
			Name: entry.Name(),
			Size: info.Size(),
			Path: filepath.Join(e.ImageDir, entry.Name()),
		})
	}
	return images, nil
}

// IsISO returns true if the filename has an .iso extension.
func IsISO(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".iso")
}

func resolveImage(nameOrURL string, knownImages map[string]string) (url, fileName string) {
	if strings.HasPrefix(nameOrURL, "http://") || strings.HasPrefix(nameOrURL, "https://") {
		parts := strings.Split(nameOrURL, "/")
		return nameOrURL, parts[len(parts)-1]
	}

	if u, ok := knownImages[nameOrURL]; ok {
		parts := strings.Split(u, "/")
		return u, parts[len(parts)-1]
	}

	// Treat as a URL anyway
	return nameOrURL, nameOrURL
}
