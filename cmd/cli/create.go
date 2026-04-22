package cli

import (
	"flag"
	"fmt"

	"github.com/c0m4r/v/engine"
)

func cmdCreate(e *engine.Engine, args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	name := fs.String("name", "", "VM name (required)")
	image := fs.String("image", "", "Base image name (required, e.g. ubuntu-24.04)")
	cpus := fs.Int("cpus", 1, "Number of vCPUs")
	memory := fs.Int("memory", 512, "Memory in MB")
	disk := fs.String("disk", "10G", "Disk size (e.g. 10G, 20G)")
	netMode := fs.String("net", "user", "Network mode: user or bridge")
	gpu := fs.String("gpu", "none", "GPU mode: none (headless), virtio, or passthrough")
	pciAddr := fs.String("pci-addr", "", "Host PCI address for passthrough mode (e.g. 01:00.0)")
	audio := fs.String("audio", "none", "Audio backend: none, pipewire, pa, or alsa")
	sshKey := fs.String("ssh-key", "", "Public SSH key string or path to .pub file")
	password := fs.String("password", "", `Root password (leave blank to auto-generate, "none" for no password)`)
	userData := fs.String("user-data", "", "Path to cloud-init user-data file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" || *image == "" {
		fmt.Println("Usage: v create --name NAME --image IMAGE [options]")
		fs.PrintDefaults()
		return fmt.Errorf("--name and --image are required")
	}

	// Resolve image name to filename
	imageName := resolveImageName(e, *image)

	var userDataContent string
	if *userData != "" {
		data, err := readFileArg(*userData)
		if err != nil {
			return fmt.Errorf("read user-data: %w", err)
		}
		userDataContent = string(data)
	}

	sshKeyContent := resolveSSHKey(*sshKey)

	vm, err := e.CreateVM(engine.CreateVMOpts{
		Name:         *name,
		CPUs:         *cpus,
		MemoryMB:     *memory,
		DiskSize:     *disk,
		Image:        imageName,
		NetMode:      *netMode,
		GPU:          *gpu,
		PCIAddr:      *pciAddr,
		Audio:        *audio,
		SSHKey:       sshKeyContent,
		RootPassword: *password,
		UserData:     userDataContent,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Created VM %q (id: %s)\n", vm.Name, vm.ID)
	fmt.Printf("  CPUs: %d, Memory: %d MB, Disk: %s\n", vm.CPUs, vm.MemoryMB, vm.DiskSize)
	fmt.Printf("  Image: %s, Net: %s\n", vm.BaseImage, vm.NetMode)
	if vm.BootDev == "cdrom" {
		fmt.Printf("  Boot: ISO (will boot from %s)\n", vm.BaseImage)
	}
	if vm.RootPassword != "" {
		fmt.Printf("  Root password: %s\n", vm.RootPassword)
	} else {
		fmt.Printf("  Root password: (none — use SSH key)\n")
	}
	fmt.Printf("\nStart with: v start %s\n", vm.Name)
	return nil
}

// resolveImageName maps a short image name to the cached filename.
func resolveImageName(e *engine.Engine, name string) string {
	if url, ok := e.KnownImages()[name]; ok {
		parts := splitLast(url, "/")
		return parts
	}
	return name
}

func splitLast(s, sep string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if string(s[i]) == sep {
			return s[i+1:]
		}
	}
	return s
}
