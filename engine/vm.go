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
	GPU       string    `json:"gpu,omitempty"`      // "none", "virtio", "passthrough"
	PCIAddr   string    `json:"pci_addr,omitempty"` // PCI address for passthrough (e.g. "01:00.0")
	Audio     string    `json:"audio,omitempty"`    // "none", "pa", "pipewire", "alsa"
	MACAddr      string    `json:"mac_address"`
	SSHPort      int       `json:"ssh_port,omitempty"`      // host port forwarded to VM:22 (user-mode only)
	RootPassword string    `json:"root_password,omitempty"` // stored plaintext password (local use only)
	CreatedAt    time.Time `json:"created_at"`
}

// State represents runtime VM state, derived from process inspection.
type State string

const (
	StateRunning State = "running"
	StateStopped State = "stopped"
)

// CreateVMOpts holds options for creating a new VM.
type CreateVMOpts struct {
	Name         string
	CPUs         int
	MemoryMB     int
	DiskSize     string
	Image        string // name of cached base image (e.g. "ubuntu-24.04.qcow2")
	NetMode      string // "bridge" or "user"
	GPU          string // "none", "virtio", "passthrough"
	PCIAddr      string // PCI address for passthrough mode (e.g. "01:00.0")
	Audio        string // "none", "pa", "pipewire", "alsa"
	SSHKey       string // public SSH key to authorize (optional)
	RootPassword string // plaintext password; empty = auto-generate, "none" = no password
	UserData     string // cloud-init user-data (optional, overrides everything)
}

// UpdateVMOpts holds fields that can be changed on a stopped VM.
// Zero/empty values mean "leave unchanged" — the UI always sends all fields.
type UpdateVMOpts struct {
	CPUs     int    `json:"CPUs"`
	MemoryMB int    `json:"MemoryMB"`
	GPU      string `json:"GPU"`
	PCIAddr  string `json:"PCIAddr"`
	Audio    string `json:"Audio"`
	BootDev  string `json:"BootDev"`
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
	if o.GPU == "" {
		o.GPU = "none"
	}
	switch o.GPU {
	case "none", "virtio":
		// ok
	case "passthrough":
		if o.PCIAddr == "" {
			return fmt.Errorf("--pci-addr is required for passthrough GPU mode")
		}
	default:
		return fmt.Errorf("gpu mode must be 'none', 'virtio', or 'passthrough'")
	}
	if o.Audio == "" {
		o.Audio = "none"
	}
	switch o.Audio {
	case "none", "pa", "pipewire", "alsa":
		// ok
	default:
		return fmt.Errorf("audio mode must be 'none', 'pa', 'pipewire', or 'alsa'")
	}
	return nil
}

// CreateVM creates a new VM: generates ID, creates disk, writes metadata.
// The plaintext root password is stored in vm.RootPassword (empty if no password was set).
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

	// Resolve root password: auto-generate if not specified, clear if "none"
	password := opts.RootPassword
	if opts.UserData == "" {
		switch password {
		case "none":
			password = ""
		case "":
			password = generatePassword()
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
		if err := e.GenerateCloudInit(ciPath, opts.Name, opts.SSHKey, password, opts.UserData); err != nil {
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
		ID:           id,
		Name:         opts.Name,
		CPUs:         opts.CPUs,
		MemoryMB:     opts.MemoryMB,
		DiskSize:     opts.DiskSize,
		BaseImage:    opts.Image,
		BootDev:      bootDev,
		NetMode:      opts.NetMode,
		GPU:          opts.GPU,
		PCIAddr:      opts.PCIAddr,
		Audio:        opts.Audio,
		MACAddr:      generateMAC(),
		SSHPort:      sshPort,
		RootPassword: password,
		CreatedAt:    time.Now().UTC(),
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

// UpdateVM applies the given opts to a stopped VM and persists the result.
// Zero/empty values in opts are skipped (field unchanged).
func (e *Engine) UpdateVM(idOrName string, opts UpdateVMOpts) (*VM, error) {
	vm, err := e.GetVM(idOrName)
	if err != nil {
		return nil, err
	}
	state, err := e.VMState(vm.ID)
	if err != nil {
		return nil, err
	}
	if state == StateRunning {
		return nil, fmt.Errorf("VM %q is running; stop it first", vm.Name)
	}

	if opts.CPUs > 0 {
		vm.CPUs = opts.CPUs
	}
	if opts.MemoryMB > 0 {
		if opts.MemoryMB < 128 {
			return nil, fmt.Errorf("memory must be at least 128 MB")
		}
		vm.MemoryMB = opts.MemoryMB
	}
	if opts.GPU != "" {
		switch opts.GPU {
		case "none", "virtio":
			// ok
		case "passthrough":
			if opts.PCIAddr == "" {
				return nil, fmt.Errorf("PCI address is required for passthrough mode")
			}
		default:
			return nil, fmt.Errorf("gpu mode must be 'none', 'virtio', or 'passthrough'")
		}
		vm.GPU = opts.GPU
		vm.PCIAddr = opts.PCIAddr
	}
	if opts.Audio != "" {
		switch opts.Audio {
		case "none", "pa", "pipewire", "alsa":
			// ok
		default:
			return nil, fmt.Errorf("audio mode must be 'none', 'pa', 'pipewire', or 'alsa'")
		}
		vm.Audio = opts.Audio
	}
	if opts.BootDev != "" {
		if opts.BootDev != "disk" && opts.BootDev != "cdrom" {
			return nil, fmt.Errorf("boot device must be 'disk' or 'cdrom'")
		}
		vm.BootDev = opts.BootDev
	}

	if err := e.saveVM(vm); err != nil {
		return nil, err
	}
	return vm, nil
}

// SetGPU updates the GPU mode for a stopped VM.
func (e *Engine) SetGPU(idOrName, mode, pciAddr string) error {
	_, err := e.UpdateVM(idOrName, UpdateVMOpts{GPU: mode, PCIAddr: pciAddr})
	return err
}

// SetAudio updates the audio backend for a stopped VM.
func (e *Engine) SetAudio(idOrName, mode string) error {
	_, err := e.UpdateVM(idOrName, UpdateVMOpts{Audio: mode})
	return err
}

// SetRootPassword updates the stored root password for a VM (local record only).
func (e *Engine) SetRootPassword(idOrName, password string) error {
	vm, err := e.GetVM(idOrName)
	if err != nil {
		return err
	}
	vm.RootPassword = password
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

const passwordChars = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generatePassword() string {
	b := make([]byte, 16)
	rand.Read(b)
	out := make([]byte, 16)
	for i, c := range b {
		out[i] = passwordChars[int(c)%len(passwordChars)]
	}
	return string(out)
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
