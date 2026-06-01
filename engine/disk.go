package engine

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// qcow2Magic is the 4-byte header that identifies a QCOW2 image.
var qcow2Magic = []byte{'Q', 'F', 'I', 0xfb}

// detectImageFormat returns "qcow2" if the file starts with the QCOW2 magic
// header, "raw" otherwise. This lets CreateDisk pass the correct -F flag to
// qemu-img when cloning a backing file (raw .img installers vs qcow2 cloud
// images).
func detectImageFormat(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	var header [4]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return "raw", nil
		}
		return "", err
	}
	if string(header[:]) == string(qcow2Magic) {
		return "qcow2", nil
	}
	return "raw", nil
}

// CreateDisk creates a qcow2 disk image. If backingFile is non-empty,
// the image is created as a thin clone (copy-on-write) of the backing file.
func (e *Engine) CreateDisk(path, size, backingFile string) error {
	args := []string{"create", "-f", "qcow2"}
	if backingFile != "" {
		bfFormat, err := detectImageFormat(backingFile)
		if err != nil {
			return fmt.Errorf("detect backing file format: %w", err)
		}
		args = append(args, "-b", backingFile, "-F", bfFormat)
	}
	args = append(args, path, size)

	cmd := exec.Command("qemu-img", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img create: %s: %w", out, err)
	}
	return nil
}
