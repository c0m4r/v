package cli

import (
	"fmt"
	"text/tabwriter"
	"os"

	"github.com/c0m4r/v/engine"
)

func cmdList(e *engine.Engine, _ []string) error {
	vms, err := e.ListVMs()
	if err != nil {
		return err
	}

	if len(vms) == 0 {
		fmt.Println("No VMs found. Create one with: v create --name NAME --image IMAGE")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCPUS\tMEMORY\tDISK\tNET\tIP/SSH\tSTATE")

	for _, vm := range vms {
		state, _ := e.VMState(vm.ID)
		access := "-"
		if state == engine.StateRunning {
			if addr := e.VMIPAddress(vm); addr != "" {
				access = addr
			} else if vm.SSHPort > 0 {
				access = fmt.Sprintf("localhost:%d", vm.SSHPort)
			}
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d MB\t%s\t%s\t%s\t%s\n",
			vm.ID, vm.Name, vm.CPUs, vm.MemoryMB, vm.DiskSize, vm.NetMode, access, state)
	}

	return w.Flush()
}

func cmdInfo(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v info <name|id>")
	}

	vm, err := e.GetVM(args[0])
	if err != nil {
		return err
	}

	state, _ := e.VMState(vm.ID)
	ip := "-"
	if state == engine.StateRunning {
		if addr := e.VMIPAddress(vm); addr != "" {
			ip = addr
		}
	}

	fmt.Printf("ID:        %s\n", vm.ID)
	fmt.Printf("Name:      %s\n", vm.Name)
	fmt.Printf("State:     %s\n", state)
	fmt.Printf("IP:        %s\n", ip)
	if vm.SSHPort > 0 {
		fmt.Printf("SSH:       ssh -p %d localhost\n", vm.SSHPort)
	}
	fmt.Printf("CPUs:      %d\n", vm.CPUs)
	fmt.Printf("Memory:    %d MB\n", vm.MemoryMB)
	fmt.Printf("Disk:      %s\n", vm.DiskSize)
	fmt.Printf("Image:     %s\n", vm.BaseImage)
	fmt.Printf("Boot:      %s\n", vm.BootDev)
	fmt.Printf("Network:   %s\n", vm.NetMode)
	gpu := vm.GPU
	if gpu == "" {
		gpu = "none"
	}
	if vm.PCIAddr != "" {
		fmt.Printf("GPU:       %s (%s)\n", gpu, vm.PCIAddr)
	} else {
		fmt.Printf("GPU:       %s\n", gpu)
	}
	audio := vm.Audio
	if audio == "" {
		audio = "none"
	}
	fmt.Printf("Audio:     %s\n", audio)
	fmt.Printf("MAC:       %s\n", vm.MACAddr)
	fmt.Printf("Created:   %s\n", vm.CreatedAt.Format("2006-01-02 15:04:05"))

	return nil
}
