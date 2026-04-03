package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const defaultUserData = `#cloud-config
password: changeme
chpasswd:
  expire: false
ssh_pwauth: true
`

// GenerateCloudInit creates a cloud-init NoCloud ISO with meta-data and user-data.
// If sshKey is provided and userData is empty, it's injected into the default user-data.
// Requires genisoimage or mkisofs on the host.
func (e *Engine) GenerateCloudInit(isoPath, hostname, sshKey, userData string) error {
	tmpDir, err := os.MkdirTemp("", "v-cloudinit-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", hostname, hostname)
	if err := os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(metaData), 0644); err != nil {
		return fmt.Errorf("write meta-data: %w", err)
	}

	if userData == "" {
		userData = defaultUserData
		if sshKey != "" {
			userData += fmt.Sprintf("ssh_authorized_keys:\n  - %s\n", strings.TrimSpace(sshKey))
		}
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(userData), 0644); err != nil {
		return fmt.Errorf("write user-data: %w", err)
	}

	// Try genisoimage first, then mkisofs
	isoTool := findISOTool()
	if isoTool == "" {
		return fmt.Errorf("neither genisoimage nor mkisofs found; install one of them")
	}

	cmd := exec.Command(isoTool,
		"-output", isoPath,
		"-volid", "cidata",
		"-joliet",
		"-rock",
		filepath.Join(tmpDir, "meta-data"),
		filepath.Join(tmpDir, "user-data"),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s: %w", isoTool, out, err)
	}
	return nil
}

func findISOTool() string {
	for _, tool := range []string{"genisoimage", "mkisofs"} {
		if path, err := exec.LookPath(tool); err == nil {
			return path
		}
	}
	return ""
}
