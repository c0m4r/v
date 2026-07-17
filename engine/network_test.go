package engine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

func TestRecoverMissingTapsRestartsOnlyAffectedBridgeVMs(t *testing.T) {
	e := testEngine(t)
	for _, vm := range []*VM{
		{ID: "missing1", Name: "missing", NetMode: "bridge"},
		{ID: "healthy1", Name: "healthy", NetMode: "bridge"},
		{ID: "usermode", Name: "user", NetMode: "user"},
		{ID: "stopped1", Name: "stopped", NetMode: "bridge"},
	} {
		if err := os.MkdirAll(e.VMPath(vm.ID), 0750); err != nil {
			t.Fatalf("create VM directory: %v", err)
		}
		if err := e.saveVM(vm); err != nil {
			t.Fatalf("save VM %q: %v", vm.Name, err)
		}
		if vm.Name != "stopped" {
			pid := []byte(processID())
			if err := os.WriteFile(filepath.Join(e.VMPath(vm.ID), "pid"), pid, 0640); err != nil {
				t.Fatalf("write PID for VM %q: %v", vm.Name, err)
			}
		}
	}

	var restarted []string
	recovered, err := e.recoverMissingTaps(
		func(name string) bool { return name == "v-tap-health" },
		func(id string) error {
			restarted = append(restarted, id)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("recoverMissingTaps: %v", err)
	}
	if !slices.Equal(restarted, []string{"missing1"}) {
		t.Fatalf("restarted VMs: got %v, want [missing1]", restarted)
	}
	if !slices.Equal(recovered, []string{"missing"}) {
		t.Fatalf("recovered VMs: got %v, want [missing]", recovered)
	}
}

func TestRecoverMissingTapsReportsRestartFailure(t *testing.T) {
	e := testEngine(t)
	vm := &VM{ID: "missing1", Name: "missing", NetMode: "bridge"}
	if err := os.MkdirAll(e.VMPath(vm.ID), 0750); err != nil {
		t.Fatalf("create VM directory: %v", err)
	}
	if err := e.saveVM(vm); err != nil {
		t.Fatalf("save VM: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.VMPath(vm.ID), "pid"), []byte(processID()), 0640); err != nil {
		t.Fatalf("write PID: %v", err)
	}

	wantErr := errors.New("restart failed")
	recovered, err := e.recoverMissingTaps(
		func(string) bool { return false },
		func(string) error { return wantErr },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error: got %v, want wrapped %v", err, wantErr)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered VMs: got %v, want none", recovered)
	}
}

func TestTapNameHandlesShortIDs(t *testing.T) {
	if got := tapName("abc"); got != "v-tap-abc" {
		t.Fatalf("tapName: got %q, want %q", got, "v-tap-abc")
	}
}

func processID() string {
	return strconv.Itoa(os.Getpid())
}
