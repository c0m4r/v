package engine

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	BridgeName = "v-br0"
	BridgeIP   = "10.10.10.1/24"
	BridgeNet  = "10.10.10.0/24"
	DHCPRange  = "10.10.10.50,10.10.10.200,12h"
)

// NetStatus holds network infrastructure status.
type NetStatus struct {
	BridgeExists bool   `json:"bridge_exists"`
	BridgeIP     string `json:"bridge_ip,omitempty"`
	DnsmasqPID   int    `json:"dnsmasq_pid,omitempty"`
	IPForward    bool   `json:"ip_forward"`
}

// SetupNetwork creates the bridge, configures NAT, and starts dnsmasq.
// Requires root privileges.
func (e *Engine) SetupNetwork() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("network setup requires root; re-run with: sudo v net setup")
	}

	// Create bridge
	log.Printf("creating bridge %s...", BridgeName)
	if err := run("ip", "link", "add", BridgeName, "type", "bridge"); err != nil {
		if !strings.Contains(err.Error(), "exists") {
			return fmt.Errorf("create bridge: %w", err)
		}
		log.Printf("bridge %s already exists, continuing", BridgeName)
	}

	if err := run("ip", "addr", "add", BridgeIP, "dev", BridgeName); err != nil {
		if !strings.Contains(err.Error(), "RTNETLINK answers: File exists") {
			return fmt.Errorf("assign bridge IP: %w", err)
		}
	}

	if err := run("ip", "link", "set", BridgeName, "up"); err != nil {
		return fmt.Errorf("bring up bridge: %w", err)
	}
	log.Printf("bridge %s up with IP %s", BridgeName, BridgeIP)

	// Enable IP forwarding
	log.Printf("enabling IP forwarding...")
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		return fmt.Errorf("enable IP forwarding: %w", err)
	}

	// Find default route interface for masquerade
	outIface, err := defaultInterface()
	if err != nil {
		return fmt.Errorf("find default interface: %w", err)
	}
	log.Printf("default interface: %s", outIface)

	// Set up NAT
	log.Printf("configuring iptables NAT and forwarding rules...")
	_ = run("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", BridgeNet, "-o", outIface, "-j", "MASQUERADE")
	if err := run("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", BridgeNet, "-o", outIface, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("setup NAT: %w", err)
	}

	// Allow forwarding for bridge traffic
	_ = run("iptables", "-D", "FORWARD", "-i", BridgeName, "-o", outIface, "-j", "ACCEPT")
	_ = run("iptables", "-D", "FORWARD", "-i", outIface, "-o", BridgeName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err := run("iptables", "-A", "FORWARD", "-i", BridgeName, "-o", outIface, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("setup forwarding: %w", err)
	}
	if err := run("iptables", "-A", "FORWARD", "-i", outIface, "-o", BridgeName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("setup return forwarding: %w", err)
	}

	// Start dnsmasq
	log.Printf("starting dnsmasq...")
	pidFile := filepath.Join(e.DataDir, "dnsmasq.pid")
	// Kill existing dnsmasq if running
	if data, err := os.ReadFile(pidFile); err == nil {
		_ = run("kill", strings.TrimSpace(string(data)))
	}

	leaseFile := filepath.Join(e.DataDir, "dnsmasq.leases")
	logFile := filepath.Join(e.DataDir, "dnsmasq.log")
	// Use Start() instead of CombinedOutput() so the daemonized child doesn't
	// keep the pipe open and block the parent.
	dnsCmd := exec.Command("dnsmasq",
		"--interface="+BridgeName,
		"--bind-interfaces",
		"--dhcp-range="+DHCPRange,
		"--dhcp-leasefile="+leaseFile,
		"--pid-file="+pidFile,
		"--log-facility="+logFile,
		"--no-resolv",
		"--server=8.8.8.8",
		"--server=1.1.1.1",
	)
	if err := dnsCmd.Start(); err != nil {
		return fmt.Errorf("start dnsmasq: %w", err)
	}
	log.Printf("dnsmasq started (log: %s)", logFile)

	return nil
}

// TeardownNetwork removes all v-tap devices, the bridge, NAT rules, and stops dnsmasq.
func (e *Engine) TeardownNetwork() error {
	e.CleanupTaps()

	pidFile := filepath.Join(e.DataDir, "dnsmasq.pid")
	if data, err := os.ReadFile(pidFile); err == nil {
		_ = run("kill", strings.TrimSpace(string(data)))
		_ = os.Remove(pidFile)
	}

	outIface, _ := defaultInterface()
	if outIface != "" {
		_ = run("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", BridgeNet, "-o", outIface, "-j", "MASQUERADE")
		_ = run("iptables", "-D", "FORWARD", "-i", BridgeName, "-o", outIface, "-j", "ACCEPT")
		_ = run("iptables", "-D", "FORWARD", "-i", outIface, "-o", BridgeName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	}

	_ = run("ip", "link", "set", BridgeName, "down")
	_ = run("ip", "link", "delete", BridgeName)

	return nil
}

// CleanupTaps removes all v-tap-* interfaces created by v.
func (e *Engine) CleanupTaps() {
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, iface := range ifaces {
		if strings.HasPrefix(iface.Name, "v-tap-") {
			_ = run("ip", "link", "delete", iface.Name)
		}
	}
}

// GetNetStatus returns the current status of the network infrastructure.
func (e *Engine) GetNetStatus() NetStatus {
	status := NetStatus{}

	iface, err := net.InterfaceByName(BridgeName)
	if err == nil {
		status.BridgeExists = true
		addrs, err := iface.Addrs()
		if err == nil && len(addrs) > 0 {
			status.BridgeIP = addrs[0].String()
		}
	}

	data, _ := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	status.IPForward = strings.TrimSpace(string(data)) == "1"

	return status
}

// CreateTap creates a tap device and attaches it to the bridge for a VM.
func (e *Engine) CreateTap(vmID string) (string, error) {
	tapName := fmt.Sprintf("v-tap-%s", vmID[:6])

	if err := run("ip", "tuntap", "add", tapName, "mode", "tap"); err != nil {
		if !strings.Contains(err.Error(), "exists") {
			return "", fmt.Errorf("create tap: %w", err)
		}
	}

	if err := run("ip", "link", "set", tapName, "master", BridgeName); err != nil {
		return "", fmt.Errorf("attach tap to bridge: %w", err)
	}

	if err := run("ip", "link", "set", tapName, "up"); err != nil {
		return "", fmt.Errorf("bring up tap: %w", err)
	}

	return tapName, nil
}

// DeleteTap removes a VM's tap device.
func (e *Engine) DeleteTap(vmID string) {
	tapName := fmt.Sprintf("v-tap-%s", vmID[:6])
	_ = run("ip", "link", "delete", tapName)
}

// VMIPAddress looks up the IP address of a running VM by its MAC address.
// Checks dnsmasq leases first, then falls back to the ARP table.
// Returns empty string if not found (e.g. user-mode networking).
func (e *Engine) VMIPAddress(vm *VM) string {
	if vm.MACAddr == "" {
		return ""
	}
	mac := strings.ToLower(vm.MACAddr)

	// Check dnsmasq leases file
	leasePath := filepath.Join(e.DataDir, "dnsmasq.leases")
	if data, err := os.ReadFile(leasePath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 3 && strings.ToLower(fields[1]) == mac {
				return fields[2]
			}
		}
	}

	// Fall back to ARP table
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.ToLower(fields[3]) == mac {
			return fields[0]
		}
	}

	return ""
}

func defaultInterface() (string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", fmt.Errorf("no default route found")
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %s: %w", name, strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}
