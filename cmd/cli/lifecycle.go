package cli

import (
	"fmt"

	"github.com/c0m4r/v/engine"
)

func cmdStart(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v start <name|id>")
	}

	vm, err := e.GetVM(args[0])
	if err != nil {
		return err
	}

	if err := e.StartVM(vm.ID); err != nil {
		return err
	}

	fmt.Printf("Started VM %q\n", vm.Name)
	switch vm.GPU {
	case "virtio":
		fmt.Printf("  GPU: virtio (GTK window opened, fullscreen: Ctrl+Alt+F)\n")
	case "passthrough":
		fmt.Printf("  GPU: passthrough (%s) — output via physical display port\n", vm.PCIAddr)
	}
	if vm.BootDev == "cdrom" {
		fmt.Printf("  Booting from ISO: %s\n", vm.BaseImage)
		fmt.Printf("  After installation, switch to disk boot with: v set-boot %s disk\n", vm.Name)
	}
	return nil
}

func cmdSetGPU(e *engine.Engine, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: v set-gpu <name|id> <none|virtio|passthrough> [<pci-addr>]")
	}
	name, mode := args[0], args[1]
	pciAddr := ""
	if len(args) >= 3 {
		pciAddr = args[2]
	}
	if err := e.SetGPU(name, mode, pciAddr); err != nil {
		return err
	}
	if mode == "passthrough" {
		fmt.Printf("GPU mode set to %q (PCI: %s)\n", mode, pciAddr)
	} else {
		fmt.Printf("GPU mode set to %q\n", mode)
	}
	return nil
}

func cmdSetAudio(e *engine.Engine, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: v set-audio <name|id> <none|pipewire|pa|alsa>")
	}
	if err := e.SetAudio(args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf("Audio mode set to %q\n", args[1])
	return nil
}

func cmdSetBoot(e *engine.Engine, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: v set-boot <name|id> <disk|cdrom>")
	}
	if err := e.SetBootDev(args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf("Boot device set to %q\n", args[1])
	return nil
}

func cmdStop(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v stop <name|id>")
	}
	if err := e.StopVM(args[0]); err != nil {
		return err
	}
	fmt.Printf("Sent shutdown signal to VM %q\n", args[0])
	return nil
}

func cmdForceStop(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v force-stop <name|id>")
	}
	if err := e.ForceStopVM(args[0]); err != nil {
		return err
	}
	fmt.Printf("Force-stopped VM %q\n", args[0])
	return nil
}

func cmdRestart(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v restart <name|id>")
	}
	if err := e.RestartVM(args[0]); err != nil {
		return err
	}
	fmt.Printf("Restarted VM %q\n", args[0])
	return nil
}

func cmdDelete(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v delete <name|id>")
	}
	if err := e.DeleteVM(args[0]); err != nil {
		return err
	}
	fmt.Printf("Deleted VM %q\n", args[0])
	return nil
}
