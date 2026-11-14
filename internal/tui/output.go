package tui

import (
	"io"
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// SharedOutput is the terminal, written by Bubble Tea and, directly, by
// the clocks that draw pictures (the lines, the visualizer, the start
// screen): each write whole, one at a time.
//
// Bubble Tea writes what it is given on a clock of its own (at most 120
// times a second), so a frame's pictures went out up to a flush late, by
// a different amount every frame against the screen's pace, and the
// terminal showed the frames for unequal times. Written here, a frame's
// pictures go out the moment the frame is drawn. Every picture's sequence
// puts the cursor back where it found it, so Bubble Tea's idea of the
// screen is untouched.
type SharedOutput struct {
	mu sync.Mutex
	f  *os.File
}

// NewSharedOutput wraps f, the terminal.
func NewSharedOutput(f *os.File) *SharedOutput { return &SharedOutput{f: f} }

func (o *SharedOutput) Write(b []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.f.Write(b)
}

func (o *SharedOutput) Read(b []byte) (int, error) { return o.f.Read(b) }
func (o *SharedOutput) Close() error               { return o.f.Close() }
func (o *SharedOutput) Fd() uintptr                { return o.f.Fd() }

// directOut is where pictures are written at once (see SharedOutput); nil:
// they go through Bubble Tea, as tea.Raw.
var directOut io.Writer

// SetDirectOutput has pictures written straight to w (see SharedOutput).
func SetDirectOutput(w io.Writer) { directOut = w }

// sendRaw sends a frame's pictures: at once where it can (see
// SharedOutput), else through Bubble Tea, the frame's cells held back
// until they have gone (see View).
func (m *Model) sendRaw(seq string) tea.Cmd {
	if seq == "" {
		return nil
	}
	if directOut != nil {
		_, _ = io.WriteString(directOut, seq)
		return nil
	}
	m.holdForRaw()
	return tea.Raw(seq)
}
