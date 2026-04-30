//go:build cgo
// +build cgo

package gui

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/c0m4r/v/engine"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type App struct {
	engine    *engine.Engine
	win       *gtk.Window
	header    *gtk.HeaderBar
	listBox   *gtk.ListBox
	statusBar *gtk.Label
	mu        sync.Mutex
}

func Run(e *engine.Engine, args []string) error {
	gtk.Init(&args)

	app := &App{engine: e}

	css := `
		.vm-running { color: #3fb950; font-weight: bold; }
		.vm-stopped { color: #8b949e; }
		.vm-name { font-size: 14px; font-weight: bold; }
		.vm-detail { font-size: 11px; color: #8b949e; }
		.vm-ip { font-size: 11px; color: #58a6ff; }
		.action-button { padding: 2px 8px; min-height: 24px; font-size: 11px; }
		.status-bar { font-size: 11px; color: #8b949e; padding: 4px 8px; }
		.dialog-form { margin: 12px; }
	`

	provider, err := gtk.CssProviderNew()
	if err != nil {
		return fmt.Errorf("create CSS provider: %w", err)
	}
	if err := provider.LoadFromData(css); err != nil {
		return fmt.Errorf("load CSS: %w", err)
	}
	screen, err := gdk.ScreenGetDefault()
	if err != nil {
		return fmt.Errorf("get default screen: %w", err)
	}
	gtk.AddProviderForScreen(screen, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return fmt.Errorf("create window: %w", err)
	}
	win.SetTitle("v - VM Manager")
	win.SetDefaultSize(960, 640)
	win.Connect("destroy", gtk.MainQuit)
	app.win = win

	header, err := gtk.HeaderBarNew()
	if err != nil {
		return fmt.Errorf("create header bar: %w", err)
	}
	header.SetShowCloseButton(true)
	header.SetTitle("v - VM Manager")
	header.SetSubtitle("KVM Virtual Machine Manager")
	app.header = header
	win.SetTitlebar(header)

	newBtn, err := gtk.ButtonNewWithLabel("New VM")
	if err != nil {
		return fmt.Errorf("create button: %w", err)
	}
	newBtn.Connect("clicked", func() { app.showCreateDialog() })
	header.PackStart(newBtn)

	pullBtn, err := gtk.ButtonNewWithLabel("Pull Image")
	if err != nil {
		return fmt.Errorf("create button: %w", err)
	}
	pullBtn.Connect("clicked", func() { app.showImageManager() })
	header.PackStart(pullBtn)

	refreshBtn, err := gtk.ButtonNewWithLabel("Refresh")
	if err != nil {
		return fmt.Errorf("create button: %w", err)
	}
	refreshBtn.Connect("clicked", func() { app.refreshVMList() })
	header.PackStart(refreshBtn)

	settingsBtn, err := gtk.ButtonNewWithLabel("Settings")
	if err != nil {
		return fmt.Errorf("create button: %w", err)
	}
	settingsBtn.Connect("clicked", func() { app.showConfigDialog() })
	header.PackEnd(settingsBtn)

	mainBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return fmt.Errorf("create box: %w", err)
	}
	win.Add(mainBox)

	sw, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		return fmt.Errorf("create scrolled window: %w", err)
	}
	sw.SetPolicy(gtk.POLICY_NEVER, gtk.POLICY_AUTOMATIC)
	mainBox.PackStart(sw, true, true, 0)

	listBox, err := gtk.ListBoxNew()
	if err != nil {
		return fmt.Errorf("create list box: %w", err)
	}
	listBox.SetSelectionMode(gtk.SELECTION_SINGLE)
	sw.Add(listBox)
	app.listBox = listBox

	statusBar, err := gtk.LabelNew("Loading...")
	if err != nil {
		return fmt.Errorf("create label: %w", err)
	}
	statusBar.SetXAlign(0)
	statusBar.SetMarginStart(8)
	statusBar.SetMarginEnd(8)
	statusBar.SetMarginTop(4)
	statusBar.SetMarginBottom(4)
	statusBar.SetHExpand(true)
	statusCtx, err := statusBar.GetStyleContext()
	if err == nil {
		statusCtx.AddClass("status-bar")
	}
	mainBox.PackStart(statusBar, false, false, 0)
	app.statusBar = statusBar

	app.refreshVMList()
	app.updateStatusBar()

	glib.TimeoutAdd(5000, func() bool {
		glib.IdleAdd(func() {
			app.refreshVMList()
		})
		return true
	})

	win.ShowAll()
	gtk.Main()
	return nil
}

func (app *App) refreshVMList() {
	app.mu.Lock()
	defer app.mu.Unlock()

	children := app.listBox.GetChildren()
	children.Foreach(func(item interface{}) {
		if row, ok := item.(*gtk.Widget); ok {
			app.listBox.Remove(row)
		}
	})

	vms, err := app.engine.ListVMs()
	if err != nil {
		app.listBox.ShowAll()
		app.updateStatusBar()
		return
	}

	if len(vms) == 0 {
		emptyLabel, _ := gtk.LabelNew("No VMs found. Click \"New VM\" to create one.")
		emptyLabel.SetMarginTop(24)
		emptyLabel.SetMarginBottom(24)
		emptyLabel.SetHExpand(true)
		emptyCtx, _ := emptyLabel.GetStyleContext()
		emptyCtx.AddClass("vm-stopped")
		row, _ := gtk.ListBoxRowNew()
		row.Add(emptyLabel)
		row.SetSensitive(false)
		app.listBox.Insert(row, -1)
		app.listBox.ShowAll()
		app.updateStatusBar()
		return
	}

	for _, vm := range vms {
		state, _ := app.engine.VMState(vm.ID)
		vmCopy := vm
		stateCopy := state
		row := app.createVMRow(vmCopy, stateCopy)
		app.listBox.Insert(row, -1)
	}

	app.listBox.ShowAll()
	app.updateStatusBar()
}

func (app *App) createVMRow(vm *engine.VM, state engine.State) *gtk.ListBoxRow {
	row, _ := gtk.ListBoxRowNew()
	row.SetMarginStart(4)
	row.SetMarginEnd(4)
	row.SetMarginTop(2)
	row.SetMarginBottom(2)

	outerBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	outerBox.SetMarginStart(8)
	outerBox.SetMarginEnd(8)
	outerBox.SetMarginTop(6)
	outerBox.SetMarginBottom(6)
	row.Add(outerBox)

	infoBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 16)
	outerBox.PackStart(infoBox, false, false, 0)

	stateDot := "●"
	stateClass := "vm-running"
	stateLabel := string(state)
	if state != engine.StateRunning {
		stateDot = "○"
		stateClass = "vm-stopped"
		stateLabel = "stopped"
	}

	nameLabel, _ := gtk.LabelNew(vm.Name)
	nameCtx, _ := nameLabel.GetStyleContext()
	nameCtx.AddClass("vm-name")
	nameLabel.SetHAlign(gtk.ALIGN_START)
	infoBox.PackStart(nameLabel, false, false, 0)

	statusLabel, _ := gtk.LabelNew(fmt.Sprintf("%s %s", stateDot, stateLabel))
	statusCtx, _ := statusLabel.GetStyleContext()
	statusCtx.AddClass(stateClass)
	infoBox.PackStart(statusLabel, false, false, 0)

	specs := fmt.Sprintf("%d CPUs | %d MB RAM | %s disk | %s | net: %s", vm.CPUs, vm.MemoryMB, vm.DiskSize, vm.BaseImage, vm.NetMode)
	specLabel, _ := gtk.LabelNew(specs)
	specCtx, _ := specLabel.GetStyleContext()
	specCtx.AddClass("vm-detail")
	specLabel.SetHAlign(gtk.ALIGN_START)
	specLabel.SetHExpand(true)
	infoBox.PackStart(specLabel, true, true, 0)

	ipInfo := ""
	if vm.NetMode == "bridge" {
		if ip := app.engine.VMIPAddress(vm); ip != "" {
			ipInfo = fmt.Sprintf("IP: %s", ip)
		}
	} else if vm.SSHPort > 0 {
		ipInfo = fmt.Sprintf("SSH: localhost:%d", vm.SSHPort)
	}
	netLabel, _ := gtk.LabelNew(ipInfo)
	netCtx, _ := netLabel.GetStyleContext()
	netCtx.AddClass("vm-ip")
	netLabel.SetHAlign(gtk.ALIGN_END)
	netLabel.SetHExpand(false)
	infoBox.PackEnd(netLabel, false, false, 0)

	btnBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	btnBox.SetMarginTop(4)
	outerBox.PackStart(btnBox, false, false, 0)

	startBtn, _ := gtk.ButtonNewWithLabel("▶ Start")
	startCtx, _ := startBtn.GetStyleContext()
	startCtx.AddClass("action-button")
	startBtn.SetSensitive(state != engine.StateRunning)
	startBtn.Connect("clicked", func() { app.startVM(vm) })
	btnBox.PackStart(startBtn, false, false, 0)

	stopBtn, _ := gtk.ButtonNewWithLabel("⏹ Stop")
	stopCtx, _ := stopBtn.GetStyleContext()
	stopCtx.AddClass("action-button")
	stopBtn.SetSensitive(state == engine.StateRunning)
	stopBtn.Connect("clicked", func() { app.stopVM(vm) })
	btnBox.PackStart(stopBtn, false, false, 0)

	forceBtn, _ := gtk.ButtonNewWithLabel("⏻ Force Stop")
	forceCtx, _ := forceBtn.GetStyleContext()
	forceCtx.AddClass("action-button")
	forceBtn.SetSensitive(state == engine.StateRunning)
	forceBtn.Connect("clicked", func() { app.forceStopVM(vm) })
	btnBox.PackStart(forceBtn, false, false, 0)

	restartBtn, _ := gtk.ButtonNewWithLabel("↺ Restart")
	restartCtx, _ := restartBtn.GetStyleContext()
	restartCtx.AddClass("action-button")
	restartBtn.SetSensitive(state == engine.StateRunning)
	restartBtn.Connect("clicked", func() { app.restartVM(vm) })
	btnBox.PackStart(restartBtn, false, false, 0)

	consoleBtn, _ := gtk.ButtonNewWithLabel("🖵 Console")
	consoleCtx, _ := consoleBtn.GetStyleContext()
	consoleCtx.AddClass("action-button")
	consoleBtn.SetSensitive(state == engine.StateRunning)
	consoleBtn.Connect("clicked", func() { app.openConsole(vm) })
	btnBox.PackStart(consoleBtn, false, false, 0)

	settingsBtn2, _ := gtk.ButtonNewWithLabel("☰ Settings")
	settingsCtx, _ := settingsBtn2.GetStyleContext()
	settingsCtx.AddClass("action-button")
	settingsBtn2.Connect("clicked", func() { app.showVMSettingsDialog(vm) })
	btnBox.PackStart(settingsBtn2, false, false, 0)

	pwdBtn, _ := gtk.ButtonNewWithLabel("🔑 Password")
	pwdCtx, _ := pwdBtn.GetStyleContext()
	pwdCtx.AddClass("action-button")
	pwdBtn.SetSensitive(vm.RootPassword != "")
	pwdBtn.Connect("clicked", func() { app.showPasswordDialog(vm) })
	btnBox.PackStart(pwdBtn, false, false, 0)

	deleteBtn, _ := gtk.ButtonNewWithLabel("🗑 Delete")
	deleteCtx, _ := deleteBtn.GetStyleContext()
	deleteCtx.AddClass("action-button")
	deleteBtn.SetSensitive(state != engine.StateRunning)
	deleteBtn.Connect("clicked", func() { app.deleteVM(vm) })
	btnBox.PackStart(deleteBtn, false, false, 0)

	return row
}

func (app *App) updateStatusBar() {
	vms, err := app.engine.ListVMs()
	if err != nil {
		app.statusBar.SetLabel("Error: " + err.Error())
		return
	}

	running := 0
	for _, vm := range vms {
		state, _ := app.engine.VMState(vm.ID)
		if state == engine.StateRunning {
			running++
		}
	}

	netStatus := app.engine.GetNetStatus()
	bridgeStatus := "not configured"
	if netStatus.BridgeExists {
		bridgeStatus = "active"
		if netStatus.BridgeIP != "" {
			bridgeStatus += fmt.Sprintf(" (%s)", netStatus.BridgeIP)
		}
	}

	status := fmt.Sprintf("%d VMs (%d running) | Bridge: %s", len(vms), running, bridgeStatus)
	if !netStatus.IPForward {
		status += " | IP forwarding: off"
	}
	app.statusBar.SetLabel(status)
}

func (app *App) startVM(vm *engine.VM) {
	go func() {
		if err := app.engine.StartVM(vm.ID); err != nil {
			glib.IdleAdd(func() {
				app.showError("Start VM", err.Error())
			})
		}
		glib.IdleAdd(func() {
			app.refreshVMList()
		})
	}()
}

func (app *App) stopVM(vm *engine.VM) {
	go func() {
		if err := app.engine.StopVM(vm.ID); err != nil {
			glib.IdleAdd(func() {
				app.showError("Stop VM", err.Error())
			})
		}
		glib.IdleAdd(func() {
			time.Sleep(2 * time.Second)
			app.refreshVMList()
		})
	}()
}

func (app *App) forceStopVM(vm *engine.VM) {
	go func() {
		if err := app.engine.ForceStopVM(vm.ID); err != nil {
			glib.IdleAdd(func() {
				app.showError("Force Stop VM", err.Error())
			})
		}
		glib.IdleAdd(func() {
			app.refreshVMList()
		})
	}()
}

func (app *App) restartVM(vm *engine.VM) {
	go func() {
		if err := app.engine.RestartVM(vm.ID); err != nil {
			glib.IdleAdd(func() {
				app.showError("Restart VM", err.Error())
			})
		}
		glib.IdleAdd(func() {
			time.Sleep(3 * time.Second)
			app.refreshVMList()
		})
	}()
}

func (app *App) deleteVM(vm *engine.VM) {
	msg := fmt.Sprintf("Delete VM <b>%s</b>?\n\nAll data including the disk image will be permanently deleted.", vm.Name)
	dlg := gtk.MessageDialogNewWithMarkup(app.win, gtk.DIALOG_MODAL, gtk.MESSAGE_QUESTION, gtk.BUTTONS_YES_NO, "%s", msg)
	res := dlg.Run()
	dlg.Destroy()

	if res != gtk.RESPONSE_YES {
		return
	}

	go func() {
		if err := app.engine.DeleteVM(vm.ID); err != nil {
			glib.IdleAdd(func() {
				app.showError("Delete VM", err.Error())
			})
		}
		glib.IdleAdd(func() {
			app.refreshVMList()
		})
	}()
}

func (app *App) showError(title, msg string) {
	dlg := gtk.MessageDialogNewWithMarkup(app.win, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s",
		fmt.Sprintf("<b>%s</b>\n\n%s", title, msg))
	dlg.Run()
	dlg.Destroy()
}

func openTerminal(command string, args ...string) error {
	terms := []string{
		"gnome-terminal", "xterm", "alacritty",
		"konsole", "terminator", "kitty", "lxterminal",
	}

	for _, term := range terms {
		path, err := exec.LookPath(term)
		if err != nil {
			continue
		}

		switch term {
		case "gnome-terminal":
			cmdArgs := []string{"--", command}
			cmdArgs = append(cmdArgs, args...)
			cmd := exec.Command(path, cmdArgs...)
			return cmd.Start()
		case "xterm":
			cmdArgs := []string{"-e", command}
			cmdArgs = append(cmdArgs, args...)
			cmd := exec.Command(path, cmdArgs...)
			return cmd.Start()
		case "alacritty":
			cmdArgs := []string{"-e", command}
			cmdArgs = append(cmdArgs, args...)
			cmd := exec.Command(path, cmdArgs...)
			return cmd.Start()
		case "konsole":
			cmdArgs := []string{"-e", command}
			cmdArgs = append(cmdArgs, args...)
			cmd := exec.Command(path, cmdArgs...)
			return cmd.Start()
		case "terminator":
			cmdArgs := []string{"-e", command}
			cmdArgs = append(cmdArgs, args...)
			cmd := exec.Command(path, cmdArgs...)
			return cmd.Start()
		case "kitty":
			cmdArgs := append([]string{command}, args...)
			cmd := exec.Command(path, cmdArgs...)
			return cmd.Start()
		case "lxterminal":
			cmdArgs := []string{"-e", command}
			cmdArgs = append(cmdArgs, args...)
			cmd := exec.Command(path, cmdArgs...)
			return cmd.Start()
		}
	}

	return fmt.Errorf("no terminal emulator found (tried: %v)", terms)
}
