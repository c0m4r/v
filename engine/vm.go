package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// VM represents a virtual machine's persistent configuration.
type VM struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CPUs      int       `json:"cpus"`
	MemoryMB  int       `json:"memory_mb"`
	DiskSize  string    `json:"disk_size"`
	BaseImage string    `json:"base_image"`
	BootDev   string    `json:"boot_dev"` // "disk" or "cdrom"
	NetMode   string    `json:"net_mode"`
	MACAddr   string    `json:"mac_address"`
	SSHPort   int       `json:"ssh_port,omitempty"` // host port forwarded to VM:22 (user-mode only)
	CreatedAt time.Time `json:"created_at"`
}

// State represents runtime VM state, derived from process inspection.
type State string

const (
	StateRunning State = "running"
	StateStopped State = "stopped"
)

// CreateVMOpts holds options for creating a new VM.
type CreateVMOpts struct {
	Name     string
	CPUs     int
	MemoryMB int
	DiskSize string
	Image    string // name of cached base image (e.g. "ubuntu-24.04.qcow2")
	NetMode  string // "bridge" or "user"
	SSHKey   string // public SSH key to authorize (optional)
	UserData string // cloud-init user-data (optional, overrides default)
}

func (o *CreateVMOpts) validate() error {
	if o.Name == "" {
		return fmt.Errorf("name is required")
	}
	if !validName.MatchString(o.Name) {
		return fmt.Errorf("name must match %s", validName.String())
	}
	if len(o.Name) > 64 {
		return fmt.Errorf("name must be 64 characters or fewer")
	}
	if o.CPUs < 1 {
		o.CPUs = 1
	}
	if o.MemoryMB < 128 {
		o.MemoryMB = 512
	}
	if o.DiskSize == "" {
		o.DiskSize = "10G"
	}
	if o.Image == "" {
		return fmt.Errorf("image is required")
	}
	if o.NetMode == "" {
		o.NetMode = "user"
	}
	if o.NetMode != "user" && o.NetMode != "bridge" {
		return fmt.Errorf("net mode must be 'user' or 'bridge'")
	}
	return nil
}

// CreateVM creates a new VM: generates ID, creates disk, writes metadata.
func (e *Engine) CreateVM(opts CreateVMOpts) (*VM, error) {
	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	// Apply default SSH key from config if none provided
	if opts.SSHKey == "" {
		if cfg, err := e.LoadConfig(); err == nil && cfg.DefaultSSHKey != "" {
			opts.SSHKey = cfg.DefaultSSHKey
		}
	}

	// Check name uniqueness
	vms, err := e.ListVMs()
	if err != nil {
		return nil, err
	}
	for _, existing := range vms {
		if existing.Name == opts.Name {
			return nil, fmt.Errorf("VM with name %q already exists", opts.Name)
		}
	}

	// Verify base image exists
	baseImage := filepath.Join(e.ImageDir, opts.Image)
	if _, err := os.Stat(baseImage); err != nil {
		return nil, fmt.Errorf("base image %q not found (run 'v image pull' first)", opts.Image)
	}

	id := generateID()
	vmDir := e.VMPath(id)
	if err := os.MkdirAll(vmDir, 0750); err != nil {
		return nil, fmt.Errorf("create VM directory: %w", err)
	}

	iso := IsISO(opts.Image)

	// For ISO images: create a blank disk (the ISO is the install media).
	// For cloud images: create a thin clone (copy-on-write) of the base.
	diskPath := filepath.Join(vmDir, "disk.qcow2")
	backingFile := baseImage
	if iso {
		backingFile = ""
	}
	if err := e.CreateDisk(diskPath, opts.DiskSize, backingFile); err != nil {
		_ = os.RemoveAll(vmDir)
		return nil, fmt.Errorf("create disk: %w", err)
	}

	// Cloud-init is only useful for cloud images, not ISO installers.
	if !iso {
		ciPath := filepath.Join(vmDir, "cloud-init.iso")
		if err := e.GenerateCloudInit(ciPath, opts.Name, opts.SSHKey, opts.UserData); err != nil {
			_ = os.RemoveAll(vmDir)
			return nil, fmt.Errorf("generate cloud-init: %w", err)
		}
	}

	var sshPort int
	if opts.NetMode == "user" {
		sshPort = e.nextSSHPort(vms)
	}

	bootDev := "disk"
	if iso {
		bootDev = "cdrom"
	}

	vm := &VM{
		ID:        id,
		Name:      opts.Name,
		CPUs:      opts.CPUs,
		MemoryMB:  opts.MemoryMB,
		DiskSize:  opts.DiskSize,
		BaseImage: opts.Image,
		BootDev:   bootDev,
		NetMode:   opts.NetMode,
		MACAddr:   generateMAC(),
		SSHPort:   sshPort,
		CreatedAt: time.Now().UTC(),
	}

	if err := e.saveVM(vm); err != nil {
		_ = os.RemoveAll(vmDir)
		return nil, err
	}

	return vm, nil
}

// GetVM loads a VM by ID or name.
func (e *Engine) GetVM(idOrName string) (*VM, error) {
	// Try direct ID lookup first
	metaPath := filepath.Join(e.VMPath(idOrName), "vm.json")
	if data, err := os.ReadFile(metaPath); err == nil {
		var vm VM
		if err := json.Unmarshal(data, &vm); err != nil {
			return nil, fmt.Errorf("parse VM metadata: %w", err)
		}
		return &vm, nil
	}

	// Fall back to name search
	vms, err := e.ListVMs()
	if err != nil {
		return nil, err
	}
	for _, vm := range vms {
		if vm.Name == idOrName {
			return vm, nil
		}
	}

	return nil, fmt.Errorf("VM %q not found", idOrName)
}

// ListVMs returns all VMs by reading metadata files.
func (e *Engine) ListVMs() ([]*VM, error) {
	entries, err := os.ReadDir(e.VMDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read VM directory: %w", err)
	}

	var vms []*VM
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(e.VMDir, entry.Name(), "vm.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue // skip directories without metadata
		}
		var vm VM
		if err := json.Unmarshal(data, &vm); err != nil {
			continue
		}
		vms = append(vms, &vm)
	}
	return vms, nil
}

// DeleteVM removes a VM and all its files. The VM must be stopped.
func (e *Engine) DeleteVM(idOrName string) error {
	vm, err := e.GetVM(idOrName)
	if err != nil {
		return err
	}

	state, err := e.VMState(vm.ID)
	if err != nil {
		return err
	}
	if state == StateRunning {
		return fmt.Errorf("VM %q is running; stop it first", vm.Name)
	}

	return os.RemoveAll(e.VMPath(vm.ID))
}

// SetBootDev changes the boot device for a VM ("disk" or "cdrom").
// The VM must be stopped.
func (e *Engine) SetBootDev(idOrName, dev string) error {
	if dev != "disk" && dev != "cdrom" {
		return fmt.Errorf("boot device must be 'disk' or 'cdrom'")
	}

	vm, err := e.GetVM(idOrName)
	if err != nil {
		return err
	}

	state, err := e.VMState(vm.ID)
	if err != nil {
		return err
	}
	if state == StateRunning {
		return fmt.Errorf("VM %q is running; stop it first", vm.Name)
	}

	vm.BootDev = dev
	return e.saveVM(vm)
}

func (e *Engine) saveVM(vm *VM) error {
	data, err := json.MarshalIndent(vm, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal VM metadata: %w", err)
	}
	metaPath := filepath.Join(e.VMPath(vm.ID), "vm.json")
	return os.WriteFile(metaPath, data, 0640)
}

func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

const sshPortBase = 2222

// nextSSHPort finds the next available host port for SSH forwarding.
func (e *Engine) nextSSHPort(existingVMs []*VM) int {
	used := make(map[int]bool)
	for _, vm := range existingVMs {
		if vm.SSHPort > 0 {
			used[vm.SSHPort] = true
		}
	}
	for port := sshPortBase; port < sshPortBase+1000; port++ {
		if !used[port] {
			return port
		}
	}
	return sshPortBase
}

func generateMAC() string {
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", b[0], b[1], b[2])
}
