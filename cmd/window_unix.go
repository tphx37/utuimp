//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
)

// detach puts the terminal in its own session so closing the shell that ran
// `utuimp --window` doesn't take the new window with it.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
