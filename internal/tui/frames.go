package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The clocks that draw the screen (the lines, the visualizer, the start
// screen, the words' effects, the lyrics) keep to the screen's own pace.

// screenFrame is a frame of the screen: sixty a second, or what fits the
// display's refresh evenly (see SetScreenPace).
var screenFrame = time.Second / 60

// SetScreenPace sets the pace the screen is drawn at from the display's
// refresh rate (Hz; 0 or less: not known, sixty a second). A frame must last
// a whole number of the display's refreshes, or the frames are shown for
// unequal times and what moves jerks: at 144 Hz sixty frames a second are
// shown two refreshes, then three, then two. So the pace is the refresh
// rate divided by a whole number, as near sixty as it goes and no more than
// ninety a second (more is a lot more work for a little): 72 at 144 Hz, 60
// at 60, 120 and 240 Hz, 75 at 75 Hz.
func SetScreenPace(hz float64) {
	screenFrame = time.Duration(float64(time.Second) / screenFPS(hz))
}

// screenFPS is the frames a second for a display refreshing hz times a
// second (see SetScreenPace).
func screenFPS(hz float64) float64 {
	if hz < 50 || hz > 1000 || math.IsNaN(hz) {
		return 60
	}
	k := max(1, math.Floor(hz/60+0.02)) // 59.94 Hz is sixty a second too
	for hz/k > 90 {
		k++
	}
	return hz / k
}

// InitScreenPace sets the pace for the display utuimp runs on, once at
// the start: UTUIMP_FPS sets the frames a second outright (10 to 240);
// otherwise Settings, Interface, Frame rate does (see applyFrameRate),
// "auto" fitting the display's refresh (see SetScreenPace).
func InitScreenPace() {
	paceEnv = 0
	if v, err := strconv.ParseFloat(os.Getenv("UTUIMP_FPS"), 64); err == nil && v >= 10 && v <= 240 {
		paceEnv = v
	}
	if paceEnv == 0 {
		displayHz = displayRefresh()
	}
	applyFrameRate(paceSetting)
}

// The pace asked for: by UTUIMP_FPS (0: not), by the settings ("", "60",
// "30"), and the display's refresh as found at the start (0: not known).
var (
	paceEnv     float64
	paceSetting string
	displayHz   float64
)

// applyFrameRate sets the pace from the settings' frame rate (see
// config.FrameRate): "60" or "30" frames a second, else as many as fit the
// display. UTUIMP_FPS goes before it.
func applyFrameRate(setting string) {
	paceSetting = setting
	switch {
	case paceEnv > 0:
		screenFrame = time.Duration(float64(time.Second) / paceEnv)
	case setting == "60" || setting == "30":
		n, _ := strconv.Atoi(setting)
		screenFrame = time.Second / time.Duration(n)
	default:
		SetScreenPace(displayHz)
	}
}

// displayRefresh finds how often the display refreshes, in Hz, or 0 when
// it can't be known: the compositor is asked (Hyprland, sway, or X11's
// xrandr). That runs a program.
func displayRefresh() float64 {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" {
		if out, err := exec.Command("hyprctl", "monitors", "-j").Output(); err == nil {
			return hyprRefresh(out)
		}
	}
	if os.Getenv("SWAYSOCK") != "" {
		if out, err := exec.Command("swaymsg", "-t", "get_outputs", "-r").Output(); err == nil {
			return swayRefresh(out)
		}
	}
	if os.Getenv("DISPLAY") != "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		if out, err := exec.Command("xrandr", "--current").Output(); err == nil {
			return xrandrRefresh(string(out))
		}
	}
	return 0
}

// hyprRefresh is the focused monitor's refresh rate in hyprctl's list.
func hyprRefresh(out []byte) float64 {
	var mons []struct {
		Focused     bool    `json:"focused"`
		RefreshRate float64 `json:"refreshRate"`
	}
	if json.Unmarshal(out, &mons) != nil {
		return 0
	}
	hz := 0.0
	for _, m := range mons {
		if m.Focused || hz == 0 {
			hz = m.RefreshRate
		}
	}
	return hz
}

// swayRefresh is the focused output's refresh rate in swaymsg's list.
func swayRefresh(out []byte) float64 {
	var outs []struct {
		Focused bool `json:"focused"`
		Mode    struct {
			Refresh int `json:"refresh"` // mHz
		} `json:"current_mode"`
	}
	if json.Unmarshal(out, &outs) != nil {
		return 0
	}
	hz := 0.0
	for _, o := range outs {
		if o.Focused || hz == 0 {
			hz = float64(o.Mode.Refresh) / 1000
		}
	}
	return hz
}

// xrandrRefresh is the rate xrandr marks as current ("143.98*+"), of the
// first screen that has one.
func xrandrRefresh(out string) float64 {
	for line := range strings.SplitSeq(out, "\n") {
		for f := range strings.FieldsSeq(line) {
			if i := strings.IndexByte(f, '*'); i > 0 {
				if hz, err := strconv.ParseFloat(f[:i], 64); err == nil {
					return hz
				}
			}
		}
	}
	return 0
}

// frameTick is the command that brings msg when the next frame on the beat
// kept in *due is due (see nextFrame). It waits for that moment itself, not
// for a span from when it was made: Bubble Tea runs a command only once the
// screen has been laid out, a little later after a heavy frame, and a
// frame timed from then came late after each heavy one and early after
// each light one (13.9 ms frames came 16.8, 11.0, 16.8 ms apart).
func frameTick(due *time.Time, d time.Duration, msg func() tea.Msg) tea.Cmd {
	nextFrame(due, d)
	at := *due
	return func() tea.Msg {
		if wait := time.Until(at); wait > 0 {
			time.Sleep(wait)
		}
		return msg()
	}
}

// nextFrame moves *at, when the last frame was due, on by d and returns how
// long to wait for it. Frames keep to a steady beat: one that comes late
// does not put the next ones off (waiting a frame from when the last came
// made every frame a little late, some 57 a second instead of 60, and the
// screen showed one of them twice now and then). A clock that has fallen
// a frame or more behind, or was not running, starts the beat from now.
func nextFrame(at *time.Time, d time.Duration) time.Duration {
	now := time.Now()
	next := at.Add(d)
	if next.Before(now.Add(-d)) || next.After(now.Add(d)) {
		next = now.Add(d)
	}
	*at = next
	return max(time.Millisecond/2, next.Sub(now))
}

// framesLog, with UTUIMP_FRAMES=<file>, notes every frame of every clock
// (the lines, the visualizer, the start screen, the words' effects) and
// every screen laid out: when, how long since the clock's last frame and
// how long it took, and any message that took long to handle. A line per
// frame: "ms clock gap work extra" (milliseconds). Buffered, so that noting
// costs the frames nothing to speak of.
var framesLog = newFrameLogger(os.Getenv("UTUIMP_FRAMES"))

type frameLogger struct {
	mu    sync.Mutex
	f     *os.File
	w     *bufio.Writer
	last  map[string]time.Time
	flush time.Time
}

func newFrameLogger(path string) *frameLogger {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // the user's own path
	if err != nil {
		return nil
	}
	return &frameLogger{f: f, w: bufio.NewWriterSize(f, 1<<16), last: map[string]time.Time{}}
}

// note notes a frame of clock that took work, ending now; the gap is from
// the start of the clock's last frame to this one's.
func (fl *frameLogger) note(clock string, work time.Duration, extra string) {
	if fl == nil {
		return
	}
	now := time.Now()
	began := now.Add(-work) // frames are timed from when they began
	fl.mu.Lock()
	defer fl.mu.Unlock()
	gap := 0.0
	if t, ok := fl.last[clock]; ok {
		gap = began.Sub(t).Seconds() * 1000
	}
	fl.last[clock] = began
	_, _ = fmt.Fprintf(fl.w, "%d %s %.2f %.2f %s\n", now.UnixMilli(), clock, gap, work.Seconds()*1000, extra)
	if now.Sub(fl.flush) > time.Second {
		_ = fl.w.Flush()
		fl.flush = now
	}
}
