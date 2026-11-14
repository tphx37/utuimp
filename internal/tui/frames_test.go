package tui

import (
	"math"
	"testing"
	"time"
)

// Frames keep to the beat: a late one does not put the next ones off, and
// a clock far behind or just started begins from now.
func TestNextFrame(t *testing.T) {
	var due time.Time
	if w := nextFrame(&due, screenFrame); w < screenFrame-time.Millisecond || w > screenFrame {
		t.Fatalf("a clock just started waits a frame, not %v", w)
	}
	// Woken 3 ms late for that frame: the next one is still due a frame
	// after it was, so the wait is that much shorter.
	due = time.Now().Add(-3 * time.Millisecond)
	first := due
	w := nextFrame(&due, screenFrame)
	if due != first.Add(screenFrame) {
		t.Errorf("the next frame is due %v after the last, want one frame", due.Sub(first))
	}
	if w > screenFrame-2*time.Millisecond {
		t.Errorf("waits %v: the lateness should come off the wait", w)
	}
	// A second behind: from now.
	due = time.Now().Add(-time.Second)
	if w := nextFrame(&due, screenFrame); w < screenFrame-time.Millisecond {
		t.Errorf("far behind: waits %v, want a frame from now", w)
	}
}

// The screen's pace fits the display's refresh: each frame lasts a whole
// number of refreshes, as near sixty a second as that goes.
func TestScreenFPS(t *testing.T) {
	for hz, want := range map[float64]float64{
		0: 60, 60: 60, 59.94: 59.94, 75: 75, 90: 90, 100: 50, 120: 60,
		143.98: 71.99, 144: 72, 165: 82.5, 240: 60, 360: 60,
	} {
		if got := screenFPS(hz); math.Abs(got-want) > 0.01 {
			t.Errorf("%v Hz: %v frames a second, want %v", hz, got, want)
		}
	}
}

func TestDisplayRefreshParsers(t *testing.T) {
	hypr := `[{"name":"HDMI-A-1","focused":false,"refreshRate":60.0},{"name":"eDP-1","focused":true,"refreshRate":144.003}]`
	if hz := hyprRefresh([]byte(hypr)); math.Abs(hz-144.003) > 0.001 {
		t.Errorf("hyprctl: %v", hz)
	}
	sway := `[{"focused":false,"current_mode":{"refresh":60000}},{"focused":true,"current_mode":{"refresh":165000}}]`
	if hz := swayRefresh([]byte(sway)); hz != 165 {
		t.Errorf("swaymsg: %v", hz)
	}
	xr := "Screen 0: minimum 8 x 8\neDP-1 connected primary 1920x1080+0+0\n   1920x1080    143.98*+  60.01  \n   1280x720     60.00\n"
	if hz := xrandrRefresh(xr); hz != 143.98 {
		t.Errorf("xrandr: %v", hz)
	}
}
