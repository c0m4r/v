package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsISO(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"alpine-virt-3.23.3-x86_64.iso", true},
		{"ubuntu.ISO", true},
		{"mixed.Iso", true},
		{"disk.qcow2", false},
		{"image.img", false},
		{"no-extension", false},
		{"tricky.iso.qcow2", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsISO(tt.name); got != tt.want {
			t.Errorf("IsISO(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestResolveImage_KnownName(t *testing.T) {
	known := map[string]string{
		"ubuntu-24.04": "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
	}
	url, fileName := resolveImage("ubuntu-24.04", known)
	if url != known["ubuntu-24.04"] {
		t.Errorf("url: got %q, want %q", url, known["ubuntu-24.04"])
	}
	if fileName != "noble-server-cloudimg-amd64.img" {
		t.Errorf("fileName: got %q, want noble-server-cloudimg-amd64.img", fileName)
	}
}

func TestResolveImage_URL(t *testing.T) {
	rawURL := "https://example.com/path/to/custom.qcow2"
	url, fileName := resolveImage(rawURL, nil)
	if url != rawURL {
		t.Errorf("url: got %q, want %q", url, rawURL)
	}
	if fileName != "custom.qcow2" {
		t.Errorf("fileName: got %q, want custom.qcow2", fileName)
	}
}

func TestResolveImage_HTTPUrl(t *testing.T) {
	rawURL := "http://example.com/image.img"
	url, fileName := resolveImage(rawURL, nil)
	if url != rawURL {
		t.Errorf("url: got %q, want %q", url, rawURL)
	}
	if fileName != "image.img" {
		t.Errorf("fileName: got %q, want image.img", fileName)
	}
}

func TestResolveImage_Unknown(t *testing.T) {
	url, fileName := resolveImage("some-unknown", map[string]string{})
	if url != "some-unknown" {
		t.Errorf("url: got %q, want some-unknown", url)
	}
	if fileName != "some-unknown" {
		t.Errorf("fileName: got %q, want some-unknown", fileName)
	}
}

func TestListImages_Empty(t *testing.T) {
	e := testEngine(t)
	images, err := e.ListImages()
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(images) != 0 {
		t.Errorf("expected 0 images, got %d", len(images))
	}
}

func TestListImages(t *testing.T) {
	e := testEngine(t)

	// Create some fake image files
	_ = os.WriteFile(filepath.Join(e.ImageDir, "ubuntu.img"), []byte("fake"), 0644)
	_ = os.WriteFile(filepath.Join(e.ImageDir, "debian.qcow2"), []byte("fake-image"), 0644)
	// Hidden files should be skipped
	_ = os.WriteFile(filepath.Join(e.ImageDir, ".download-tmp"), []byte("x"), 0644)
	// Directories should be skipped
	_ = os.MkdirAll(filepath.Join(e.ImageDir, "subdir"), 0750)

	images, err := e.ListImages()
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}

	names := map[string]bool{}
	for _, img := range images {
		names[img.Name] = true
		if img.Size == 0 {
			t.Errorf("image %q has 0 size", img.Name)
		}
		if !strings.Contains(img.Path, e.ImageDir) {
			t.Errorf("image path %q doesn't contain image dir", img.Path)
		}
	}
	if !names["ubuntu.img"] || !names["debian.qcow2"] {
		t.Errorf("unexpected image names: %v", names)
	}
}

func TestListImages_NoDir(t *testing.T) {
	e := &Engine{
		DataDir:  "/nonexistent-path-for-test",
		VMDir:    "/nonexistent-path-for-test/vms",
		ImageDir: "/nonexistent-path-for-test/images",
	}
	images, err := e.ListImages()
	if err != nil {
		t.Fatalf("ListImages on nonexistent dir should not error, got: %v", err)
	}
	if images != nil {
		t.Errorf("expected nil, got %v", images)
	}
}
