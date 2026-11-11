package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

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
	if strings.Count(frame, "▀") < 80*6 {
		t.Error("the ribbons should be drawn")
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
