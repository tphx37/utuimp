package tui

import (
	"math"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/gfx"
)

// The quick forms of e^x, the tone curve and a Gaussian down a column keep
// to the exact ones far closer than a colour can show.
func TestQuickMath(t *testing.T) {
	for q := 0.0; q < 30; q += 0.0071 {
		if d := math.Abs(expNeg(q)-math.Exp(-q)) / math.Exp(-q); d > 3e-6 {
			t.Fatalf("expNeg(%v) off by %.2g of the value", q, d)
		}
	}
	for c := -0.5; c < 1.5; c += 0.00013 {
		want := 0.0
		if c > 0 {
			want = (1 - math.Exp(-c*1.4)) * 255 / 0.75
		}
		if got := toneOf(c); math.Min(got, 255) != math.Min(want, 255) && math.Abs(got-want) > 1e-4 {
			t.Fatalf("toneOf(%v) = %v, want %v", c, got, want)
		}
	}
	g := make([]float64, 60)
	for _, c := range []float64{-7, 0, 13.3, 29.5, 59.9, 80} {
		for _, s := range []float64{0.8, 2.5, 6} {
			gaussRow(g, c, s, math.Exp(-2/(s*s)))
			for y, v := range g {
				if want := math.Exp(-sq((float64(y) - c) / s)); math.Abs(v-want) > 1e-9 {
					t.Fatalf("gaussRow c=%v s=%v: row %d is %v, want %v", c, s, y, v, want)
				}
			}
		}
	}
}

// A thin line is sent without its clear rows and columns.
func TestAlphaBounds(t *testing.T) {
	const w, h = 10, 8
	pix := make([]byte, w*h*4)
	if y0, y1 := alphaRows(pix, w, h); y1 > y0 {
		t.Error("an empty picture has no rows")
	}
	pix[(3*w+2)*4+3], pix[(5*w+6)*4+3] = 9, 9
	if y0, y1 := alphaRows(pix, w, h); y0 != 3 || y1 != 6 {
		t.Errorf("rows %d..%d, want 3..6", y0, y1)
	}
	if x0, x1 := alphaCols(pix, w, h); x0 != 2 || x1 != 7 {
		t.Errorf("columns %d..%d, want 2..7", x0, x1)
	}
}

// The lyrics go in bands, and only the bands that changed are sent again.
func TestLyricsBands(t *testing.T) {
	l := newUILines()
	l.mode, l.cellW, l.cellH = gfx.Shm, 4, 4
	st := &lineStrip{col: 2, row: 3, cols: 5, rows: 8}
	w, h := 5*4, 8*4
	pix := make([]byte, w*h*4)
	now := time.Now()
	first := l.sendBands(pix, w, h, st, now, false)
	if n := strings.Count(first, "a=T"); n != 4 {
		t.Fatalf("eight rows in bands of %d: %d pictures sent, want 4", lyricBandRows, n)
	}
	if again := l.sendBands(pix, w, h, st, now, false); again != "" {
		t.Error("nothing changed: nothing is sent")
	}
	pix[(20*w+3)*4+3] = 200 // the third band
	if n := strings.Count(l.sendBands(pix, w, h, st, now, false), "a=T"); n != 1 {
		t.Errorf("one band changed: %d sent", n)
	}
	if n := strings.Count(l.sendBands(pix, w, h, st, now, true), "a=T"); n != 4 {
		t.Errorf("all again when asked: %d sent", n)
	}
	if strings.Count(l.bandsDelete(), "a=d") != 4 {
		t.Error("all four taken off the screen")
	}
}

// A frame of the lines that changes nothing in the cells shows the screen
// laid out last; so does the program handing a terminal sequence back.
func TestViewReuse(t *testing.T) {
	m := coverModel(t, 120, 36)
	m.coverView = true
	m.lines.mode = gfx.Shm
	m.lines.cellW, m.lines.cellH = 9, 18
	m.cfg.UIMotion = config.MotionCalm
	m.noAnim = false
	m.lines.ticking = true
	_ = m.View()
	for range 30 { // whatever was coming up has come
		m.soft = nil
		_ = m.View()
	}
	m.viewSec = int(m.smoothPosition().Seconds())
	m.Update(linesTickMsg{m.lines.gen})
	made := m.lines.views
	_ = m.View()
	m.Update(tea.RawMsg{Msg: "x"})
	_ = m.View()
	m.Update(linesTickMsg{m.lines.gen})
	_ = m.View()
	if m.lines.views != made {
		t.Errorf("a frame of the lines laid the screen out anew (%d times), why: %q", m.lines.views-made, m.lines.why)
	}
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	_ = m.View()
	if m.lines.views == made {
		t.Error("a key lays the screen out anew")
	}
}

// With nothing playing and nothing moving the lines' clock slows to a
// watch, and anything but a tick brings it back.
func TestLinesWatch(t *testing.T) {
	m := coverModel(t, 120, 36)
	m.coverView = true
	m.lines.mode = gfx.Shm
	m.lines.cellW, m.lines.cellH = 9, 18
	m.playerState.Playing = false
	_ = m.View()
	l := m.lines
	l.ticking = true
	long := time.Now().Add(-time.Minute) // everything came long ago
	m.linesFrameMsg(linesTickMsg{l.gen})
	for i := range l.strips {
		l.strips[i].born, l.strips[i].cameAt = long, long
	}
	l.coverAt, l.covAt, l.labelsAt, m.soft = long, long, long, nil
	for range 200 {
		m.linesFrameMsg(linesTickMsg{l.gen})
	}
	if l.quiet < linesQuietAfter {
		t.Fatalf("nothing moving: quiet for %d frames", l.quiet)
	}
	gen := l.gen
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if l.quiet != 0 || l.gen == gen {
		t.Error("a key wakes the lines' clock")
	}
	l.quiet = linesQuietAfter
	gen = l.gen
	m.Update(glowTickMsg(time.Now()))
	if l.gen != gen {
		t.Error("a clock's tick does not")
	}
}
