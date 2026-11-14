package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/gfx"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
)

// fxModel is a player with a queue on screen and the effects at level.
func fxModel(t *testing.T, level string) *Model {
	t.Helper()
	m := newModel(nil)
	m.cfg.UIMotion = level
	m.noAnim = false
	m.introStep = introDone
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	var tracks []provider.Track
	for i := range 12 {
		tracks = append(tracks, provider.Track{ID: fmt.Sprint(i), Title: fmt.Sprintf("Song number %d", i+1), Artist: "Some Artist"})
	}
	m.queue.m.SetTracks(tracks)
	m.queueTracks = tracks
	m.queueMiniCursor = 3
	return m
}

// fxRun lets the frame clock run for d, as the program would.
func fxRun(m *Model, d time.Duration) {
	end := time.Now().Add(d)
	for time.Now().Before(end) && m.fx != nil && m.fx.ticking {
		m.Update(fxTickMsg{m.fx.gen})
		time.Sleep(screenFrame)
	}
}

func TestFX_NothingMovesOnKeys(t *testing.T) {
	m := fxModel(t, config.MotionLively)
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.fx != nil && m.fx.ticking {
		t.Error("moving the cursor should animate nothing")
	}
}

func TestFX_PanelComesUpAndSettles(t *testing.T) {
	m := fxModel(t, config.MotionLively)
	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.fx == nil || !m.fx.ticking {
		t.Fatal("opening a panel should start the frame clock")
	}
	m.Update(fxTickMsg{m.fx.gen})
	if got := m.View().Content; got == m.renderMain() {
		t.Error("the panel's text should be coming up")
	} else if ansi.Strip(got) != ansi.Strip(m.renderMain()) {
		t.Error("fading must not move or change any text")
	}
	fxRun(m, time.Second)
	if m.fx.ticking {
		t.Error("the clock should stop once it settles")
	}
	if got := m.View().Content; got != m.renderMain() {
		t.Error("a settled screen should be drawn exactly as without effects")
	}
}

func TestFX_OffAndInTests(t *testing.T) {
	m := fxModel(t, config.MotionOff)
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.fx != nil && m.fx.ticking {
		t.Error("off: no animation")
	}
	m = fxModel(t, config.MotionWild)
	m.noAnim = true
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.fx != nil && m.fx.ticking {
		t.Error("noAnim: no animation")
	}
}

func TestFX_NewSongSlidesIn(t *testing.T) {
	m := fxModel(t, config.MotionWild)
	m.Update(playerStateMsg(player.State{Track: &provider.Track{ID: "x", Title: "New One", Artist: "A"}}))
	if m.fx == nil || !m.fx.ticking {
		t.Fatal("a new song should start the sweep")
	}
	time.Sleep(300 * time.Millisecond)
	m.Update(fxTickMsg{m.fx.gen})
	if m.View().Content == m.renderMain() {
		t.Error("the sweep should colour the screen")
	}
}

func TestEncodeCells_RoundTrip(t *testing.T) {
	m := fxModel(t, config.MotionOff)
	frame := m.renderMain()
	cells := parseCells(frame, m.width, m.height, [3]uint8{200, 200, 200}, [3]uint8{})
	back := encodeCells(cells, m.width, m.height)
	if ansi.Strip(back) != ansi.Strip(frame) {
		t.Errorf("text changed:\n%s\n---\n%s", ansi.Strip(frame), ansi.Strip(back))
	}
}

// TestFX_Dump writes frames of each effect when FX_DUMP names a directory
// (to look at them, see the scratch renderer).
func TestFX_Dump(t *testing.T) {
	dir := os.Getenv("FX_DUMP")
	if dir == "" {
		t.Skip("FX_DUMP not set")
	}
	m := fxModel(t, config.MotionWild)
	save := func(name string) {
		_ = os.WriteFile(filepath.Join(dir, name+".ans"), []byte(m.View().Content), 0o600) //nolint:gosec // a directory the tester asked for
	}
	run := func(name string, n int) {
		for i := range n {
			m.Update(fxTickMsg{m.fx.gen})
			save(fmt.Sprintf("%s-%02d", name, i))
			time.Sleep(screenFrame)
		}
		fxRun(m, 2*time.Second)
	}
	m.Update(playerStateMsg(player.State{Track: &provider.Track{ID: "x", Title: "New One", Artist: "A"}}))
	run("track", 20)
	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	run("wipe", 16)
}

func TestFX_RowsKeepTheirWidth(t *testing.T) {
	m := fxModel(t, config.MotionWild)
	m.Update(playerStateMsg(player.State{Track: &provider.Track{ID: "x", Title: "Wide 漢字 🅴 title", Artist: "A"}, Playing: true}))
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	for i := range 30 {
		m.Update(fxTickMsg{m.fx.gen})
		for n, line := range strings.Split(m.View().Content, "\n") {
			if w := ansi.StringWidth(line); w != m.width {
				t.Fatalf("frame %d row %d is %d wide, want %d: %q", i, n, w, m.width, ansi.Strip(line))
			}
		}
		time.Sleep(screenFrame / 2)
	}
}

func TestFX_PanelIsFoundRightOfTheCover(t *testing.T) {
	frame := "╭──╮ ╭───╮\n│♪ │ │abc│\n╰──╯ ╰───╯"
	cells := parseCells(frame, 10, 3, [3]uint8{}, [3]uint8{})
	if r, ok := fxPanel(cells, 0, 4); !ok || r != (fxRect{6, 1, 9, 2}) {
		t.Errorf("got %v %v, want the inside of the right frame", r, ok)
	}
}

func TestCoverStatusRows_SameInEveryPanel(t *testing.T) {
	m := fxModel(t, config.MotionOff)
	m.coverView = true
	m.Update(tea.WindowSizeMsg{Width: 104, Height: 30})
	want := len(m.coverStatusLines())
	for p := -1; p < len(m.panels); p++ {
		m.activePanel = p
		if got := len(m.coverStatusLines()); got != want {
			t.Errorf("panel %d: %d status rows, want %d", p, got, want)
		}
	}
	m.activePanel, m.queueMiniCursor = -1, 0
	if got := len(m.coverStatusLines()); got != want {
		t.Errorf("queue cursor: %d status rows, want %d", got, want)
	}
}

// Where the lines are pictures, new words come up out of the background and
// old ones fade into it; words that only moved, and a changed letter or
// two, are shown at once.
func TestFxSoft(t *testing.T) {
	m := coverModel(t, 40, 6)
	m.lines.mode = gfx.Shm
	m.cfg.UIMotion = config.MotionCalm
	m.noAnim = false
	frame := func(rows ...string) string {
		for len(rows) < 6 {
			rows = append(rows, "")
		}
		for i := range rows {
			rows[i] += strings.Repeat(" ", 40-len([]rune(rows[i])))
		}
		return strings.Join(rows, "\n")
	}
	first := frame("\x1b[38;2;200;200;200mhello there world\x1b[0m", "0:05")
	if got := m.fxSoft(first); got != first {
		t.Fatal("the first frame is shown as it is")
	}
	if got := m.fxSoft(first); got != first {
		t.Error("nothing changed: nothing is touched")
	}
	// New words arrive in a sweep, each letter whole: at first only the
	// start of them is there, in its own colour, and a moment later all.
	next := frame("\x1b[38;2;200;200;200mother words here!\x1b[0m", "0:06")
	got := m.fxSoft(next)
	if got == next {
		t.Fatal("new words should not all be there at once")
	}
	if strings.Contains(ansi.Strip(got), "other words here!") || !strings.Contains(ansi.Strip(got), "0:06") {
		t.Errorf("only the start of the new words yet, and the clock at once: %q", ansi.Strip(got))
	}
	if strings.Count(got, "38;2;") != strings.Count(got, "38;2;200;200;200") {
		t.Error("letters are never shown in a colour that is not theirs")
	}
	time.Sleep(time.Duration((softIn + 0.03) * float64(time.Second)))
	if got := m.fxSoft(next); ansi.Strip(got) != ansi.Strip(next) {
		t.Errorf("a moment later the words are all there: %q", ansi.Strip(got))
	}
	// Words that went are swept away, not gone at once.
	gone := m.fxSoft(frame("", "0:06"))
	if !strings.Contains(ansi.Strip(gone), "words here!") {
		t.Errorf("words that went should still be going: %q", ansi.Strip(gone))
	}
	// The same rows moved by one: as they are.
	m.soft = nil
	a := frame("one one one one", "two two two two", "three three three")
	m.fxSoft(a)
	b := frame("two two two two", "three three three", "four four four four")
	out := m.fxSoft(b)
	rows := strings.Split(out, "\n")
	if strings.Contains(rows[0], "38;2;") || strings.Contains(rows[1], "38;2;") {
		t.Errorf("rows that only moved are not faded: %q", rows[:2])
	}
	// Off with the simple style.
	m.cfg.UIStyle = config.StyleSimple
	if got := m.fxSoft(next); got != next {
		t.Error("the simple style changes text at once")
	}
}

// Words that stay but change colour go from the one to the other, never
// through the background, and end in their own.
func TestFxSoft_Colour(t *testing.T) {
	m := coverModel(t, 40, 3)
	m.lines.mode = gfx.Shm
	m.cfg.UIMotion = config.MotionCalm
	m.noAnim = false
	row := func(r, g, b int) string {
		s := fmt.Sprintf("\x1b[38;2;%d;%d;%dmchosen row here\x1b[0m", r, g, b)
		return s + strings.Repeat(" ", 40-15) + "\n" + strings.Repeat(" ", 40) + "\n" + strings.Repeat(" ", 40)
	}
	m.fxSoft(row(200, 200, 200))
	red := row(240, 40, 40)
	got := m.fxSoft(red)
	if got == red {
		t.Fatal("the colour should change over, not at once")
	}
	if ansi.Strip(got) != ansi.Strip(red) {
		t.Error("the letters stay as they are")
	}
	cells := parseCells(got, 40, 3, [3]uint8{}, [3]uint8{})
	c := cells[0][0].fg
	if c[0] < 200 || c[1] < 40 || c[1] > 200 || c == [3]uint8{240, 40, 40} {
		t.Errorf("between the two colours, not dark: %v", c)
	}
	time.Sleep(time.Duration((softTint + 0.03) * float64(time.Second)))
	if got := m.fxSoft(red); got != red {
		t.Error("a moment later it is its own colour")
	}
}

// A key held down (a list run through) shows the text as it is, at once,
// with nothing left to come up or change colour when it is let go.
func TestFxSoft_KeyHeld(t *testing.T) {
	m := coverModel(t, 40, 6)
	m.lines.mode = gfx.Shm
	m.cfg.UIMotion = config.MotionCalm
	m.noAnim = false
	frame := func(rows ...string) string {
		for len(rows) < 6 {
			rows = append(rows, "")
		}
		for i := range rows {
			rows[i] += strings.Repeat(" ", 40-ansi.StringWidth(rows[i]))
		}
		return strings.Join(rows, "\n")
	}
	m.fxSoft(frame("\x1b[38;2;200;200;200mfirst song of the list\x1b[0m"))
	m.noteKeyRepeat("j")
	m.noteKeyRepeat("j") // repeated: held
	next := frame("\x1b[38;2;90;200;90meleventh song further on\x1b[0m")
	if got := m.fxSoft(next); got != next {
		t.Errorf("held, the text should be as it is: %q", ansi.Strip(got))
	}
	// Let go: the next frame has nothing on its way.
	m.heldAt = time.Now().Add(-time.Second)
	if got := m.fxSoft(next); got != next || m.soft.busy {
		t.Error("let go, nothing should be left to come up")
	}
	// A single press is not a held key.
	m.repeatKey, m.repeatAt, m.heldAt = "", time.Time{}, time.Time{}
	m.noteKeyRepeat("j")
	if m.keyHeld(time.Now()) {
		t.Error("one press is not a held key")
	}
}

// Play and pause pressed again and again: each a touch of its own, so the
// progress line ripples at every one.
func TestTouchPlayEveryPress(t *testing.T) {
	l := newUILines()
	l.touch(touchPlay, 0)
	l.touch(touchPlay, 0)
	l.touch(touchPlay, 0)
	n := 0
	for _, tc := range l.touches {
		if tc.kind == touchPlay && tc.heldAt.IsZero() {
			n++
		}
	}
	if n != 3 {
		t.Errorf("%d touches of play, want 3, none held", n)
	}
	l.touch(touchScroll, 1)
	l.touch(touchScroll, 1)
	if len(l.touches) != 4 {
		t.Errorf("a list run through is one touch, held: %d touches", len(l.touches))
	}
}
