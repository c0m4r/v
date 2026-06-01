package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectImageFormat(t *testing.T) {
	dir := t.TempDir()

	qcow2Path := filepath.Join(dir, "img.qcow2")
	if err := os.WriteFile(qcow2Path, append(qcow2Magic, 0, 0, 0, 0), 0o644); err != nil {
		t.Fatalf("write qcow2: %v", err)
	}

	rawPath := filepath.Join(dir, "img.raw")
	if err := os.WriteFile(rawPath, []byte("RANDOMDATA"), 0o644); err != nil {
		t.Fatalf("write raw: %v", err)
	}

	shortPath := filepath.Join(dir, "tiny")
	if err := os.WriteFile(shortPath, []byte("ab"), 0o644); err != nil {
		t.Fatalf("write short: %v", err)
	}

	cases := []struct {
		path string
		want string
	}{
		{qcow2Path, "qcow2"},
		{rawPath, "raw"},
		{shortPath, "raw"},
	}
	for _, c := range cases {
		got, err := detectImageFormat(c.path)
		if err != nil {
			t.Errorf("detectImageFormat(%s): %v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("detectImageFormat(%s) = %q, want %q", c.path, got, c.want)
		}
	}

	if _, err := detectImageFormat(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Error("expected error for missing file")
	}
}
