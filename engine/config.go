package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds persistent user settings.
type Config struct {
	DefaultSSHKey string            `json:"default_ssh_key,omitempty"`
	Images        map[string]string `json:"images,omitempty"`
}

// defaultImages are built-in cloud image shortcuts shipped with v.
var defaultImages = map[string]string{
	"ubuntu-24.04":  "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
	"ubuntu-22.04":  "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img",
	"debian-13":     "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-generic-amd64.qcow2",
	"debian-12":     "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2",
	"alpine-v3.23":  "https://dl-cdn.alpinelinux.org/alpine/v3.23/releases/x86_64/alpine-virt-3.23.3-x86_64.iso",
	"rocky-10":      "https://dl.rockylinux.org/pub/rocky/10/images/x86_64/Rocky-10-GenericCloud-Base.latest.x86_64.qcow2",
}

func (e *Engine) configPath() string {
	return filepath.Join(e.DataDir, "config.json")
}

// LoadConfig reads the config file, returning defaults if it doesn't exist.
func (e *Engine) LoadConfig() (*Config, error) {
	data, err := os.ReadFile(e.configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// KnownImages returns the merged set of default + user-configured images.
// User entries override defaults with the same key.
func (e *Engine) KnownImages() map[string]string {
	cfg, err := e.LoadConfig()
	if err != nil {
		// Fall back to defaults on error
		m := make(map[string]string, len(defaultImages))
		for k, v := range defaultImages {
			m[k] = v
		}
		return m
	}

	merged := make(map[string]string, len(defaultImages)+len(cfg.Images))
	for k, v := range defaultImages {
		merged[k] = v
	}
	for k, v := range cfg.Images {
		merged[k] = v
	}
	return merged
}

// SaveConfig writes the config file to disk.
func (e *Engine) SaveConfig(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(e.configPath(), data, 0640)
}
