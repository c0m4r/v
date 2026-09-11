package engine

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestTeardownNetworkRequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires an unprivileged user")
	}
	if err := testEngine(t).TeardownNetwork(); err == nil || !strings.Contains(err.Error(), "requires root") {
		t.Fatalf("expected root requirement, got %v", err)
	}
}

func TestTeardownNetworkReportsFailuresAndContinues(t *testing.T) {
	e := testEngine(t)
	pidFile := filepath.Join(e.DataDir, "dnsmasq.pid")
	if err := os.WriteFile(pidFile, []byte("12345\n"), 0640); err != nil {
		t.Fatal(err)
	}
	tapErr := errors.New("tap permission denied")
	bridgeErr := errors.New("bridge permission denied")
	killErr := errors.New("signal permission denied")
	iptablesErr := errors.New("iptables unavailable")
	var commands []string
	err := e.teardownNetwork(
		func() ([]net.Interface, error) {
			return []net.Interface{{Name: "lo"}, {Name: "v-tap-test"}, {Name: BridgeName}}, nil
		},
		func(name string, args ...string) ([]byte, error) {
			cmd := name + " " + strings.Join(args, " ")
			commands = append(commands, cmd)
			switch cmd {
			case "ip link delete v-tap-test":
				return nil, tapErr
			case "ip link delete v-br0":
				return nil, bridgeErr
			case "kill 12345":
				return nil, killErr
			default:
				return nil, iptablesErr
			}
		},
	)
	for _, wantErr := range []error{tapErr, bridgeErr, killErr, iptablesErr} {
		if !errors.Is(err, wantErr) {
			t.Errorf("cleanup error %v does not include %v", err, wantErr)
		}
	}
	if !slices.Contains(commands, "ip link delete v-br0") {
		t.Fatal("bridge deletion was skipped after an earlier failure")
	}
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatalf("PID file must be retained when stopping dnsmasq fails: %v", err)
	}
}

func TestTeardownNetworkRemovesInstalledRulesWithoutDefaultRoute(t *testing.T) {
	e := testEngine(t)
	natRule := "-A POSTROUTING -s 10.10.10.0/24 -o old-uplink -j MASQUERADE"
	forwardRule := "-A FORWARD -i v-br0 -o old-uplink -j ACCEPT"
	returnRule := "-A FORWARD -i old-uplink -o v-br0 -m state --state RELATED,ESTABLISHED -j ACCEPT"
	var deleted []string
	err := e.teardownNetwork(
		func() ([]net.Interface, error) { return []net.Interface{{Name: BridgeName}}, nil },
		func(name string, args ...string) ([]byte, error) {
			cmd := name + " " + strings.Join(args, " ")
			switch cmd {
			case "iptables -t nat -S POSTROUTING":
				return []byte(strings.Join([]string{
					"-P POSTROUTING ACCEPT", natRule, natRule,
					"-A POSTROUTING -s 172.17.0.0/16 -o old-uplink -j MASQUERADE",
				}, "\n")), nil
			case "iptables -t filter -S FORWARD":
				return []byte(strings.Join([]string{
					forwardRule, returnRule,
					"-A FORWARD -i docker0 -o old-uplink -j ACCEPT",
					forwardRule + " -m comment --comment custom",
				}, "\n")), nil
			default:
				deleted = append(deleted, cmd)
				return nil, nil
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"iptables -t nat " + strings.Replace(natRule, "-A", "-D", 1),
		"iptables -t nat " + strings.Replace(natRule, "-A", "-D", 1),
		"iptables -t filter " + strings.Replace(forwardRule, "-A", "-D", 1),
		"iptables -t filter " + strings.Replace(returnRule, "-A", "-D", 1),
		"ip link delete v-br0",
	}
	if !slices.Equal(deleted, want) {
		t.Fatalf("deleted commands: got %v, want %v", deleted, want)
	}
}

func TestTeardownNetworkAlreadyRemoved(t *testing.T) {
	err := testEngine(t).teardownNetwork(
		func() ([]net.Interface, error) { return []net.Interface{{Name: "lo"}}, nil },
		func(name string, args ...string) ([]byte, error) {
			if name != "iptables" || !slices.Contains(args, "-S") {
				t.Errorf("unexpected cleanup command: %s %v", name, args)
			}
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTeardownNetworkReportsInterfaceDiscoveryFailure(t *testing.T) {
	wantErr := errors.New("netlink permission denied")
	err := testEngine(t).teardownNetwork(
		func() ([]net.Interface, error) { return nil, wantErr },
		func(string, ...string) ([]byte, error) { return nil, nil },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error: got %v, want wrapped %v", err, wantErr)
	}
}

func TestTeardownNetworkRemovesDnsmasqPID(t *testing.T) {
	for _, killErr := range []error{nil, errors.New("kill: No such process")} {
		e := testEngine(t)
		pidFile := filepath.Join(e.DataDir, "dnsmasq.pid")
		if err := os.WriteFile(pidFile, []byte("12345\n"), 0640); err != nil {
			t.Fatal(err)
		}
		err := e.teardownNetwork(
			func() ([]net.Interface, error) { return nil, nil },
			func(name string, args ...string) ([]byte, error) {
				if name == "kill" {
					return nil, killErr
				}
				return nil, nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
			t.Fatalf("PID file should be removed, got %v", err)
		}
	}
}

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
