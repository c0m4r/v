package main

import (
	"fmt"
	"os"

	"github.com/c0m4r/v/cmd/cli"
	"github.com/c0m4r/v/cmd/web"
	"github.com/c0m4r/v/engine"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

const usage = `v - lightweight KVM virtualization manager

Usage:
  v <command> [options]

Commands:
  create      Create a new VM
  list        List all VMs
  info        Show VM details
  start       Start a VM (--graphical / -g for a display window)
  stop        Gracefully stop a VM (ACPI shutdown)
  force-stop  Force stop a VM
  restart     Restart a VM
  delete      Delete a VM
  console     Attach to VM serial console

  image pull  Download a cloud image
  image list  List cached images

  disk create Create a standalone disk image

  config         Show configuration
  config set     Set a config value (e.g. v config set ssh-key ~/.ssh/id_ed25519.pub)

  serve       Start the web UI server

  net setup      Set up bridge networking (requires root)
  net teardown   Tear down bridge networking (requires root)
  net status     Show network status
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	e, err := engine.New(engine.DefaultDataDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "serve":
		if err := web.Serve(e, args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "version", "--version", "-v":
		fmt.Printf("v %s (built %s)\n", version, buildTime)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		if err := cli.Run(e, cmd, args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
}
