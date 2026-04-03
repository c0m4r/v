package engine

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ImageInfo holds metadata about a cached image.
type ImageInfo struct {
	Name string
	Size int64
	Path string
}

// PullImage downloads a cloud image by name or URL into the image cache.
// progress receives bytes written so far; pass nil to ignore.
func (e *Engine) PullImage(nameOrURL string, progress func(int64)) (string, error) {
	url, fileName := resolveImage(nameOrURL, e.KnownImages())
	destPath := filepath.Join(e.ImageDir, fileName)

	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil // already cached
	}

	resp, err := http.Get(url)
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
