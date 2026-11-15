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
	"strconv"
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
	// File: the pixels are written to a private temporary file and its path
	// is sent. This works on systems without POSIX /dev/shm.
	File
	// Direct: the pixels travel in the escape sequence itself. Works over
	// ssh; slow, so pictures are kept small.
	Direct
)

func (m Mode) String() string {
	switch m {
	case Shm:
		return "shm"
	case File:
		return "file"
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
	if st, err := os.Stat(shmDir); err == nil && st.IsDir() {
		return Shm
	}
	return File
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
	// Files for the last few frames, whether shared memory or temporary.
	// One the terminal has not read yet is kept briefly, then removed here.
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
		im.makeFrameRoom()
		im.n++
		name := fmt.Sprintf("utuimp-%d-%d-%d", os.Getpid(), im.ID, im.n)
		path := filepath.Join(shmDir, name)
		if err := os.WriteFile(path, rgb, 0o600); err != nil {
			return "", err
		}
		im.sent = append(im.sent, path)
		// kitty takes the name only with its leading slash.
		sb.WriteString(apc(ctl+",t=s", base64.StdEncoding.EncodeToString([]byte("/"+name))))
	case File:
		path, err := im.writeFrameFile(rgb)
		if err != nil {
			return "", err
		}
		sb.WriteString(apc(ctl+",t=f", base64.StdEncoding.EncodeToString([]byte(path))))
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

// makeFrameRoom bounds the frames a terminal may not have read yet. Keep a
// few because the terminal consumes the PTY asynchronously.
func (im *Image) makeFrameRoom() {
	for len(im.sent) >= keepFrames {
		_ = os.Remove(im.sent[0])
		im.sent = im.sent[1:]
	}
}

// writeFrameFile gives the terminal a complete, private file to read. A new
// name per frame avoids racing the terminal when it is still reading the
// previous one.
func (im *Image) writeFrameFile(rgb []byte) (string, error) {
	im.makeFrameRoom()
	f, err := os.CreateTemp(os.TempDir(), fmt.Sprintf("utuimp-%d-%d-", os.Getpid(), im.ID))
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err = f.Write(rgb); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	im.sent = append(im.sent, path)
	return path, nil
}

// CleanStale removes frames left by utuimp processes that are gone (one that
// was killed has no chance to tidy up after itself).
func CleanStale() {
	cleanStaleDir(shmDir)
	cleanStaleDir(os.TempDir())
}

func cleanStaleDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "utuimp-") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(name, "utuimp-"), "-", 2)
		if len(parts) != 2 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil || pid <= 0 {
			continue
		}
		// Our own files are stale at process startup. On Linux, leave files
		// belonging to a live process alone; on other systems /proc is absent,
		// so only this process's old files can be identified safely.
		if pid == os.Getpid() {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
			continue
		}
		if _, err := os.Stat("/proc"); err == nil {
			_ = os.Remove(filepath.Join(dir, name))
		}
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
	prefix := fmt.Sprintf("utuimp-%d-", os.Getpid())
	for _, dir := range []string{shmDir, os.TempDir()} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), prefix) {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
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
// showing it: Place shows it then, as often and wherever. Shared memory or
// file transfer; ok is false when it could not be sent.
func (im *Image) Upload(mode Mode, rgb []byte, w, h int) (seq string, ok bool) {
	bpp, format := 3, 24
	if im.Alpha {
		bpp, format = 4, 32
	}
	if (mode != Shm && mode != File) || w <= 0 || h <= 0 || len(rgb) < w*h*bpp {
		return "", false
	}
	rgb = rgb[:w*h*bpp]
	if mode == File {
		path, err := im.writeFrameFile(rgb)
		if err != nil {
			return "", false
		}
		return apc(fmt.Sprintf("a=t,f=%d,s=%d,v=%d,i=%d,q=2,t=f", format, w, h, im.ID), base64.StdEncoding.EncodeToString([]byte(path))), true
	}
	im.makeFrameRoom()
	im.n++
	name := fmt.Sprintf("utuimp-%d-%d-%d", os.Getpid(), im.ID, im.n)
	path := filepath.Join(shmDir, name)
	if err := os.WriteFile(path, rgb[:w*h*bpp], 0o600); err != nil {
		return "", false
	}
	im.sent = append(im.sent, path)
	return apc(fmt.Sprintf("a=t,f=%d,s=%d,v=%d,i=%d,q=2,t=s", format, w, h, im.ID), base64.StdEncoding.EncodeToString([]byte("/"+name))), true
}
