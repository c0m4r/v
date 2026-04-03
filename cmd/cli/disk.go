package cli

import (
	"flag"
	"fmt"

	"github.com/c0m4r/v/engine"
)

func cmdDisk(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v disk create --path PATH --size SIZE")
	}

	switch args[0] {
	case "create":
		return cmdDiskCreate(e, args[1:])
	default:
		return fmt.Errorf("unknown disk command: %s", args[0])
	}
}

func cmdDiskCreate(e *engine.Engine, args []string) error {
	fs := flag.NewFlagSet("disk create", flag.ExitOnError)
	path := fs.String("path", "", "Output path for the disk image (required)")
	size := fs.String("size", "10G", "Disk size (e.g. 10G, 50G)")
	backing := fs.String("backing", "", "Backing file for thin clone (optional)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *path == "" {
		return fmt.Errorf("--path is required")
	}

	if err := e.CreateDisk(*path, *size, *backing); err != nil {
		return err
	}

	fmt.Printf("Created disk: %s (%s)\n", *path, *size)
	return nil
}
