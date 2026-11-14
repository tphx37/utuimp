// Package gfx draws real pixels in terminals that speak the kitty graphics
// protocol: the program renders a picture itself and the terminal puts it
// on screen as a texture, scaled to a rectangle of cells and, if asked,
// under the text.
package gfx

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mode is how pictures reach the terminal.
type Mode int

const (
	// Off: the terminal can't show pictures (or it wasn't asked to).
	Off Mode = iota
	// Shm: the pixels are left in shared memory and only its name is sent.
	// Fast enough for full-screen animation; local terminals only.
	Shm
	// Direct: the pixels travel in the escape sequence itself. Works over
	// ssh; slow, so pictures are kept small.
	Direct
)

func (m Mode) String() string {
	switch m {
	case Shm:
		return "shm"
	case Direct:
		return "direct"
	}
	return "off"
}

// Detect says how this terminal takes pictures, from its environment.
// setting is the user's choice: "off", "on" (trust the terminal even if it
// isn't recognised) or anything else for automatic.
func Detect(getenv func(string) string, setting string) Mode {
	if setting == "off" {
		return Off
	}
	if getenv("TMUX") != "" || getenv("STY") != "" || strings.HasPrefix(getenv("TERM"), "screen") {
		return Off // multiplexers don't pass pictures through
	}
	known := getenv("KITTY_WINDOW_ID") != "" || getenv("TERM") == "xterm-kitty" ||
		getenv("TERM") == "xterm-ghostty" || getenv("GHOSTTY_RESOURCES_DIR") != ""
	if !known && setting != "on" {
		return Off
	}
	if getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" {
		return Direct // the terminal is on another machine: no shared memory
	}
	if st, err := os.Stat(shmDir); err != nil || !st.IsDir() {
		return Direct
	}
	return Shm
}

// shmDir is where POSIX shared memory objects live on Linux.
var shmDir = "/dev/shm" //nolint:gochecknoglobals // replaced in tests

// Image is one picture on screen, replaced frame after frame.
type Image struct {
	ID int // the protocol's image id, 1…
	Z  int // below 0 puts the picture under the text
	// Alpha: the pixels are four bytes each, the last one how opaque the
	// pixel is; what is under the picture shows through the rest.
	Alpha bool
	// Native shows the picture at its own size in pixels, not stretched
	// over the cells, with its corner OffX, OffY pixels into the first
	// cell: for one that moves or grows smoothly.
	Native     bool
	OffX, OffY int
	// SrcW, SrcH, when set, show only that much of the picture (from its
	// corner), stretched over the cells: with a clear margin round a
	// picture this scales it to any size, not just whole cells (see Place).
	SrcW, SrcH int
	n          uint64
	// The shared memory files of the last few frames. The terminal removes
	// each once it has read it; one it never read (it was busy, or isn't
	// what it seemed) is removed here when it falls out of the list.
	sent []string
}

// keepFrames is how many frames the terminal may be behind.
const keepFrames = 6

// apc wraps a graphics command.
func apc(control, payload string) string {
	return "\x1b_G" + control + ";" + payload + "\x1b\\"
}

// Frame returns the escape sequence that shows rgb (w×h pixels, three bytes
// each, or four with Alpha) over cols×rows cells with its corner at the cell (col, row),
// counted from 0. It replaces what the image showed before. The cursor is
// put back where it was.
func (im *Image) Frame(mode Mode, rgb []byte, w, h, col, row, cols, rows int) (string, error) {
	bpp, format := 3, 24
	if im.Alpha {
		bpp, format = 4, 32
	}
	if mode == Off || w <= 0 || h <= 0 || len(rgb) < w*h*bpp {
		return "", nil
	}
	rgb = rgb[:w*h*bpp]
	ctl := fmt.Sprintf("a=T,f=%d,s=%d,v=%d,i=%d,p=1,c=%d,r=%d,z=%d,C=1,q=2", format, w, h, im.ID, cols, rows, im.Z)
	if !im.Native && (im.OffX != 0 || im.OffY != 0) {
		ctl += fmt.Sprintf(",X=%d,Y=%d", im.OffX, im.OffY)
	}
	if im.Native {
		ctl = fmt.Sprintf("a=T,f=%d,s=%d,v=%d,i=%d,p=1,X=%d,Y=%d,z=%d,C=1,q=2", format, w, h, im.ID, im.OffX, im.OffY, im.Z)
	}
	var sb strings.Builder
	// Save the cursor, go to the corner; synchronised so the old and the
	// new frame never show half and half.
	fmt.Fprintf(&sb, "\x1b[?2026h\x1b7\x1b[%d;%dH", row+1, col+1)
	switch mode {
	case Shm:
		for len(im.sent) >= keepFrames {
			_ = os.Remove(im.sent[0])
			im.sent = im.sent[1:]
		}
		im.n++
		name := fmt.Sprintf("utuimp-%d-%d-%d", os.Getpid(), im.ID, im.n)
		path := filepath.Join(shmDir, name)
		if err := os.WriteFile(path, rgb, 0o600); err != nil {
			return "", err
		}
		im.sent = append(im.sent, path)
		// kitty takes the name only with its leading slash.
		sb.WriteString(apc(ctl+",t=s", base64.StdEncoding.EncodeToString([]byte("/"+name))))
	default:
		enc := base64.StdEncoding.EncodeToString(rgb)
		const chunk = 4096
		for first := true; len(enc) > 0; first = false {
			part := enc
			if len(part) > chunk {
				part = part[:chunk]
			}
			enc = enc[len(part):]
			more := 0
			if len(enc) > 0 {
				more = 1
			}
			if first {
				sb.WriteString(apc(fmt.Sprintf("%s,t=d,m=%d", ctl, more), part))
			} else {
				sb.WriteString(apc(fmt.Sprintf("m=%d", more), part))
			}
		}
	}
	sb.WriteString("\x1b8\x1b[?2026l")
	return sb.String(), nil
}

// CleanStale removes frames left in shared memory by utuimp processes that
// are gone (one that was killed has no chance to tidy up after itself).
func CleanStale() {
	entries, err := os.ReadDir(shmDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		var pid, id, n int
		if c, err := fmt.Sscanf(e.Name(), "utuimp-%d-%d-%d", &pid, &id, &n); err != nil || c != 3 || pid <= 0 {
			continue
		}
		// This process's own are stale too: this is called before it has
		// drawn anything, so they are from before it restarted in place.
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil && pid != os.Getpid() {
			continue // still running
		}
		_ = os.Remove(filepath.Join(shmDir, e.Name()))
	}
}

// Delete returns the sequence that takes the picture off the screen and
// frees it, and forgets its last frame.
func (im *Image) Delete() string {
	for _, path := range im.sent {
		_ = os.Remove(path)
	}
	im.sent = nil
	return apc(fmt.Sprintf("a=d,d=I,i=%d,q=2", im.ID), "")
}

// CleanOwn removes the frames this process left in shared memory: the last
// few a terminal had not read yet when the program ended.
func CleanOwn() {
	entries, err := os.ReadDir(shmDir)
	if err != nil {
		return
	}
	prefix := fmt.Sprintf("utuimp-%d-", os.Getpid())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			_ = os.Remove(filepath.Join(shmDir, e.Name()))
		}
	}
}

// Place returns the sequence that puts the picture the terminal already
// has (see Frame) somewhere else, or at another size: over cols×rows cells
// from the cell (col, row), OffX, OffY pixels into it. Nothing is sent but
// the place, so a picture can travel at the screen's own pace however
// large it is.
//
// With Native the picture keeps its own size in pixels and cols, rows are
// not used: it only moves.
func (im *Image) Place(col, row, cols, rows int) string {
	ctl := fmt.Sprintf("a=p,i=%d,p=1,c=%d,r=%d,X=%d,Y=%d,z=%d,C=1,q=2", im.ID, cols, rows, im.OffX, im.OffY, im.Z)
	switch {
	case im.Native:
		ctl = fmt.Sprintf("a=p,i=%d,p=1,X=%d,Y=%d,z=%d,C=1,q=2", im.ID, im.OffX, im.OffY, im.Z)
	case im.SrcW > 0 && im.SrcH > 0:
		ctl += fmt.Sprintf(",x=0,y=0,w=%d,h=%d", im.SrcW, im.SrcH)
	}
	return fmt.Sprintf("\x1b[?2026h\x1b7\x1b[%d;%dH", row+1, col+1) + apc(ctl, "") + "\x1b8\x1b[?2026l"
}

// Upload hands the terminal a picture (w×h pixels, as for Frame) without
// showing it: Place shows it then, as often and wherever. Shared memory
// only; ok is false when it could not be sent.
func (im *Image) Upload(mode Mode, rgb []byte, w, h int) (seq string, ok bool) {
	bpp, format := 3, 24
	if im.Alpha {
		bpp, format = 4, 32
	}
	if mode != Shm || w <= 0 || h <= 0 || len(rgb) < w*h*bpp {
		return "", false
	}
	for len(im.sent) >= keepFrames {
		_ = os.Remove(im.sent[0])
		im.sent = im.sent[1:]
	}
	im.n++
	name := fmt.Sprintf("utuimp-%d-%d-%d", os.Getpid(), im.ID, im.n)
	path := filepath.Join(shmDir, name)
	if err := os.WriteFile(path, rgb[:w*h*bpp], 0o600); err != nil {
		return "", false
	}
	im.sent = append(im.sent, path)
	return apc(fmt.Sprintf("a=t,f=%d,s=%d,v=%d,i=%d,q=2,t=s", format, w, h, im.ID), base64.StdEncoding.EncodeToString([]byte("/"+name))), true
}
