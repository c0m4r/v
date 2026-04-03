package cli

import (
	"fmt"

	"github.com/c0m4r/v/engine"
)

func cmdConfig(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return cmdConfigShow(e)
	}

	switch args[0] {
	case "show":
		return cmdConfigShow(e)
	case "set":
		return cmdConfigSet(e, args[1:])
	default:
		return fmt.Errorf("usage: v config [show|set <key> <value>]\n\nKeys:\n  ssh-key    Default SSH public key for new VMs")
	}
}

func cmdConfigShow(e *engine.Engine) error {
	cfg, err := e.LoadConfig()
	if err != nil {
		return err
	}

	sshKey := "(not set)"
	if cfg.DefaultSSHKey != "" {
		// Show truncated key
		k := cfg.DefaultSSHKey
		if len(k) > 60 {
			k = k[:57] + "..."
		}
		sshKey = k
	}
	fmt.Printf("Default SSH key:  %s\n", sshKey)
	return nil
}

func cmdConfigSet(e *engine.Engine, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: v config set <key> <value>")
	}

	cfg, err := e.LoadConfig()
	if err != nil {
		return err
	}

	key, value := args[0], args[1]

	switch key {
	case "ssh-key":
		cfg.DefaultSSHKey = resolveSSHKey(value)
		fmt.Printf("Default SSH key set\n")
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	return e.SaveConfig(cfg)
}
