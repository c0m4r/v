package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_NameRequired(t *testing.T) {
	opts := &CreateVMOpts{Image: "test.qcow2"}
	if err := opts.validate(); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidate_NamePattern(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"myvm", false},
		{"my-vm", false},
		{"my.vm", false},
		{"my_vm", false},
		{"VM1", false},
		{"1start", false},
		{"-invalid", true},
		{".invalid", true},
		{"has space", true},
		{"special!char", true},
		{"", true},
	}
	for _, tt := range tests {
		opts := &CreateVMOpts{Name: tt.name, Image: "test.qcow2"}
		err := opts.validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("validate(%q): got err=%v, wantErr=%v", tt.name, err, tt.wantErr)
		}
	}
}

func TestValidate_NameTooLong(t *testing.T) {
	opts := &CreateVMOpts{
		Name:  strings.Repeat("a", 65),
		Image: "test.qcow2",
	}
	if err := opts.validate(); err == nil {
		t.Fatal("expected error for name >64 chars")
	}
}

func TestValidate_Defaults(t *testing.T) {
	opts := &CreateVMOpts{Name: "test", Image: "test.qcow2"}
	if err := opts.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if opts.CPUs != 1 {
		t.Errorf("CPUs: got %d, want 1", opts.CPUs)
	}
	if opts.MemoryMB != 512 {
		t.Errorf("MemoryMB: got %d, want 512", opts.MemoryMB)
	}
	if opts.DiskSize != "10G" {
		t.Errorf("DiskSize: got %q, want 10G", opts.DiskSize)
	}
	if opts.NetMode != "user" {
		t.Errorf("NetMode: got %q, want user", opts.NetMode)
	}
}

func TestValidate_ImageRequired(t *testing.T) {
	opts := &CreateVMOpts{Name: "test"}
	if err := opts.validate(); err == nil {
		t.Fatal("expected error for empty image")
	}
}

func TestValidate_InvalidNetMode(t *testing.T) {
	opts := &CreateVMOpts{Name: "test", Image: "test.qcow2", NetMode: "invalid"}
	if err := opts.validate(); err == nil {
		t.Fatal("expected error for invalid net mode")
	}
}

func TestValidate_ValidNetModes(t *testing.T) {
	for _, mode := range []string{"user", "bridge"} {
		opts := &CreateVMOpts{Name: "test", Image: "test.qcow2", NetMode: mode}
		if err := opts.validate(); err != nil {
			t.Errorf("validate(net=%s): unexpected error: %v", mode, err)
		}
	}
}

func TestValidate_CPUsNotClamped(t *testing.T) {
	opts := &CreateVMOpts{Name: "test", Image: "test.qcow2", CPUs: 4}
	_ = opts.validate()
	if opts.CPUs != 4 {
		t.Errorf("CPUs: got %d, want 4 (should not be clamped)", opts.CPUs)
	}
}

func TestValidate_MemoryNotClamped(t *testing.T) {
	opts := &CreateVMOpts{Name: "test", Image: "test.qcow2", MemoryMB: 2048}
	_ = opts.validate()
	if opts.MemoryMB != 2048 {
		t.Errorf("MemoryMB: got %d, want 2048", opts.MemoryMB)
	}
}

func TestGenerateID(t *testing.T) {
	id := generateID()
	if len(id) != 8 {
		t.Errorf("generateID: got len=%d, want 8", len(id))
	}
	// Should be unique
	id2 := generateID()
	if id == id2 {
		t.Error("generateID: two calls returned the same ID")
	}
}

func TestGenerateMAC(t *testing.T) {
	mac := generateMAC()
	if !strings.HasPrefix(mac, "52:54:00:") {
		t.Errorf("generateMAC: got %q, want prefix 52:54:00:", mac)
	}
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		t.Errorf("generateMAC: got %d parts, want 6", len(parts))
	}
}

func TestNextSSHPort_Empty(t *testing.T) {
	e := testEngine(t)
	port := e.nextSSHPort(nil)
	if port != sshPortBase {
		t.Errorf("nextSSHPort: got %d, want %d", port, sshPortBase)
	}
}

func TestNextSSHPort_SkipsUsed(t *testing.T) {
	e := testEngine(t)
	vms := []*VM{
		{SSHPort: sshPortBase},
		{SSHPort: sshPortBase + 1},
	}
	port := e.nextSSHPort(vms)
	if port != sshPortBase+2 {
		t.Errorf("nextSSHPort: got %d, want %d", port, sshPortBase+2)
	}
}

func TestNextSSHPort_GapFilling(t *testing.T) {
	e := testEngine(t)
	vms := []*VM{
		{SSHPort: sshPortBase},
		{SSHPort: sshPortBase + 2}, // gap at +1
	}
	port := e.nextSSHPort(vms)
	if port != sshPortBase+1 {
		t.Errorf("nextSSHPort: got %d, want %d (should fill gap)", port, sshPortBase+1)
	}
}

func TestListVMs_EmptyDir(t *testing.T) {
	e := testEngine(t)
	vms, err := e.ListVMs()
	if err != nil {
		t.Fatalf("ListVMs: %v", err)
	}
	if len(vms) != 0 {
		t.Errorf("expected 0 VMs, got %d", len(vms))
	}
}

func TestSaveAndGetVM(t *testing.T) {
	e := testEngine(t)
	vm := &VM{
		ID:       "abcd1234",
		Name:     "test-vm",
		CPUs:     2,
		MemoryMB: 1024,
		DiskSize: "20G",
		BootDev:  "disk",
		NetMode:  "user",
		MACAddr:  "52:54:00:aa:bb:cc",
		SSHPort:  2222,
	}
	_ = os.MkdirAll(e.VMPath(vm.ID), 0750)
	if err := e.saveVM(vm); err != nil {
		t.Fatalf("saveVM: %v", err)
	}

	// Get by ID
	got, err := e.GetVM(vm.ID)
	if err != nil {
		t.Fatalf("GetVM by ID: %v", err)
	}
	if got.Name != vm.Name {
		t.Errorf("Name: got %q, want %q", got.Name, vm.Name)
	}
	if got.BootDev != "disk" {
		t.Errorf("BootDev: got %q, want disk", got.BootDev)
	}

	// Get by name
	got, err = e.GetVM(vm.Name)
	if err != nil {
		t.Fatalf("GetVM by name: %v", err)
	}
	if got.ID != vm.ID {
		t.Errorf("ID: got %q, want %q", got.ID, vm.ID)
	}
}

func TestGetVM_NotFound(t *testing.T) {
	e := testEngine(t)
	_, err := e.GetVM("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent VM")
	}
}

func TestListVMs_SkipsBadEntries(t *testing.T) {
	e := testEngine(t)

	// Create a valid VM
	vmDir := filepath.Join(e.VMDir, "good-vm")
	_ = os.MkdirAll(vmDir, 0750)
	data, _ := json.Marshal(&VM{ID: "good-vm", Name: "good"})
	_ = os.WriteFile(filepath.Join(vmDir, "vm.json"), data, 0640)

	// Create a dir without vm.json
	_ = os.MkdirAll(filepath.Join(e.VMDir, "no-meta"), 0750)

	// Create a dir with invalid JSON
	badDir := filepath.Join(e.VMDir, "bad-json")
	_ = os.MkdirAll(badDir, 0750)
	_ = os.WriteFile(filepath.Join(badDir, "vm.json"), []byte("{bad"), 0640)

	// Create a regular file (not a dir)
	_ = os.WriteFile(filepath.Join(e.VMDir, "not-a-dir"), []byte("x"), 0640)

	vms, err := e.ListVMs()
	if err != nil {
		t.Fatalf("ListVMs: %v", err)
	}
	if len(vms) != 1 {
		t.Errorf("expected 1 VM, got %d", len(vms))
	}
	if vms[0].Name != "good" {
		t.Errorf("VM name: got %q, want good", vms[0].Name)
	}
}

func TestSetBootDev(t *testing.T) {
	e := testEngine(t)
	vm := &VM{ID: "boottest", Name: "boot-test", BootDev: "cdrom"}
	_ = os.MkdirAll(e.VMPath(vm.ID), 0750)
	_ = e.saveVM(vm)

	if err := e.SetBootDev("boot-test", "disk"); err != nil {
		t.Fatalf("SetBootDev: %v", err)
	}

	got, _ := e.GetVM("boottest")
	if got.BootDev != "disk" {
		t.Errorf("BootDev: got %q, want disk", got.BootDev)
	}
}

func TestSetBootDev_InvalidDev(t *testing.T) {
	e := testEngine(t)
	vm := &VM{ID: "boottest2", Name: "boot-test2"}
	_ = os.MkdirAll(e.VMPath(vm.ID), 0750)
	_ = e.saveVM(vm)

	if err := e.SetBootDev("boottest2", "floppy"); err == nil {
		t.Fatal("expected error for invalid boot device")
	}
}

func TestSetBootDev_NotFound(t *testing.T) {
	e := testEngine(t)
	if err := e.SetBootDev("nonexistent", "disk"); err == nil {
		t.Fatal("expected error for nonexistent VM")
	}
}

func TestVMPath(t *testing.T) {
	e := testEngine(t)
	got := e.VMPath("abc123")
	want := filepath.Join(e.VMDir, "abc123")
	if got != want {
		t.Errorf("VMPath: got %q, want %q", got, want)
	}
}
