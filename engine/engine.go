package engine

import (
	"fmt"
	"os"
	"path/filepath"
)

// Engine is the core virtualization manager. Both CLI and web UI
// call methods on this struct — it owns all VM state and operations.
type Engine struct {
	DataDir  string
	VMDir    string
	ImageDir string
}

// DefaultDataDir returns ~/.local/share/v as the default data directory.
func DefaultDataDir() string {
	if d := os.Getenv("V_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/v"
	}
	return filepath.Join(home, ".local", "share", "v")
}

// New creates an Engine and ensures all data directories exist.
func New(dataDir string) (*Engine, error) {
	e := &Engine{
		DataDir:  dataDir,
		VMDir:    filepath.Join(dataDir, "vms"),
		ImageDir: filepath.Join(dataDir, "images"),
	}

	for _, dir := range []string{e.DataDir, e.VMDir, e.ImageDir} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return e, nil
}

// VMPath returns the directory path for a given VM.
func (e *Engine) VMPath(id string) string {
	return filepath.Join(e.VMDir, id)
}
