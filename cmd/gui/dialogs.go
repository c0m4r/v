//go:build cgo
// +build cgo

package gui

import (
	"fmt"
	"os"
	"strings"

	"github.com/c0m4r/v/engine"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func (app *App) showCreateDialog() {
	dlg, err := gtk.DialogNew()
	if err != nil {
		app.showError("Error", "Failed to create dialog: "+err.Error())
		return
	}
	defer dlg.Destroy()
	dlg.SetTitle("Create New VM")
	dlg.SetTransientFor(app.win)
	dlg.SetModal(true)
	_, _ = dlg.AddButton("Cancel", gtk.RESPONSE_CANCEL)
	_, _ = dlg.AddButton("Create VM", gtk.RESPONSE_OK)
	dlg.SetDefaultSize(500, 500)

	content, err := dlg.GetContentArea()
	if err != nil {
		app.showError("Error", "Failed to get content area: "+err.Error())
		return
	}
	content.SetSpacing(8)
	content.SetMarginStart(16)
	content.SetMarginEnd(16)
	content.SetMarginTop(12)
	content.SetMarginBottom(12)

	grid, err := gtk.GridNew()
	if err != nil {
		app.showError("Error", "Failed to create grid: "+err.Error())
		return
	}
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(12)
	grid.SetHExpand(true)
	content.PackStart(grid, true, true, 0)

	row := 0
	addRow := func(labelText string, widget gtk.IWidget) {
		lbl, _ := gtk.LabelNew(labelText + ":")
		lbl.SetHAlign(gtk.ALIGN_END)
		grid.Attach(lbl, 0, row, 1, 1)
		widget.ToWidget().SetHExpand(true)
		grid.Attach(widget.ToWidget(), 1, row, 1, 1)
		row++
	}

	nameEntry, _ := gtk.EntryNew()
	nameEntry.SetPlaceholderText("my-vm")
	addRow("Name", nameEntry)

	imageCombo, err := gtk.ComboBoxTextNew()
	if err != nil {
		app.showError("Error", err.Error())
		return
	}
	images, err := app.engine.ListImages()
	if err == nil {
		cached := make(map[string]bool)
		for _, img := range images {
			cached[img.Name] = true
			imageCombo.Append(img.Name, img.Name)
			if strings.HasSuffix(img.Name, ".qcow2") || strings.HasSuffix(img.Name, ".img") {
				imageCombo.SetActive(len(cached) - 1)
			}
		}
		if len(images) == 0 {
			imageCombo.Append("", "-- No cached images. Pull one first --")
			imageCombo.SetActive(0)
			imageCombo.SetSensitive(false)
		}
		known := app.engine.KnownImages()
		for name := range known {
			if !cached[name] {
				imageCombo.Append(name, name)
			}
		}
	}
	addRow("Image", imageCombo)

	cpuSpin, err := gtk.SpinButtonNewWithRange(1, 256, 1)
	if err != nil {
		app.showError("Error", err.Error())
		return
	}
	cpuSpin.SetValue(4)
	cpuSpin.SetDigits(0)
	addRow("CPUs", cpuSpin)

	memSpin, err := gtk.SpinButtonNewWithRange(128, float64(maxMemoryMB()), 128)
	if err != nil {
		app.showError("Error", err.Error())
		return
	}
	memSpin.SetValue(2048)
	memSpin.SetDigits(0)
	addRow("Memory (MB)", memSpin)

	diskEntry, _ := gtk.EntryNew()
	diskEntry.SetText("10G")
	diskEntry.SetPlaceholderText("10G")
	addRow("Disk Size", diskEntry)

	netCombo, err := gtk.ComboBoxTextNew()
	if err != nil {
		app.showError("Error", err.Error())
		return
	}
	netCombo.Append("user", "User-mode (NAT)")
	netCombo.Append("bridge", "Bridge (needs root)")
	netCombo.SetActive(0)
	if os.Getuid() != 0 {
		netStatus := app.engine.GetNetStatus()
		if !netStatus.BridgeExists {
			netCombo.SetSensitive(false)
			netCombo.SetActive(0)
		}
	}
	addRow("Network", netCombo)

	gpuCombo, err := gtk.ComboBoxTextNew()
	if err != nil {
		app.showError("Error", err.Error())
		return
	}
	gpuCombo.Append("none", "None (headless)")
	gpuCombo.Append("virtio", "Virtio (virtio-vga)")
	gpuCombo.Append("passthrough", "Passthrough (VFIO)")
	gpuCombo.SetActive(0)
	addRow("GPU", gpuCombo)

	pciEntry, _ := gtk.EntryNew()
	pciEntry.SetPlaceholderText("01:00.0")
	pciEntry.SetSensitive(false)
	addRow("PCI Address", pciEntry)

	gpuCombo.Connect("changed", func() {
		pciEntry.SetSensitive(gpuCombo.GetActiveText() == "passthrough")
	})

	audioCombo, err := gtk.ComboBoxTextNew()
	if err != nil {
		app.showError("Error", err.Error())
		return
	}
	audioCombo.Append("none", "None")
	audioCombo.Append("pa", "PulseAudio")
	audioCombo.Append("pipewire", "PipeWire")
	audioCombo.Append("alsa", "ALSA")
	audioCombo.SetActive(0)
	addRow("Audio", audioCombo)

	sshKeyEntry, _ := gtk.EntryNew()
	sshKeyEntry.SetPlaceholderText("Optional SSH public key")
	cfg, err := app.engine.LoadConfig()
	if err == nil && cfg.DefaultSSHKey != "" {
		sshKeyEntry.SetPlaceholderText("Default key set in settings")
	}
	addRow("SSH Key", sshKeyEntry)

	passwordLabel, _ := gtk.LabelNew("Root Password:")
	passwordLabel.SetHAlign(gtk.ALIGN_END)
	grid.Attach(passwordLabel, 0, row, 1, 1)

	pwdBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	pwdBox.SetHExpand(true)
	grid.Attach(pwdBox, 1, row, 1, 1)

	pwdEntry, _ := gtk.EntryNew()
	pwdEntry.SetVisibility(false)
	pwdEntry.SetText(engine.GeneratePassword())
	pwdEntry.SetHExpand(true)
	pwdBox.PackStart(pwdEntry, true, true, 0)

	showPwdCheck, _ := gtk.CheckButtonNewWithLabel("Show")
	showPwdCheck.Connect("toggled", func() {
		pwdEntry.SetVisibility(showPwdCheck.GetActive())
	})
	pwdBox.PackStart(showPwdCheck, false, false, 0)

	genPwdBtn, _ := gtk.ButtonNewWithLabel("\u21BA")
	genPwdBtn.Connect("clicked", func() {
		pwdEntry.SetText(engine.GeneratePassword())
	})
	pwdBox.PackStart(genPwdBtn, false, false, 0)

	noPwdCheck, _ := gtk.CheckButtonNewWithLabel("No password")
	noPwdCheck.Connect("toggled", func() {
		sensitive := !noPwdCheck.GetActive()
		pwdEntry.SetSensitive(sensitive)
		showPwdCheck.SetSensitive(sensitive)
		genPwdBtn.SetSensitive(sensitive)
	})
	pwdBox.PackStart(noPwdCheck, false, false, 0)
	row++

	dlg.ShowAll()

	if dlg.Run() != gtk.RESPONSE_OK {
		return
	}

	imageText := imageCombo.GetActiveText()
	if imageText == "" {
		app.showError("Validation", "Please select an image")
		return
	}

	netMode := "user"
	if netCombo.GetActiveText() == "bridge" {
		netMode = "bridge"
	}

	pwdText, _ := pwdEntry.GetText()
	password := pwdText
	if noPwdCheck.GetActive() {
		password = "none"
	}

	nameText, _ := nameEntry.GetText()
	diskText, _ := diskEntry.GetText()
	sshText, _ := sshKeyEntry.GetText()
	pciText, _ := pciEntry.GetText()

	opts := engine.CreateVMOpts{
		Name:     strings.TrimSpace(nameText),
		CPUs:     cpuSpin.GetValueAsInt(),
		MemoryMB: memSpin.GetValueAsInt(),
		DiskSize: strings.TrimSpace(diskText),
		Image:    imageText,
		NetMode:  netMode,
		GPU:      gpuCombo.GetActiveText(),
		Audio:    audioCombo.GetActiveText(),
		SSHKey:   strings.TrimSpace(sshText),
	}

	if opts.GPU == "passthrough" {
		opts.PCIAddr = strings.TrimSpace(pciText)
	}

	if password != "none" {
		opts.RootPassword = password
	}

	go func() {
		vm, err := app.engine.CreateVM(opts)
		if err != nil {
			glib.IdleAdd(func() {
				app.showError("Create VM", err.Error())
			})
			return
		}
		glib.IdleAdd(func() {
			app.refreshVMList()
			msg := fmt.Sprintf("VM <b>%s</b> created successfully.\n\nRoot password: <b>%s</b>",
				vm.Name, vm.RootPassword)
			if vm.RootPassword == "" {
				msg = fmt.Sprintf("VM <b>%s</b> created successfully.", vm.Name)
			} else if vm.SSHPort > 0 && vm.NetMode == "user" {
				msg += fmt.Sprintf("\nSSH: ssh -p %d root@localhost", vm.SSHPort)
			}
			dlg := gtk.MessageDialogNewWithMarkup(app.win, gtk.DIALOG_MODAL, gtk.MESSAGE_INFO, gtk.BUTTONS_OK, "%s", msg)
			dlg.Run()
			dlg.Destroy()
		})
	}()
}

func (app *App) showVMSettingsDialog(vm *engine.VM) {
	dlg, err := gtk.DialogNew()
	if err != nil {
		app.showError("Error", "Failed to create dialog: "+err.Error())
		return
	}
	defer dlg.Destroy()
	dlg.SetTitle("VM Settings: " + vm.Name)
	dlg.SetTransientFor(app.win)
	dlg.SetModal(true)
	_, _ = dlg.AddButton("Cancel", gtk.RESPONSE_CANCEL)
	_, _ = dlg.AddButton("Save", gtk.RESPONSE_OK)
	dlg.SetDefaultSize(400, 350)

	content, err := dlg.GetContentArea()
	if err != nil {
		return
	}
	content.SetSpacing(8)
	content.SetMarginStart(16)
	content.SetMarginEnd(16)
	content.SetMarginTop(12)
	content.SetMarginBottom(12)

	grid, _ := gtk.GridNew()
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(12)
	grid.SetHExpand(true)
	content.PackStart(grid, true, true, 0)

	row := 0
	addRow := func(labelText string, widget gtk.IWidget) {
		lbl, _ := gtk.LabelNew(labelText + ":")
		lbl.SetHAlign(gtk.ALIGN_END)
		grid.Attach(lbl, 0, row, 1, 1)
		widget.ToWidget().SetHExpand(true)
		grid.Attach(widget.ToWidget(), 1, row, 1, 1)
		row++
	}

	cpuSpin, _ := gtk.SpinButtonNewWithRange(1, 256, 1)
	cpuSpin.SetValue(float64(vm.CPUs))
	addRow("CPUs", cpuSpin)

	memSpin, _ := gtk.SpinButtonNewWithRange(128, float64(maxMemoryMB()), 128)
	memSpin.SetValue(float64(vm.MemoryMB))
	addRow("Memory (MB)", memSpin)

	gpuCombo, _ := gtk.ComboBoxTextNew()
	gpuCombo.Append("none", "None (headless)")
	gpuCombo.Append("virtio", "Virtio (virtio-vga)")
	gpuCombo.Append("passthrough", "Passthrough (VFIO)")
	if vm.GPU == "" {
		gpuCombo.SetActive(0)
	} else {
		gpuCombo.SetActiveID(vm.GPU)
	}
	addRow("GPU", gpuCombo)

	pciEntry, _ := gtk.EntryNew()
	pciEntry.SetText(vm.PCIAddr)
	pciEntry.SetPlaceholderText("01:00.0")
	pciEntry.SetSensitive(vm.GPU == "passthrough")
	addRow("PCI Address", pciEntry)

	gpuCombo.Connect("changed", func() {
		pciEntry.SetSensitive(gpuCombo.GetActiveText() == "passthrough")
	})

	audioCombo, _ := gtk.ComboBoxTextNew()
	audioCombo.Append("none", "None")
	audioCombo.Append("pa", "PulseAudio")
	audioCombo.Append("pipewire", "PipeWire")
	audioCombo.Append("alsa", "ALSA")
	if vm.Audio == "" {
		audioCombo.SetActive(0)
	} else {
		audioCombo.SetActiveID(vm.Audio)
	}
	addRow("Audio", audioCombo)

	bootCombo, _ := gtk.ComboBoxTextNew()
	bootCombo.Append("disk", "Disk")
	bootCombo.Append("cdrom", "CD-ROM (ISO)")
	if vm.BootDev == "cdrom" {
		bootCombo.SetActive(1)
	} else {
		bootCombo.SetActive(0)
	}
	addRow("Boot Device", bootCombo)

	dlg.ShowAll()

	if dlg.Run() != gtk.RESPONSE_OK {
		return
	}

	pciText, _ := pciEntry.GetText()

	opts := engine.UpdateVMOpts{
		CPUs:     cpuSpin.GetValueAsInt(),
		MemoryMB: memSpin.GetValueAsInt(),
		GPU:      gpuCombo.GetActiveText(),
		PCIAddr:  strings.TrimSpace(pciText),
		Audio:    audioCombo.GetActiveText(),
		BootDev:  bootCombo.GetActiveText(),
	}

	go func() {
		if _, err := app.engine.UpdateVM(vm.ID, opts); err != nil {
			glib.IdleAdd(func() {
				app.showError("Update VM", err.Error())
			})
			return
		}
		glib.IdleAdd(func() {
			app.refreshVMList()
		})
	}()
}

func (app *App) showPasswordDialog(vm *engine.VM) {
	if vm.RootPassword == "" {
		app.showError("Password", "No password set for this VM.")
		return
	}

	dlg, err := gtk.DialogNew()
	if err != nil {
		app.showError("Error", "Failed to create dialog: "+err.Error())
		return
	}
	defer dlg.Destroy()
	dlg.SetTitle("VM Password: " + vm.Name)
	dlg.SetTransientFor(app.win)
	dlg.SetModal(true)
	_, _ = dlg.AddButton("Close", gtk.RESPONSE_CLOSE)
	dlg.SetDefaultSize(350, 150)

	content, err := dlg.GetContentArea()
	if err != nil {
		return
	}
	content.SetSpacing(8)
	content.SetMarginStart(16)
	content.SetMarginEnd(16)
	content.SetMarginTop(12)
	content.SetMarginBottom(12)

	infoLabel, _ := gtk.LabelNew(fmt.Sprintf("Root password for VM <b>%s</b>:", vm.Name))
	infoLabel.SetUseMarkup(true)
	content.PackStart(infoLabel, false, false, 0)

	pwdBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	content.PackStart(pwdBox, false, false, 0)

	pwdEntry, _ := gtk.EntryNew()
	pwdEntry.SetText(vm.RootPassword)
	pwdEntry.SetVisibility(false)
	pwdEntry.SetEditable(false)
	pwdEntry.SetHExpand(true)
	pwdBox.PackStart(pwdEntry, true, true, 0)

	showCheck, _ := gtk.CheckButtonNewWithLabel("Show")
	showCheck.Connect("toggled", func() {
		pwdEntry.SetVisibility(showCheck.GetActive())
	})
	pwdBox.PackStart(showCheck, false, false, 0)

	copyBtn, _ := gtk.ButtonNewWithLabel("Copy")
	copyBtn.Connect("clicked", func() {
		clipboard, err := gtk.ClipboardGet(gdk.SELECTION_CLIPBOARD)
		if err == nil && clipboard != nil {
			clipboard.SetText(vm.RootPassword)
		}
		copyBtn.SetLabel("Copied!")
	})
	pwdBox.PackStart(copyBtn, false, false, 0)

	dlg.ShowAll()
	dlg.Run()
}

func (app *App) showConfigDialog() {
	cfg, err := app.engine.LoadConfig()
	if err != nil {
		app.showError("Error", "Failed to load config: "+err.Error())
		return
	}

	dlg, err := gtk.DialogNew()
	if err != nil {
		app.showError("Error", "Failed to create dialog: "+err.Error())
		return
	}
	defer dlg.Destroy()
	dlg.SetTitle("Settings")
	dlg.SetTransientFor(app.win)
	dlg.SetModal(true)
	_, _ = dlg.AddButton("Cancel", gtk.RESPONSE_CANCEL)
	_, _ = dlg.AddButton("Save", gtk.RESPONSE_OK)
	dlg.SetDefaultSize(450, 200)

	content, err := dlg.GetContentArea()
	if err != nil {
		return
	}
	content.SetSpacing(8)
	content.SetMarginStart(16)
	content.SetMarginEnd(16)
	content.SetMarginTop(12)
	content.SetMarginBottom(12)

	grid, _ := gtk.GridNew()
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(12)
	grid.SetHExpand(true)
	content.PackStart(grid, true, true, 0)

	sshLabel, _ := gtk.LabelNew("Default SSH Key:")
	sshLabel.SetHAlign(gtk.ALIGN_END)
	grid.Attach(sshLabel, 0, 0, 1, 1)

	sshEntry, _ := gtk.EntryNew()
	sshEntry.SetText(cfg.DefaultSSHKey)
	sshEntry.SetPlaceholderText("ssh-ed25519 AAA... or ~/.ssh/id_ed25519.pub")
	sshEntry.SetHExpand(true)
	grid.Attach(sshEntry, 1, 0, 1, 1)

	sshHelp, _ := gtk.LabelNew("Path to a public key file (e.g. ~/.ssh/id_ed25519.pub) or the key content itself.\nThis key will be pre-filled for new VMs.")
	sshHelp.SetLineWrap(true)
	sshHelp.SetHAlign(gtk.ALIGN_START)
	sshHelp.SetMarginTop(4)
	grid.Attach(sshHelp, 1, 1, 1, 1)

	dlg.ShowAll()

	if dlg.Run() != gtk.RESPONSE_OK {
		return
	}

	sshText, _ := sshEntry.GetText()
	cfg.DefaultSSHKey = strings.TrimSpace(sshText)

	go func() {
		if err := app.engine.SaveConfig(cfg); err != nil {
			glib.IdleAdd(func() {
				app.showError("Save Config", err.Error())
			})
			return
		}
		glib.IdleAdd(func() {
			msg := gtk.MessageDialogNewWithMarkup(app.win, gtk.DIALOG_MODAL, gtk.MESSAGE_INFO, gtk.BUTTONS_OK,
				"Settings saved successfully.")
			msg.Run()
			msg.Destroy()
		})
	}()
}

func maxMemoryMB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 65536
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			var kb int
			_, _ = fmt.Sscanf(line, "MemTotal: %d kB", &kb)
			if kb > 0 {
				return kb / 1024
			}
		}
	}
	return 65536
}
