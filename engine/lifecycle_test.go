package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVMState_NoPIDFile(t *testing.T) {
	e := testEngine(t)
	// Create a VM directory without a PID file
	_ = os.MkdirAll(e.VMPath("test-vm"), 0750)

	state, err := e.VMState("test-vm")
	if err != nil {
		t.Fatalf("VMState: %v", err)
	}
	if state != StateStopped {
		t.Errorf("state: got %q, want %q", state, StateStopped)
	}
}

func TestVMState_StalePID(t *testing.T) {
	e := testEngine(t)
	vmDir := e.VMPath("stale-vm")
	_ = os.MkdirAll(vmDir, 0750)

	// Write a PID that definitely doesn't exist
	_ = os.WriteFile(filepath.Join(vmDir, "pid"), []byte("999999999"), 0640)

	state, err := e.VMState("stale-vm")
	if err != nil {
		t.Fatalf("VMState: %v", err)
	}
	if state != StateStopped {
		t.Errorf("state: got %q, want %q (stale PID should be detected)", state, StateStopped)
	}

	// Stale PID file should be cleaned up
	if _, err := os.Stat(filepath.Join(vmDir, "pid")); !os.IsNotExist(err) {
		t.Error("stale PID file should have been removed")
	}
}

func TestVMState_InvalidPID(t *testing.T) {
	e := testEngine(t)
	vmDir := e.VMPath("badpid-vm")
	_ = os.MkdirAll(vmDir, 0750)

	_ = os.WriteFile(filepath.Join(vmDir, "pid"), []byte("not-a-number"), 0640)

	state, err := e.VMState("badpid-vm")
	if err != nil {
		t.Fatalf("VMState: %v", err)
	}
	if state != StateStopped {
		t.Errorf("state: got %q, want %q", state, StateStopped)
	}
}

func TestConsoleSocketPath_VMNotFound(t *testing.T) {
	e := testEngine(t)
	_, err := e.ConsoleSocketPath("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent VM")
	}
}

func TestConsoleSocketPath_VMStopped(t *testing.T) {
	e := testEngine(t)
	vm := &VM{ID: "stopped-vm", Name: "stopped"}
	_ = os.MkdirAll(e.VMPath(vm.ID), 0750)
	_ = e.saveVM(vm)

	_, err := e.ConsoleSocketPath("stopped-vm")
	if err == nil {
		t.Fatal("expected error for stopped VM")
	}
}
