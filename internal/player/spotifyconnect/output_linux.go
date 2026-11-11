//go:build linux

package spotifyconnect

import (
	"io"
	"os"

	"golang.org/x/sys/unix"

	"github.com/tphx37/utuimp/internal/player/gst"
)

// newOutput plays the raw sound read from r (see audioOut).
func newOutput(r io.Reader) (audioOut, error) {
	p, err := gst.NewRaw(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// shrinkPipe makes the pipe librespot writes into hold a tenth of a second
// instead of a third: whatever is in it is still heard after a pause that
// comes from outside.
func shrinkPipe(w *os.File) {
	_, _ = unix.FcntlInt(w.Fd(), unix.F_SETPIPE_SZ, 16384)
}

// pipePending is how many bytes wait to be read from f (a pipe).
func pipePending(f *os.File) int {
	if f == nil {
		return 0
	}
	n, err := unix.IoctlGetInt(int(f.Fd()), unix.TIOCINQ) // FIONREAD //nolint:gosec // a file descriptor
	if err != nil {
		return 0
	}
	return n
}
