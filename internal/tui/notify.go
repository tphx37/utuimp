package tui

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/provider"
	"golang.org/x/image/draw"
)

// A new song is announced on the desktop (Settings, Playback, Song
// notifications): its title, its artist and album, and its cover. The
// notification waits a moment for the cover (notifyWait), and goes without
// it if it doesn't come. One replaces the last instead of piling up.

// notifyWait is how long a notification waits for the song's cover.
const notifyWait = 1500 * time.Millisecond

// windowAway is whether the terminal utuimp is in is not the window in use
// (the terminal says so when it gains or loses focus).
var windowAway atomic.Bool

// notifyRun shows a notification by running a program (swapped in tests).
var notifyRun = func(name string, args ...string) error { //nolint:gochecknoglobals // swapped in tests
	return exec.Command(name, args...).Run() //nolint:gosec // a program of our own choosing, its words as arguments
}

// notifyTimeoutMsg ends the wait for a cover (see notifyWait).
type notifyTimeoutMsg struct{ id string }

// notifyWanted reports whether a new song is announced now.
func (m *Model) notifyWanted() bool {
	if m.cfg == nil {
		return false
	}
	switch m.cfg.Notify {
	case config.NotifyAlways:
		return true
	case config.NotifyAway:
		return windowAway.Load()
	}
	return false
}

// notifyTrack starts announcing t, which has just begun: once its cover is
// in (see notifyCover), or without it after notifyWait.
func (m *Model) notifyTrack(t *provider.Track) tea.Cmd {
	if t == nil || !m.notifyWanted() {
		m.notifyPending = ""
		return nil
	}
	id := t.ID + "\x00" + t.Title
	m.notifyPending = id
	if t.ArtworkURL == "" || m.artwork.failed {
		return m.notifySend(t, nil)
	}
	if m.artwork.img != nil && m.artwork.url == t.ArtworkURL {
		return m.notifySend(t, m.artwork.img)
	}
	return tea.Tick(notifyWait, func(time.Time) tea.Msg { return notifyTimeoutMsg{id} })
}

// notifyCover sends the notification waiting for the song now playing,
// with its cover (nil: there is none).
func (m *Model) notifyCover(img image.Image) tea.Cmd {
	t := m.playerState.Track
	if m.notifyPending == "" || t == nil || m.notifyPending != t.ID+"\x00"+t.Title {
		return nil
	}
	return m.notifySend(t, img)
}

// notifySend sends the notification for t, now.
func (m *Model) notifySend(t *provider.Track, img image.Image) tea.Cmd {
	m.notifyPending = ""
	if !m.notifyWanted() {
		return nil // came back to the window while the cover was on its way
	}
	title := t.Title
	var body []string
	if t.Artist != "" {
		body = append(body, t.Artist)
	}
	if t.Album != "" && t.Album != t.Title {
		body = append(body, t.Album)
	}
	text := strings.Join(body, " — ")
	return func() tea.Msg {
		icon := ""
		if img != nil {
			icon = notifyIcon(img)
		}
		name, args := notifyCommand(runtime.GOOS, title, text, icon)
		_ = notifyRun(name, args...)
		return nil
	}
}

// notifyCommand is the program and its arguments that show the notification
// on goos: notify-send, or osascript on macOS.
func notifyCommand(goos, title, text, icon string) (string, []string) {
	if goos == "darwin" {
		q := func(s string) string {
			return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
		}
		return "osascript", []string{"-e", "display notification " + q(text) + " with title " + q(title)}
	}
	// The body may be read as markup.
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	args := []string{"--app-name=utuimp",
		// One notification, replaced by the next, not a pile.
		"--hint=string:x-canonical-private-synchronous:utuimp", "--hint=string:x-dunst-stack-tag:utuimp"}
	if icon != "" {
		args = append(args, "--icon="+icon)
	}
	return "notify-send", append(args, "--", title, esc.Replace(text))
}

// notifyIcon writes the cover, at a notification's size, where the
// notification can show it, and returns where ("" if it can't).
func notifyIcon(img image.Image) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir = filepath.Join(dir, "utuimp")
	if os.MkdirAll(dir, 0o700) != nil {
		return ""
	}
	const side = 256
	small := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.CatmullRom.Scale(small, small.Bounds(), img, img.Bounds(), draw.Src, nil)
	path := filepath.Join(dir, "notify-cover.png")
	f, err := os.Create(path) //nolint:gosec // our own cache file
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	if png.Encode(f, small) != nil {
		return ""
	}
	return path
}
