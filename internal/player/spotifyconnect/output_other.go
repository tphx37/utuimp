//go:build !linux

package spotifyconnect

import (
	"errors"
	"io"
	"os"
)

// newOutput: only the Linux build can play librespot's sound itself;
// elsewhere librespot plays it.
func newOutput(io.Reader) (audioOut, error) {
	return nil, errors.New("no audio pipeline on this system")
}

func shrinkPipe(*os.File) {}

func pipePending(*os.File) int { return 0 }
