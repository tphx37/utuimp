package tui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/gfx"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// The start screen draws the ribbons over the whole body, with the logo
// and the status line on top.
func TestIntro_RibbonsBehindTheLogo(t *testing.T) {
	m := coverModel(t, 80, 24)
	m.introStep = len(introFrames) - 1
	m.initStatus = "connecting…"
	if m.introBegin() == nil || m.intro == nil {
		t.Fatal("the ribbons should start with the start screen")
	}
	m.intro.start = time.Now().Add(-300 * time.Millisecond)
	m.intro.move(0.3, 1.0/30, 0) // in the middle of the splash
	frame := m.renderIntro()
	lines := strings.Split(frame, "\n")
	if len(lines) != 24 {
		t.Fatalf("the ribbons should fill the body: %d rows, want 24", len(lines))
	}
	plain := ansi.Strip(frame)
	if !strings.Contains(plain, "♪ utuimp") || !strings.Contains(plain, "connecting…") {
		t.Errorf("the logo and the status should be over the ribbons, got %q", plain)
	}
	if strings.Count(frame, "▀") < 80*4 {
		t.Errorf("the ribbons should be drawn, got %d half blocks", strings.Count(frame, "▀"))
	}
	// Where there is no ribbon the terminal's own background shows: no
	// colour of the picture's own to frame it. The corners are always so.
	for _, row := range []int{0, 23} {
		if l := lines[row]; strings.Contains(l, "48;2;") && strings.HasPrefix(ansi.Strip(l), "▀") {
			t.Errorf("row %d starts with a coloured cell: the picture has an edge", row)
		}
	}
}

// When the interface opens it grows out of the middle: half way through,
// the middle is the interface and the corners still the start screen.
func TestIntro_InterfaceOpensFromTheMiddle(t *testing.T) {
	m := coverModel(t, 80, 24)
	m.introStep = len(introFrames) - 1
	m.introBegin()
	m.intro.start = time.Now().Add(-2 * time.Second)
	m.intro.move(2, 1.0/30, 0)
	main := strings.Repeat(strings.Repeat("X", 80)+"\n", 23) + strings.Repeat("X", 80)
	m.intro.fadeAt = time.Now().Add(-time.Duration(introReveal * 0.5 * float64(time.Second)))
	lines := strings.Split(ansi.Strip(m.renderIntroDance("♪ utuimp", "", func() string { return main })), "\n")
	if len(lines) != 24 {
		t.Fatalf("%d rows, want 24", len(lines))
	}
	if mid := []rune(lines[(24-3)/2]); mid[40] != 'X' {
		t.Errorf("the middle should be the interface already, got %q", string(mid[35:45]))
	}
	if corner := []rune(lines[0]); corner[0] == 'X' {
		t.Error("the corner should still be the start screen")
	}
	m.intro.fadeAt = time.Now().Add(-time.Duration(introReveal * float64(time.Second)))
	full := ansi.Strip(m.renderIntroDance("♪ utuimp", "", func() string { return main }))
	if full != main {
		t.Error("when it has opened the screen should be the interface, exactly")
	}
}

// The interface's frame is read back cell by cell, colours and wide
// characters included.
func TestParseCells(t *testing.T) {
	frame := "\x1b[1;38;2;10;20;30mab\x1b[0m中c\n\x1b[44m d\x1b[49m"
	cells := parseCells(frame, 5, 2, [3]uint8{200, 200, 200}, [3]uint8{0, 0, 0})
	if c := cells[0][0]; c.ch != "a" || c.fg != [3]uint8{10, 20, 30} || c.sgr != "1;38;2;10;20;30" || !c.bgDefault {
		t.Errorf("first cell %+v", c)
	}
	if cells[0][2].ch != "中" || cells[0][3].ch != "" || cells[0][4].ch != "c" {
		t.Errorf("a wide character takes two cells: %q %q %q", cells[0][2].ch, cells[0][3].ch, cells[0][4].ch)
	}
	if c := cells[1][1]; c.ch != "d" || c.bgDefault || c.sgr != "44" {
		t.Errorf("second row %+v", c)
	}
}

// When the player is ready the ribbons fade out, and only then does the
// player show.
func TestIntro_FadesOutWhenReady(t *testing.T) {
	m := coverModel(t, 80, 24)
	m.introStep = 0
	m.introBegin()
	for range len(introFrames) + 2 {
		m.Update(introTickMsg(time.Now()))
	}
	if m.introStep == introDone {
		t.Fatal("the start screen should fade out, not vanish at once")
	}
	if m.intro.fadeAt.IsZero() {
		t.Fatal("the fade should have begun once the player was ready")
	}
	m.intro.fadeAt = time.Now().Add(-time.Second)
	m.Update(introVizTickMsg{})
	if m.introStep != introDone || m.intro != nil {
		t.Errorf("after the fade the start screen should be done (step %d)", m.introStep)
	}
}

// On the Linux console the logo stands alone.
func TestIntro_NoRibbonsOnTheConsole(t *testing.T) {
	styles.Console = true
	t.Cleanup(func() { styles.Console = false })
	m := coverModel(t, 80, 24)
	m.introStep = 0
	if m.introBegin() != nil || m.intro != nil {
		t.Error("no ribbons on the console")
	}
	for range len(introFrames) + 2 {
		m.Update(introTickMsg(time.Now()))
	}
	if m.introStep != introDone {
		t.Error("on the console the start screen ends at once when the player is ready")
	}
}

// "intro": false in the config skips the start screen: the player shows
// from the first frame.
func TestIntro_OffInTheConfig(t *testing.T) {
	cfg := testCfg()
	off := false
	cfg.Intro = &off
	m := New(cfg, &mockProvider{}, newMockPlayer(), Options{})
	m.Init()
	if m.introStep != introDone || m.intro != nil {
		t.Errorf("with the intro off there should be no start screen (step %d)", m.introStep)
	}
	if !testCfg().IntroEnabled() {
		t.Error("the intro is on unless the config turns it off")
	}
}

// In a terminal that shows pictures the start screen's ribbons are one,
// under the text: the cells carry only the logo, and the picture has a
// hole where the interface has opened.
func TestIntro_RibbonsAsAPicture(t *testing.T) {
	m := coverModel(t, 80, 24)
	m.lines.mode = gfx.Shm
	m.introStep = len(introFrames) - 1
	m.introBegin()
	m.intro.start = time.Now().Add(-300 * time.Millisecond)
	m.intro.move(0.3, 1.0/30, 0)
	frame := m.renderIntro()
	if strings.Contains(frame, "▀") {
		t.Error("no ribbons in the cells: they are a picture")
	}
	if !strings.Contains(ansi.Strip(frame), "♪ utuimp") {
		t.Error("the logo stays text")
	}
	d := m.intro
	if !strings.Contains(d.raw, "a=T,f=32") || !d.shown {
		t.Fatalf("no picture sent: %q", d.raw)
	}
	W, H := 80*introPxFor(80, 24), 24*2*introPxFor(80, 24)
	lit := 0
	for i := 0; i < len(d.pix); i += 4 {
		if d.pix[i+3] > 20 {
			lit++
		}
	}
	if len(d.pix) != W*H*4 || lit < W*H/50 {
		t.Fatalf("the ribbons should be in the picture: %d of %d pixels", lit, W*H)
	}
	_, screenBG, _ := styles.ScreenColors()
	for x := range W {
		for _, i := range []int{x * 4, ((H-1)*W + x) * 4} {
			if d.pix[i+3] != 0 {
				t.Fatal("the picture should fade away at the screen edges")
			}
		}
	}
	if dir := os.Getenv("LINES_PNG"); dir != "" {
		img := image.NewRGBA(image.Rect(0, 0, W, H))
		for i := 0; i < W*H; i++ {
			a := float64(d.pix[i*4+3]) / 255
			img.Set(i%W, i/W, color.RGBA{uint8(float64(d.pix[i*4])*a + float64(screenBG.R)*(1-a)), uint8(float64(d.pix[i*4+1])*a + float64(screenBG.G)*(1-a)), uint8(float64(d.pix[i*4+2])*a + float64(screenBG.B)*(1-a)), 255})
		}
		f, _ := os.Create(dir + "/intro.png") //nolint:gosec // a directory the tester asked for
		_ = png.Encode(f, img)
		_ = f.Close()
	}
	// As the interface opens, the intro picture fades through to it.
	d.fadeAt = time.Now().Add(-time.Duration(introReveal * 0.7 * float64(time.Second)))
	m.renderIntro()
	mid := ((H/2)*W + W/2) * 4
	if d.pix[mid+3] == 255 {
		t.Errorf("the picture should fade where the interface has opened: alpha %d", d.pix[mid+3])
	}
	// The end takes the picture away.
	d.fadeAt = time.Now().Add(-2 * time.Second)
	if cmd := m.introStepDance(); cmd == nil || m.intro != nil {
		t.Error("the start screen's end should take its picture off the screen")
	}
}

// TestIntroFrameCost reports how long a frame of the start screen takes at
// a full HD screen's size (INTRO_COST=1).
func TestIntroFrameCost(t *testing.T) {
	if os.Getenv("INTRO_COST") == "" {
		t.Skip("INTRO_COST=1 to measure")
	}
	m := coverModel(t, 192, 50)
	m.artCellAsp = 2.1
	m.lines.mode = gfx.Shm
	defer gfx.CleanOwn()
	m.introStep = len(introFrames) - 1
	m.introBegin()
	m.intro.start = time.Now().Add(-300 * time.Millisecond)
	const n = 60
	var ru0, ru1 syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru0)
	began := time.Now()
	for i := range n {
		m.intro.move(0.3+float64(i)/72, 1.0/72, 0)
		m.intro.frame = ""
		m.renderIntro()
	}
	wall := time.Since(began)
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru1)
	cpu := time.Duration(ru1.Utime.Nano()+ru1.Stime.Nano()-ru0.Utime.Nano()-ru0.Stime.Nano()) * time.Nanosecond
	t.Logf("a frame: %.1f ms, %.1f ms of processor (%dx%d pixels)", wall.Seconds()*1000/n, cpu.Seconds()*1000/n, m.intro.v.w, m.intro.v.h2)
	if dir := os.Getenv("LINES_PNG"); dir != "" {
		d := m.intro
		W, H := d.v.w, d.v.h2
		img := image.NewRGBA(image.Rect(0, 0, W, H))
		for i := 0; i < W*H; i++ {
			a := float64(d.pix[i*4+3]) / 255
			img.Set(i%W, i/W, color.RGBA{uint8(float64(d.pix[i*4])*a + 24*(1-a)), uint8(float64(d.pix[i*4+1])*a + 24*(1-a)), uint8(float64(d.pix[i*4+2])*a + 24*(1-a)), 255})
		}
		f, _ := os.Create(dir + "/intro-full.png") //nolint:gosec // a directory the tester asked for
		_ = png.Encode(f, img)
		_ = f.Close()
	}
}
