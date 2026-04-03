package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	e, err := New(dir)
	if err != nil {
		t.Fatalf("New(%q): %v", dir, err)
	}
	return e
}

func TestLoadConfig_NoFile(t *testing.T) {
	e := testEngine(t)
	cfg, err := e.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DefaultSSHKey != "" {
		t.Errorf("expected empty SSH key, got %q", cfg.DefaultSSHKey)
	}
	if cfg.Images != nil {
		t.Errorf("expected nil Images, got %v", cfg.Images)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	e := testEngine(t)
	cfg := &Config{
		DefaultSSHKey: "ssh-ed25519 AAAA testkey",
		Images: map[string]string{
			"custom": "https://example.com/custom.qcow2",
		},
	}
	if err := e.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := e.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.DefaultSSHKey != cfg.DefaultSSHKey {
		t.Errorf("SSH key: got %q, want %q", loaded.DefaultSSHKey, cfg.DefaultSSHKey)
	}
	if loaded.Images["custom"] != cfg.Images["custom"] {
		t.Errorf("Images[custom]: got %q, want %q", loaded.Images["custom"], cfg.Images["custom"])
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	e := testEngine(t)
	_ = os.WriteFile(e.configPath(), []byte("{invalid"), 0640)

	_, err := e.LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestKnownImages_DefaultsOnly(t *testing.T) {
	e := testEngine(t)
	images := e.KnownImages()

	if len(images) != len(defaultImages) {
		t.Errorf("expected %d images, got %d", len(defaultImages), len(images))
	}
	for k, v := range defaultImages {
		if images[k] != v {
			t.Errorf("images[%s]: got %q, want %q", k, images[k], v)
		}
	}
}

func TestKnownImages_UserOverride(t *testing.T) {
	e := testEngine(t)
	cfg := &Config{
		Images: map[string]string{
			"ubuntu-24.04": "https://custom-mirror.example.com/ubuntu.img",
			"my-distro":    "https://example.com/my-distro.qcow2",
		},
	}
	if err := e.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	images := e.KnownImages()

	// User override
	if images["ubuntu-24.04"] != cfg.Images["ubuntu-24.04"] {
		t.Errorf("ubuntu-24.04: got %q, want %q", images["ubuntu-24.04"], cfg.Images["ubuntu-24.04"])
	}
	// User addition
	if images["my-distro"] != cfg.Images["my-distro"] {
		t.Errorf("my-distro: got %q, want %q", images["my-distro"], cfg.Images["my-distro"])
	}
	// Default still present
	if images["debian-12"] != defaultImages["debian-12"] {
		t.Errorf("debian-12: got %q, want %q", images["debian-12"], defaultImages["debian-12"])
	}
	// Total count
	want := len(defaultImages) + 1 // "my-distro" is new, "ubuntu-24.04" is an override
	if len(images) != want {
		t.Errorf("total images: got %d, want %d", len(images), want)
	}
}

func TestKnownImages_BadConfig(t *testing.T) {
	e := testEngine(t)
	_ = os.WriteFile(e.configPath(), []byte("{bad json"), 0640)

	images := e.KnownImages()
	// Should fall back to defaults
	if len(images) != len(defaultImages) {
		t.Errorf("expected %d default images on error, got %d", len(defaultImages), len(images))
	}
}

func TestConfigPath(t *testing.T) {
	e := testEngine(t)
	want := filepath.Join(e.DataDir, "config.json")
	if got := e.configPath(); got != want {
		t.Errorf("configPath: got %q, want %q", got, want)
	}
}

func TestSaveConfig_JSONFormat(t *testing.T) {
	e := testEngine(t)
	cfg := &Config{DefaultSSHKey: "testkey"}
	if err := e.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	data, err := os.ReadFile(e.configPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Verify it's valid, indented JSON
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}
	if parsed["default_ssh_key"] != "testkey" {
		t.Errorf("unexpected JSON content: %s", data)
	}
}
