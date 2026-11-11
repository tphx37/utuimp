//go:build unix

package styles

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

var paletteReply = regexp.MustCompile(`\x1b\]4;(\d+);rgb:([0-9a-fA-F]+)/([0-9a-fA-F]+)/([0-9a-fA-F]+)`)

// QueryPalette asks the terminal for the real colours of its 16 palette
// entries (OSC 4), which a theme that draws with the terminal's own palette
// needs whenever a colour has to be blended (the interface's fades). It
// must run before the program takes over the terminal. The question ends
// with one every terminal answers, so there is no waiting on those that
// don't know OSC 4; a silent terminal costs half a second, once.
func QueryPalette() {
	if !ANSIMode || Console {
		return
	}
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	fd := f.Fd()
	st, err := term.MakeRaw(fd)
	if err != nil {
		return
	}
	defer func() { _ = term.Restore(fd, st) }()
	var q strings.Builder
	for i := range 16 {
		fmt.Fprintf(&q, "\x1b]4;%d;?\x07", i)
	}
	q.WriteString("\x1b[c")
	if _, err := f.WriteString(q.String()); err != nil {
		return
	}
	var buf []byte
	tmp := make([]byte, 4096)
	for end := time.Now().Add(500 * time.Millisecond); time.Now().Before(end); {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}} //nolint:gosec // a file descriptor
		n, err := unix.Poll(fds, 50)
		if err == unix.EINTR || err == nil && n == 0 {
			continue
		}
		if err != nil {
			break
		}
		k, err := unix.Read(int(fd), tmp) //nolint:gosec // a file descriptor
		if err != nil || k <= 0 {
			break
		}
		buf = append(buf, tmp[:k]...)
		if i := bytes.Index(buf, []byte("\x1b[?")); i >= 0 && bytes.IndexByte(buf[i:], 'c') >= 0 {
			break
		}
	}
	ParsePalette(buf)
}

// ParsePalette reads OSC 4 replies; the palette is known once all 16 came.
func ParsePalette(buf []byte) {
	comp := func(b []byte) uint8 {
		s := string(b)
		if len(s) == 1 {
			s += s
		}
		v, _ := strconv.ParseUint(s[:2], 16, 8)
		return uint8(v)
	}
	var pal [16][3]uint8
	seen := 0
	for _, m := range paletteReply.FindAllSubmatch(buf, -1) {
		if i, err := strconv.Atoi(string(m[1])); err == nil && i >= 0 && i < 16 {
			pal[i] = [3]uint8{comp(m[2]), comp(m[3]), comp(m[4])}
			seen |= 1 << i
		}
	}
	if seen == 1<<16-1 {
		Palette, PaletteKnown = pal, true
	}
}
