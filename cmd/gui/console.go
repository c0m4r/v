//go:build cgo
// +build cgo

package gui

import (
	"fmt"

	"github.com/c0m4r/v/engine"
)

func (app *App) openConsole(vm *engine.VM) {
	sockPath, err := app.engine.ConsoleSocketPath(vm.ID)
	if err != nil {
		app.showError("Console", err.Error())
		return
	}

	if err := openTerminal("socat", "-", fmt.Sprintf("UNIX-CONNECT:%s", sockPath)); err != nil {
		if err := openTerminal("nc", "-U", sockPath); err != nil {
			app.showError("Console",
				fmt.Sprintf("Could not open terminal for console.\n\nSocket path: %s\n\nError: %v\n\nInstall socat or netcat to use the console.", sockPath, err))
			return
		}
	}
}
