package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	dir := t.TempDir()
	e, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if e.DataDir != dir {
		t.Errorf("DataDir: got %q, want %q", e.DataDir, dir)
	}
	if e.VMDir != filepath.Join(dir, "vms") {
		t.Errorf("VMDir: got %q", e.VMDir)
	}
	if e.ImageDir != filepath.Join(dir, "images") {
		t.Errorf("ImageDir: got %q", e.ImageDir)
	}

	// Directories should exist
	for _, d := range []string{e.DataDir, e.VMDir, e.ImageDir} {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("directory %q does not exist: %v", d, err)
		} else if !info.IsDir() {
			t.Errorf("%q is not a directory", d)
		}
	}
}

func TestNew_Idempotent(t *testing.T) {
	dir := t.TempDir()
	_, err := New(dir)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	// Second call should succeed (directories already exist)
	_, err = New(dir)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
}

func TestDefaultDataDir(t *testing.T) {
	// With env override
	t.Setenv("V_DATA_DIR", "/tmp/custom-v")
	if got := DefaultDataDir(); got != "/tmp/custom-v" {
		t.Errorf("DefaultDataDir with env: got %q, want /tmp/custom-v", got)
	}

	// Without env override
	t.Setenv("V_DATA_DIR", "")
	dir := DefaultDataDir()
	if dir == "" {
		t.Error("DefaultDataDir returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("DefaultDataDir returned non-absolute path: %q", dir)
	}
}

func TestDefaultDataDir_NoHome(t *testing.T) {
	t.Setenv("V_DATA_DIR", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	// Should fall back gracefully (to /tmp/v)
	dir := DefaultDataDir()
	if dir == "" {
		t.Error("DefaultDataDir returned empty string with no HOME")
	}
}
