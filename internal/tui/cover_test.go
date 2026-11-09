package tui

import (
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/lyrics"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/art"
)

func coverModel(t *testing.T, w, h int) *Model {
	t.Helper()
	m := newModel(newMockPlayer())
	m.width, m.height = w, h
	m.introStep = introDone
	m.supportsArtColor = func() bool { return true }
	tr := &provider.Track{ID: "1", Title: "Night Song", Artist: "Band", Album: "Record",
		Duration: 3 * time.Minute, ArtworkURL: "http://cover"}
	m.playerState.Track = tr
	m.playerState.Playing = true
	m.queueTracks = []provider.Track{*tr, {ID: "2", Title: "Next One", Artist: "Band"}}
	m.queue.SetTracks(m.queueTracks)
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.Black)
	m.artwork = artworkCache{url: tr.ArtworkURL, img: img, rendered: map[art.Size][]string{}}
	m.lyricsP.m.SetLyrics(&lyrics.Result{Lines: []lyrics.Line{{Text: "first line"}, {Text: "second line"}}}, nil)
	m.lastLyricsTrackID = "1"
	return m
}

func TestCoverView_ToggleAndPanels(t *testing.T) {
	m := coverModel(t, 120, 36)
	m.handleNormalKey(tea.KeyPressMsg{Code: 'v', Text: "v"}, "v")
	if !m.coverView || m.cfg.Layout != LayoutCover {
		t.Fatal("v should switch to the cover view and remember it")
	}
	frame := ansi.Strip(m.View().Content)
	if os.Getenv("COVER_DUMP") != "" {
		t.Log("\n" + frame)
	}
	for _, want := range []string{"Night Song", "second line"} {
		if !strings.Contains(frame, want) {
			t.Errorf("cover view lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "╭") {
		t.Errorf("lyrics in the cover view should have no frame:\n%s", frame)
	}
	if lines := strings.Split(frame, "\n"); len(lines) != m.height {
		t.Errorf("frame has %d rows, want %d", len(lines), m.height)
	}

	// esc on the lyrics shows the queue in a frame; esc again goes back.
	m.handleNormalKey(tea.KeyPressMsg{Code: tea.KeyEscape}, "esc")
	frame = ansi.Strip(m.View().Content)
	if os.Getenv("COVER_DUMP") != "" {
		t.Log("\n" + frame)
	}
	if !strings.Contains(frame, "╭") || !strings.Contains(frame, "Next One") {
		t.Errorf("esc should show the framed queue:\n%s", frame)
	}
	m.handleNormalKey(tea.KeyPressMsg{Code: tea.KeyEscape}, "esc")
	if !m.lyricsVisible() {
		t.Error("esc on the queue should return to the lyrics")
	}

	// Another panel replaces the lyrics; closing it returns to them.
	m.handleNormalKey(tea.KeyPressMsg{Code: 'e', Text: "e"}, "e")
	if m.lyricsVisible() {
		t.Fatal("e should open the equalizer")
	}
	m.handleNormalKey(tea.KeyPressMsg{Code: 'e', Text: "e"}, "e")
	if !m.lyricsVisible() {
		t.Error("closing a panel in the cover view should return to the lyrics")
	}

	m.handleNormalKey(tea.KeyPressMsg{Code: 'v', Text: "v"}, "v")
	if m.coverView || m.activePanel != -1 {
		t.Error("v again should restore the boxed layout with the queue")
	}
}

func TestCoverView_NarrowStacks(t *testing.T) {
	m := coverModel(t, 60, 30)
	m.toggleCoverView()
	frame := ansi.Strip(m.View().Content)
	lines := strings.Split(frame, "\n")
	if len(lines) != m.height {
		t.Errorf("frame has %d rows, want %d:\n%s", len(lines), m.height, frame)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > m.width {
			t.Errorf("row wider than the terminal: %q", l)
		}
	}
}
