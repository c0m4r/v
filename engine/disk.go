package engine

import (
	"fmt"
	"os/exec"
)

// CreateDisk creates a qcow2 disk image. If backingFile is non-empty,
// the image is created as a thin clone (copy-on-write) of the backing file.
func (e *Engine) CreateDisk(path, size, backingFile string) error {
	args := []string{"create", "-f", "qcow2"}
	if backingFile != "" {
		args = append(args, "-b", backingFile, "-F", "qcow2")
	}
	args = append(args, path, size)

	cmd := exec.Command("qemu-img", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img create: %s: %w", out, err)
	}
	return nil
}
