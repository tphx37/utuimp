package tui

import (
	"fmt"
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

// With nothing playing the cover view keeps its shape: a frame where the
// cover goes, a greyed song block and hints instead of "no lyrics".
func TestCoverView_Idle(t *testing.T) {
	m := newModel(newMockPlayer())
	m.width, m.height = 120, 36
	m.introStep = introDone
	m.toggleCoverView()
	frame := ansi.Strip(m.View().Content)
	if os.Getenv("COVER_DUMP") != "" {
		t.Log("\n" + frame)
	}
	for _, want := range []string{"╭", "♪", "Nothing playing", "search"} {
		if !strings.Contains(frame, want) {
			t.Errorf("idle cover view lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "no lyrics found") {
		t.Errorf("idle cover view shouldn't talk about lyrics:\n%s", frame)
	}
	lines := strings.Split(frame, "\n")
	if strings.TrimSpace(lines[0]) != "" {
		t.Errorf("the first row should be padding, got %q", lines[0])
	}

	// '/' opens search over the lyrics; esc goes back to them.
	m.handleNormalKey(tea.KeyPressMsg{Code: '/', Text: "/"}, "/")
	if m.mode != modeSearch {
		t.Fatal("/ should open search in the cover view")
	}
}

// The lyrics stay between the cover's top and bottom edges, with the sung
// line about a third of the way down and the earlier ones above it.
func TestCoverView_LyricsBetweenCoverEdges(t *testing.T) {
	m := coverModel(t, 140, 40)
	var ls []lyrics.Line
	for i := range 40 {
		ls = append(ls, lyrics.Line{Start: time.Duration(i) * 5 * time.Second,
			Text: fmt.Sprintf("line %02d and a few more words to make it wrap on narrow screens", i)})
	}
	m.lyricsP.m.SetLyrics(&lyrics.Result{Lines: ls, Synced: true}, nil)
	m.lyricsP.m.SetPosition(101 * time.Second) // line 20
	m.toggleCoverView()
	frame := ansi.Strip(m.View().Content)
	if os.Getenv("COVER_DUMP") != "" {
		t.Log("\n" + frame)
	}
	rows := strings.Split(frame, "\n")
	coverTop, coverBottom, cur := -1, -1, -1
	for i, r := range rows {
		if strings.Contains(r, "▀") || strings.Contains(r, "▄") {
			if coverTop < 0 {
				coverTop = i
			}
			coverBottom = i
		}
		if strings.Contains(r, "line 20") {
			cur = i
		}
	}
	if coverTop < 0 || cur < 0 {
		t.Fatalf("cover rows %d–%d, current line at %d:\n%s", coverTop, coverBottom, cur, frame)
	}
	for i, r := range rows {
		if strings.Contains(r, "line ") && (i < coverTop || i > coverBottom) {
			t.Errorf("lyrics row %d outside the cover's edges %d–%d: %q", i, coverTop, coverBottom, r)
		}
	}
	if cur < coverTop+2 || cur > coverTop+(coverBottom-coverTop)/2 {
		t.Errorf("current line at row %d, want in the upper part of %d–%d", cur, coverTop, coverBottom)
	}
	if !strings.Contains(frame, "line 19") {
		t.Error("the line just sung should still show above the current one")
	}
}

// Every key the cover view's hints list does something visible there.
func TestCoverView_HintedKeysWork(t *testing.T) {
	keys := []struct {
		k    string
		code rune
	}{
		{"/", '/'}, {"l", 'l'}, {"q", 'q'}, {"F", 'F'}, {"esc", tea.KeyEscape}, {"e", 'e'},
		{"i", 'i'}, {"j", 'j'}, {"t", 't'}, {"T", 'T'}, {"c", 'c'}, {"v", 'v'}, {":", ':'}, {"?", '?'},
	}
	for _, key := range keys {
		t.Run(key.k, func(t *testing.T) {
			m := coverModel(t, 140, 40)
			var ls []lyrics.Line
			for i := range 30 {
				ls = append(ls, lyrics.Line{Start: time.Duration(i) * time.Second, Text: fmt.Sprintf("words %02d", i)})
			}
			m.lyricsP.m.SetLyrics(&lyrics.Result{Lines: ls, Synced: true}, nil)
			m.lyricsP.m.SetPosition(10500 * time.Millisecond)
			m.toggleCoverView()
			before := ansi.Strip(m.View().Content)
			msg := tea.KeyPressMsg{Code: key.code}
			if key.code != tea.KeyEscape {
				msg.Text = key.k
			}
			cmd := m.handleKey(msg)
			if key.k == "t" && cmd != nil {
				// t asks the model to translate: run that request.
				m.Update(cmd())
			}
			after := ansi.Strip(m.View().Content)
			if after == before {
				t.Errorf("%q changed nothing on screen:\n%s", key.k, after)
			}
		})
	}
}
