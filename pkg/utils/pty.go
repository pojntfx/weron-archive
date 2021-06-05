package utils

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// See https://github.com/creack/pty#shell
func RunInPTY(cmd *exec.Cmd) error {
	// Start the command with a pty.
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer func() {
		_ = ptmx.Close() // Best effort
	}()

	// Handle pty size.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx) // Best effort
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.
	defer func() {
		// Cleanup signals when done.
		signal.Stop(ch)
		close(ch)
	}()

	// Set stdin in raw mode.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() {
		_ = term.Restore(int(os.Stdin.Fd()), oldState) // Best effort
	}()

	// Copy stdin to the pty and the pty to stdout.
	// NOTE: The goroutine will keep reading until the next keystroke before returning.
	go func() {
		_, _ = io.Copy(ptmx, os.Stdin) // Best effort
	}()

	_, _ = io.Copy(os.Stdout, ptmx) // Best effort

	return nil
}
