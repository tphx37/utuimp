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
		time.Sleep(fxFrame)
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
		_ = os.WriteFile(filepath.Join(dir, name+".ans"), []byte(m.View().Content), 0o600)
	}
	run := func(name string, n int) {
		for i := range n {
			m.Update(fxTickMsg{m.fx.gen})
			save(fmt.Sprintf("%s-%02d", name, i))
			time.Sleep(fxFrame)
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
		time.Sleep(fxFrame / 2)
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
