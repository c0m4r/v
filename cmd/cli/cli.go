package cli

import (
	"fmt"

	"github.com/c0m4r/v/engine"
)

// Run dispatches CLI subcommands to their handlers.
func Run(e *engine.Engine, cmd string, args []string) error {
	switch cmd {
	case "create":
		return cmdCreate(e, args)
	case "list", "ls":
		return cmdList(e, args)
	case "info":
		return cmdInfo(e, args)
	case "start":
		return cmdStart(e, args)
	case "stop":
		return cmdStop(e, args)
	case "force-stop":
		return cmdForceStop(e, args)
	case "restart":
		return cmdRestart(e, args)
	case "delete", "rm":
		return cmdDelete(e, args)
	case "console":
		return cmdConsole(e, args)
	case "image":
		return cmdImage(e, args)
	case "disk":
		return cmdDisk(e, args)
	case "net":
		return cmdNet(e, args)
	case "set-boot":
		return cmdSetBoot(e, args)
	case "config":
		return cmdConfig(e, args)
	default:
		return fmt.Errorf("unknown command: %s (try 'v help')", cmd)
	}
}
