package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var validPCIAddr = regexp.MustCompile(`^[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9a-fA-F]$`)
var validDiskSize = regexp.MustCompile(`^[0-9]+[KMGTkmgt]$`)

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

// maxAllowedCPUs returns the CPU cap: 4× host logical CPUs, max 256.
func maxAllowedCPUs() int {
	n := runtime.NumCPU() * 4
	if n > 256 {
		return 256
	}
	return n
}

// maxAllowedMemoryMB returns the memory cap: total host RAM in MB, read from
// /proc/meminfo. Falls back to 65536 (64 GiB) if the file is unreadable.
func maxAllowedMemoryMB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 65536
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			var kb int
			_, _ = fmt.Sscanf(line, "MemTotal: %d kB", &kb)
			if kb > 0 {
				return kb / 1024
			}
		}
	}
	return 65536
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
	if maxCPUs := maxAllowedCPUs(); o.CPUs > maxCPUs {
		return fmt.Errorf("CPUs must not exceed %d (4× host logical CPUs)", maxCPUs)
	}
	if o.MemoryMB < 128 {
		o.MemoryMB = 512
	}
	if maxMem := maxAllowedMemoryMB(); o.MemoryMB > maxMem {
		return fmt.Errorf("memory must not exceed %d MB (host total RAM)", maxMem)
	}
	if o.DiskSize == "" {
		o.DiskSize = "10G"
	}
	if !validDiskSize.MatchString(o.DiskSize) {
		return fmt.Errorf("disk size must be a positive number followed by K, M, G, or T (e.g. 10G)")
	}
	if o.Image == "" {
		return fmt.Errorf("image is required")
	}
	// Reject path traversal in image name.
	if strings.ContainsAny(o.Image, "/\\\x00") || strings.Contains(o.Image, "..") {
		return fmt.Errorf("image name must not contain path separators or directory traversal")
	}
	// Reject YAML-injection characters in password and SSH key.
	if strings.ContainsAny(o.RootPassword, "\n\r\x00") {
		return fmt.Errorf("password must not contain newlines or null bytes")
	}
	if strings.ContainsAny(o.SSHKey, "\n\r\x00") {
		return fmt.Errorf("SSH key must not contain newlines or null bytes")
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
		if !validPCIAddr.MatchString(o.PCIAddr) {
			return fmt.Errorf("invalid PCI address %q: must match XX:XX.X format (e.g. 01:00.0)", o.PCIAddr)
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

	// Verify base image exists and is confined to the image directory.
	baseImage := filepath.Join(e.ImageDir, opts.Image)
	if !strings.HasPrefix(filepath.Clean(baseImage)+string(os.PathSeparator), filepath.Clean(e.ImageDir)+string(os.PathSeparator)) {
		return nil, fmt.Errorf("image %q escapes the image directory", opts.Image)
	}
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
		if maxCPUs := maxAllowedCPUs(); opts.CPUs > maxCPUs {
			return nil, fmt.Errorf("CPUs must not exceed %d (4× host logical CPUs)", maxCPUs)
		}
		vm.CPUs = opts.CPUs
	}
	if opts.MemoryMB > 0 {
		if opts.MemoryMB < 128 {
			return nil, fmt.Errorf("memory must be at least 128 MB")
		}
		if maxMem := maxAllowedMemoryMB(); opts.MemoryMB > maxMem {
			return nil, fmt.Errorf("memory must not exceed %d MB (host total RAM)", maxMem)
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
			if !validPCIAddr.MatchString(opts.PCIAddr) {
				return nil, fmt.Errorf("invalid PCI address %q: must match XX:XX.X format (e.g. 01:00.0)", opts.PCIAddr)
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
