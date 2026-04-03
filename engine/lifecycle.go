package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// VMState checks whether a VM is running by inspecting its PID file.
func (e *Engine) VMState(id string) (State, error) {
	pidFile := filepath.Join(e.VMPath(id), "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return StateStopped, nil
		}
		return StateStopped, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return StateStopped, nil
	}

	// Check if process is alive
	if err := syscall.Kill(pid, 0); err != nil {
		// Process is dead, clean up stale PID file
		_ = os.Remove(pidFile)
		return StateStopped, nil
	}

	return StateRunning, nil
}

// StartVM launches QEMU for the given VM.
func (e *Engine) StartVM(idOrName string) error {
	vm, err := e.GetVM(idOrName)
	if err != nil {
		return err
	}

	state, err := e.VMState(vm.ID)
	if err != nil {
		return err
	}
	if state == StateRunning {
		return fmt.Errorf("VM %q is already running", vm.Name)
	}

	vmDir := e.VMPath(vm.ID)
	diskPath := filepath.Join(vmDir, "disk.qcow2")
	pidPath := filepath.Join(vmDir, "pid")
	qmpPath := filepath.Join(vmDir, "qmp.sock")
	consolePath := filepath.Join(vmDir, "console.sock")

	// Clean up stale sockets
	_ = os.Remove(qmpPath)
	_ = os.Remove(consolePath)

	args := []string{
		"-enable-kvm",
		"-cpu", "host",
		"-smp", strconv.Itoa(vm.CPUs),
		"-m", strconv.Itoa(vm.MemoryMB),
		"-drive", fmt.Sprintf("file=%s,format=qcow2,if=virtio", diskPath),
	}

	// Attach CDROM and set boot order based on BootDev.
	// "cdrom": boot from the base ISO image (installer).
	// "disk" (default): boot from the qcow2 disk.
	bootDev := vm.BootDev
	if bootDev == "" {
		bootDev = "disk"
	}

	if bootDev == "cdrom" && IsISO(vm.BaseImage) {
		isoPath := filepath.Join(e.ImageDir, vm.BaseImage)
		args = append(args, "-cdrom", isoPath, "-boot", "d")
	} else {
		ciPath := filepath.Join(vmDir, "cloud-init.iso")
		if _, err := os.Stat(ciPath); err == nil {
			args = append(args, "-cdrom", ciPath)
		}
	}

	args = append(args,
		"-qmp", fmt.Sprintf("unix:%s,server=on,wait=off", qmpPath),
		"-serial", fmt.Sprintf("unix:%s,server=on,wait=off", consolePath),
		"-display", "none",
		"-daemonize",
		"-pidfile", pidPath,
	)

	// Networking
	switch vm.NetMode {
	case "bridge":
		tapName := fmt.Sprintf("v-tap-%s", vm.ID[:6])
		args = append(args,
			"-netdev", fmt.Sprintf("tap,id=net0,ifname=%s,script=no,downscript=no", tapName),
			"-device", fmt.Sprintf("virtio-net-pci,netdev=net0,mac=%s", vm.MACAddr),
		)
	default: // "user"
		sshFwd := ""
		if vm.SSHPort > 0 {
			sshFwd = fmt.Sprintf(",hostfwd=tcp::%d-:22", vm.SSHPort)
		}
		args = append(args,
			"-netdev", fmt.Sprintf("user,id=net0%s", sshFwd),
			"-device", fmt.Sprintf("virtio-net-pci,netdev=net0,mac=%s", vm.MACAddr),
		)
	}

	cmd := exec.Command("qemu-system-x86_64", args...)
	cmd.Dir = vmDir

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start QEMU: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// StopVM sends an ACPI shutdown signal via QMP (graceful).
func (e *Engine) StopVM(idOrName string) error {
	vm, err := e.GetVM(idOrName)
	if err != nil {
		return err
	}

	state, err := e.VMState(vm.ID)
	if err != nil {
		return err
	}
	if state == StateStopped {
		return fmt.Errorf("VM %q is not running", vm.Name)
	}

	qmpPath := filepath.Join(e.VMPath(vm.ID), "qmp.sock")
	c, err := qmpConnect(qmpPath)
	if err != nil {
		return fmt.Errorf("connect to VM: %w", err)
	}
	defer c.close()

	return c.execute("system_powerdown", nil)
}

// ForceStopVM kills the QEMU process immediately.
func (e *Engine) ForceStopVM(idOrName string) error {
	vm, err := e.GetVM(idOrName)
	if err != nil {
		return err
	}

	state, err := e.VMState(vm.ID)
	if err != nil {
		return err
	}
	if state == StateStopped {
		return fmt.Errorf("VM %q is not running", vm.Name)
	}

	pidFile := filepath.Join(e.VMPath(vm.ID), "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("read PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("parse PID: %w", err)
	}

	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill process: %w", err)
	}

	// Wait briefly for cleanup, then remove PID file
	time.Sleep(200 * time.Millisecond)
	_ = os.Remove(pidFile)
	_ = os.Remove(filepath.Join(e.VMPath(vm.ID), "qmp.sock"))
	_ = os.Remove(filepath.Join(e.VMPath(vm.ID), "console.sock"))

	return nil
}

// RestartVM does a graceful stop followed by a start. If the VM doesn't
// shut down within the timeout, it's force-stopped.
func (e *Engine) RestartVM(idOrName string) error {
	vm, err := e.GetVM(idOrName)
	if err != nil {
		return err
	}

	state, err := e.VMState(vm.ID)
	if err != nil {
		return err
	}

	if state == StateRunning {
		// Try graceful shutdown first
		_ = e.StopVM(vm.ID)

		// Wait up to 30 seconds for shutdown
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(1 * time.Second)
			s, _ := e.VMState(vm.ID)
			if s == StateStopped {
				break
			}
		}

		// Force stop if still running
		s, _ := e.VMState(vm.ID)
		if s == StateRunning {
			if err := e.ForceStopVM(vm.ID); err != nil {
				return fmt.Errorf("force stop for restart: %w", err)
			}
		}
	}

	return e.StartVM(vm.ID)
}

// ConsoleSocketPath returns the path to the VM's serial console unix socket.
func (e *Engine) ConsoleSocketPath(idOrName string) (string, error) {
	vm, err := e.GetVM(idOrName)
	if err != nil {
		return "", err
	}

	state, err := e.VMState(vm.ID)
	if err != nil {
		return "", err
	}
	if state != StateRunning {
		return "", fmt.Errorf("VM %q is not running", vm.Name)
	}

	sockPath := filepath.Join(e.VMPath(vm.ID), "console.sock")
	if _, err := os.Stat(sockPath); err != nil {
		return "", fmt.Errorf("console socket not found")
	}

	return sockPath, nil
}
