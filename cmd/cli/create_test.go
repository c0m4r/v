package cli

import (
	"testing"

	"github.com/c0m4r/v/engine"
)

func TestResolveImageName_Known(t *testing.T) {
	dir := t.TempDir()
	e, err := engine.New(dir)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	got := resolveImageName(e, "ubuntu-24.04")
	want := "noble-server-cloudimg-amd64.img"
	if got != want {
		t.Errorf("resolveImageName(ubuntu-24.04) = %q, want %q", got, want)
	}
}

func TestResolveImageName_Unknown(t *testing.T) {
	dir := t.TempDir()
	e, _ := engine.New(dir)

	got := resolveImageName(e, "custom-image.qcow2")
	if got != "custom-image.qcow2" {
		t.Errorf("resolveImageName(custom) = %q, want passthrough", got)
	}
}

func TestResolveImageName_ISO(t *testing.T) {
	dir := t.TempDir()
	e, _ := engine.New(dir)

	got := resolveImageName(e, "alpine-v3.23")
	want := "alpine-virt-3.23.3-x86_64.iso"
	if got != want {
		t.Errorf("resolveImageName(alpine-v3.23) = %q, want %q", got, want)
	}
}

func TestResolveImageName_UserConfigured(t *testing.T) {
	dir := t.TempDir()
	e, _ := engine.New(dir)

	// Add a custom image to config
	cfg := &engine.Config{
		Images: map[string]string{
			"my-os": "https://example.com/path/to/my-os-v1.qcow2",
		},
	}
	_ = e.SaveConfig(cfg)

	got := resolveImageName(e, "my-os")
	if got != "my-os-v1.qcow2" {
		t.Errorf("resolveImageName(my-os) = %q, want my-os-v1.qcow2", got)
	}
}
