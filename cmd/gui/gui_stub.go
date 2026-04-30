//go:build !cgo
// +build !cgo

package gui

import (
	"errors"

	"github.com/c0m4r/v/engine"
)

func Run(e *engine.Engine, args []string) error {
	return errors.New("GUI requires CGO (build with: CGO_ENABLED=1 go build .)")
}
