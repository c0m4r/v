package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// buildUserData generates the default cloud-init user-data.
// password is the plaintext password; empty string means no password auth.
// The password is applied to both root and the distro default user.
func buildUserData(sshKey, password string) string {
	var b strings.Builder
	b.WriteString("#cloud-config\n")
	if password != "" {
		// `password` sets the default user's password; chpasswd list also sets root.
		b.WriteString("password: " + password + "\n")
		b.WriteString("chpasswd:\n  expire: false\n  list: |\n    root:" + password + "\nssh_pwauth: true\n")
	} else {
		b.WriteString("chpasswd:\n  expire: false\nssh_pwauth: false\n")
	}
	if sshKey != "" {
		b.WriteString("ssh_authorized_keys:\n  - " + strings.TrimSpace(sshKey) + "\n")
	}
	return b.String()
}

// GenerateCloudInit creates a cloud-init NoCloud ISO with meta-data and user-data.
// password is the plaintext password to set; pass empty string for no password.
// If userData is non-empty it overrides everything else.
// Requires genisoimage or mkisofs on the host.
func (e *Engine) GenerateCloudInit(isoPath, hostname, sshKey, password, userData string) error {
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
		userData = buildUserData(sshKey, password)
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
