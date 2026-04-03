package cli

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/c0m4r/v/engine"
	"golang.org/x/term"
)

func cmdConsole(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v console <name|id>")
	}

	sockPath, err := e.ConsoleSocketPath(args[0])
	if err != nil {
		return err
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("connect to console: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Set terminal to raw mode
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("set raw mode: %w", err)
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	fmt.Fprintf(os.Stderr, "\r\nConnected to console. Press Ctrl+] to detach.\r\n")

	// Restore terminal on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		_ = term.Restore(fd, oldState)
		os.Exit(0)
	}()

	done := make(chan struct{})

	// Socket -> stdout
	go func() {
		_, _ = io.Copy(os.Stdout, conn)
		close(done)
	}()

	// Stdin -> socket (with Ctrl+] escape detection)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				break
			}
			for i := 0; i < n; i++ {
				if buf[i] == 0x1d { // Ctrl+]
					fmt.Fprintf(os.Stderr, "\r\nDetached.\r\n")
					_ = conn.Close()
					return
				}
			}
			_, _ = conn.Write(buf[:n])
		}
	}()

	<-done
	return nil
}
