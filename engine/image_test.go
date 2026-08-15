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
	url, fileName, err := resolveImage("ubuntu-24.04", known)
	if err != nil {
		t.Fatalf("resolveImage: %v", err)
	}
	if url != known["ubuntu-24.04"] {
		t.Errorf("url: got %q, want %q", url, known["ubuntu-24.04"])
	}
	if fileName != "noble-server-cloudimg-amd64.img" {
		t.Errorf("fileName: got %q, want noble-server-cloudimg-amd64.img", fileName)
	}
}

func TestResolveImage_URL(t *testing.T) {
	rawURL := "https://example.com/path/to/custom.qcow2"
	url, fileName, err := resolveImage(rawURL, nil)
	if err != nil {
		t.Fatalf("resolveImage: %v", err)
	}
	if url != rawURL {
		t.Errorf("url: got %q, want %q", url, rawURL)
	}
	if fileName != "custom.qcow2" {
		t.Errorf("fileName: got %q, want custom.qcow2", fileName)
	}
}

func TestResolveImage_HTTPUrl(t *testing.T) {
	rawURL := "http://example.com/image.img"
	url, fileName, err := resolveImage(rawURL, nil)
	if err != nil {
		t.Fatalf("resolveImage: %v", err)
	}
	if url != rawURL {
		t.Errorf("url: got %q, want %q", url, rawURL)
	}
	if fileName != "image.img" {
		t.Errorf("fileName: got %q, want image.img", fileName)
	}
}

func TestResolveImage_Unknown(t *testing.T) {
	if _, _, err := resolveImage("some-unknown", map[string]string{}); err == nil {
		t.Error("expected error for unknown name with no scheme, got nil")
	}
}

func TestResolveImage_BadScheme(t *testing.T) {
	if _, _, err := resolveImage("file:///etc/passwd", nil); err == nil {
		t.Error("expected error for file:// scheme, got nil")
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

func TestSanitizeImageName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"alpine.iso", "alpine.iso"},
		{"Windows 11 (x64).iso", "Windows-11-x64-.iso"},
		{"...leading.iso", "leading.iso"},
		{"a/b", "a-b"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := sanitizeImageName(tt.in); got != tt.want {
			t.Errorf("sanitizeImageName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	long := strings.Repeat("x", 300) + ".iso"
	if got := sanitizeImageName(long); len(got) != 255 || !strings.HasSuffix(got, ".iso") {
		t.Errorf("sanitizeImageName(long) = %d chars, suffix kept=%v; want 255 chars ending in .iso",
			len(got), strings.HasSuffix(got, ".iso"))
	}
}

// writeISO creates a fake ISO outside the image cache and returns its fully
// resolved path, so tests can compare it against link targets directly.
func writeISO(t *testing.T, name string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake-iso-payload"), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestImportImage(t *testing.T) {
	e := testEngine(t)
	src := writeISO(t, "custom.iso")

	dest, err := e.ImportImage(src, "")
	if err != nil {
		t.Fatalf("ImportImage: %v", err)
	}
	if want := filepath.Join(e.ImageDir, "custom.iso"); dest != want {
		t.Errorf("dest: got %q, want %q", dest, want)
	}

	target, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("cache entry is not a symlink: %v", err)
	}
	if target != src {
		t.Errorf("link target: got %q, want %q", target, src)
	}

	// The original must not have been moved or copied away.
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source file disappeared: %v", err)
	}
}

func TestImportImage_CustomNameAndSanitizing(t *testing.T) {
	e := testEngine(t)
	src := writeISO(t, "plain.iso")

	dest, err := e.ImportImage(src, "My Windows 11.iso")
	if err != nil {
		t.Fatalf("ImportImage: %v", err)
	}
	if got := filepath.Base(dest); got != "My-Windows-11.iso" {
		t.Errorf("name: got %q, want My-Windows-11.iso", got)
	}
	if !validImageFilename.MatchString(filepath.Base(dest)) {
		t.Errorf("imported name %q is not a valid cache filename", filepath.Base(dest))
	}
}

func TestImportImage_Idempotent(t *testing.T) {
	e := testEngine(t)
	src := writeISO(t, "again.iso")

	first, err := e.ImportImage(src, "")
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, err := e.ImportImage(src, "")
	if err != nil {
		t.Fatalf("re-import of the same file should succeed, got: %v", err)
	}
	if first != second {
		t.Errorf("paths differ: %q vs %q", first, second)
	}
}

func TestImportImage_NameCollision(t *testing.T) {
	e := testEngine(t)
	a := writeISO(t, "same.iso")
	b := writeISO(t, "same.iso")

	if _, err := e.ImportImage(a, ""); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if _, err := e.ImportImage(b, ""); err == nil {
		t.Error("expected error importing a different file under an existing name, got nil")
	}
}

func TestImportImage_AlreadyInCache(t *testing.T) {
	e := testEngine(t)
	cached := filepath.Join(e.ImageDir, "pulled.iso")
	if err := os.WriteFile(cached, []byte("fake"), 0644); err != nil {
		t.Fatalf("write cached image: %v", err)
	}

	dest, err := e.ImportImage(cached, "")
	if err != nil {
		t.Fatalf("ImportImage: %v", err)
	}
	if dest != cached {
		t.Errorf("dest: got %q, want %q", dest, cached)
	}
	info, err := os.Lstat(cached)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("existing cache entry was replaced by a symlink to itself")
	}
}

func TestImportImage_Rejects(t *testing.T) {
	e := testEngine(t)

	tests := []struct {
		name string
		path string
	}{
		{"empty path", ""},
		{"missing file", filepath.Join(t.TempDir(), "nope.iso")},
		{"directory", t.TempDir()},
		{"unsupported extension", writeISO(t, "notes.txt")},
		{"no extension", writeISO(t, "bare")},
	}
	for _, tt := range tests {
		if _, err := e.ImportImage(tt.path, ""); err == nil {
			t.Errorf("%s: expected error, got nil", tt.name)
		}
	}
}

func TestListImages_Imported(t *testing.T) {
	e := testEngine(t)
	src := writeISO(t, "linked.iso")
	if _, err := e.ImportImage(src, ""); err != nil {
		t.Fatalf("ImportImage: %v", err)
	}

	images, err := e.ListImages()
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	img := images[0]
	if img.Link != src {
		t.Errorf("Link: got %q, want %q", img.Link, src)
	}
	if img.Broken {
		t.Error("image reported as broken while the source exists")
	}
	// Size must come from the link target, not the symlink inode.
	if want := int64(len("fake-iso-payload")); img.Size != want {
		t.Errorf("Size: got %d, want %d", img.Size, want)
	}

	// Removing the source must surface as a broken import, not a vanished image.
	if err := os.Remove(src); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	images, err = e.ListImages()
	if err != nil {
		t.Fatalf("ListImages after removal: %v", err)
	}
	if len(images) != 1 || !images[0].Broken {
		t.Errorf("expected 1 broken image, got %+v", images)
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
