//go:build linux || darwin

package cmd

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// captureStderr points file descriptor 2 at a log file while the TUI runs and
// returns a function that restores it. Native libraries (PipeWire, GStreamer,
// the CDM) write warnings and failed assertions straight to fd 2 — unplugging
// headphones, for one, makes PipeWire print one — and on the terminal that
// text lands on top of the interface. Go's os.Stderr is the same descriptor,
// so a crash report ends up in the log too.
func captureStderr() (restore func()) {
	noop := func() {}
	dir, err := os.UserCacheDir()
	if err != nil {
		return noop
	}
	dir = filepath.Join(dir, "utuimp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return noop
	}
	name := "stderr.log"
	if debug {
		name = "debug.log"
	}
	path := filepath.Join(dir, name)
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if st, err := os.Stat(path); !debug && err == nil && st.Size() > 1<<20 {
		flags |= os.O_TRUNC // keep the log small
	}
	f, err := os.OpenFile(path, flags, 0o600) //nolint:gosec // our own cache file
	if err != nil {
		return noop
	}
	saved, err := unix.Dup(2)
	if err != nil {
		_ = f.Close()
		return noop
	}
	if err := dupTo(int(f.Fd()), 2); err != nil { //nolint:gosec // fd fits in int
		_ = unix.Close(saved)
		_ = f.Close()
		return noop
	}
	return func() {
		_ = dupTo(saved, 2)
		_ = unix.Close(saved)
		_ = f.Close()
	}
}
