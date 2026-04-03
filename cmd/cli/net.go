package cli

import (
	"fmt"

	"github.com/c0m4r/v/engine"
)

func cmdNet(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v net <setup|teardown|status>")
	}

	switch args[0] {
	case "setup":
		if err := e.SetupNetwork(); err != nil {
			return err
		}
		fmt.Println("Network bridge configured successfully")
		fmt.Printf("  Bridge: %s (%s)\n", engine.BridgeName, engine.BridgeIP)
		fmt.Printf("  DHCP range: %s\n", engine.DHCPRange)
		return nil

	case "teardown":
		if err := e.TeardownNetwork(); err != nil {
			return err
		}
		fmt.Println("Network bridge removed")
		return nil

	case "status":
		status := e.GetNetStatus()
		if status.BridgeExists {
			fmt.Printf("Bridge: %s (IP: %s)\n", engine.BridgeName, status.BridgeIP)
		} else {
			fmt.Printf("Bridge: not configured\n")
		}
		fmt.Printf("IP forwarding: %v\n", status.IPForward)
		return nil

	default:
		return fmt.Errorf("unknown net command: %s", args[0])
	}
}
