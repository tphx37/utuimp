package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tphx37/utuimp/internal/lyrics"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

func TestLyricsModel_WordHighlightingAndSeek(t *testing.T) {
	m := NewLyrics()
	m.SetSize(60, 10)
	m.SetLyrics(&lyrics.Result{
		Synced: true,
		Lines: []lyrics.Line{
			{
				Start: 5 * time.Second,
				Text:  "first timed line",
				Words: []lyrics.Word{
					{Start: 5 * time.Second, Text: "first "},
					{Start: 6 * time.Second, Text: "timed "},
					{Start: 7 * time.Second, Text: "line"},
				},
			},
			{
				Start: 10 * time.Second,
				Text:  "second timed line",
			},
			{
				Start: 15 * time.Second,
				Text:  "third timed line",
			},
		},
	}, nil)

	m.SetPosition(6500 * time.Millisecond)
	view := m.View()
	for _, word := range []string{"first", "timed", "line"} {
		if !strings.Contains(view, word) {
			t.Errorf("view missing word %q:\n%s", word, view)
		}
	}

	// Move cursor down to "second timed line" (unfollows active line).
	_ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	view = m.View()
	if !strings.Contains(view, "▸ second timed line") {
		t.Errorf("expected selected indicator on second line, got:\n%s", view)
	}

	// Press Enter to seek to the selected line.
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected non-nil command on Enter")
	}
	msg := cmd()
	seekMsg, ok := msg.(LyricsSeekMsg)
	if !ok {
		t.Fatalf("expected LyricsSeekMsg, got %T", msg)
	}
	if seekMsg.Position != 10*time.Second {
		t.Errorf("seek position = %v, want 10s", seekMsg.Position)
	}
}

func TestLyricsModel_BottomScrollAndResumeFollow(t *testing.T) {
	lines := make([]lyrics.Line, 20)
	for i := range lines {
		lines[i] = lyrics.Line{
			Start: time.Duration(i) * time.Second,
			Text:  "line-" + string(rune('A'+i)),
		}
	}
	m := NewLyrics()
	m.SetSize(40, 10) // visibleRows = 8
	m.SetLyrics(&lyrics.Result{Synced: true, Lines: lines}, nil)
	m.SetPosition(0)

	// Jump to bottom with G: last line (index 19, "line-T") must be visible.
	_ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	view := m.View()
	if !strings.Contains(view, "▸ line-T") {
		t.Errorf("expected last line visible after G, got:\n%s", view)
	}

	// Back() (triggered on Esc) should re-attach follow mode without closing.
	if !m.Back() {
		t.Error("expected Back() to return true when detached from follow")
	}
	if m.Back() {
		t.Error("expected second Back() to return false when already following")
	}

	// Detach again onto line 2 ("line-C"); when playback reaches line 2, follow resumes.
	_ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.follow {
		t.Fatal("expected follow=false after scrolling ahead")
	}
	m.SetPosition(2 * time.Second)
	if !m.follow {
		t.Error("expected follow to resume when active line catches up to cursor")
	}
}

func TestLyricsModel_TranslationRowsAndPicker(t *testing.T) {
	m := NewLyrics()
	m.SetSize(60, 10)
	m.SetLyrics(&lyrics.Result{Synced: true, Source: lyrics.SourceNetEase, WordSynced: true, Lines: []lyrics.Line{
		{Start: time.Second, Text: "bonjour"},
		{Start: 2 * time.Second, Text: "au revoir"},
	}}, nil)
	m.SetPosition(1500 * time.Millisecond)

	cmd := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if cmd == nil {
		t.Fatal("turning translation on should request a translation")
	}
	if _, ok := cmd().(LyricsTranslateRequestMsg); !ok {
		t.Fatal("expected LyricsTranslateRequestMsg")
	}
	m.SetTranslating("ru")
	m.SetTranslation([]string{"привет", "до свидания"}, nil)
	view := m.View()
	for _, want := range []string{"NetEase", "word sync", "⇄ ru", "привет", "до свидания"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	// Short lines keep their translation on the same row, flush right.
	for row := range strings.SplitSeq(stripANSI(view), "\n") {
		if strings.Contains(row, "bonjour") {
			if !strings.HasSuffix(row, "привет") || len([]rune(row)) != 60 {
				t.Errorf("translation not right-aligned on the lyric's row: %q", row)
			}
		}
	}

	_ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if !m.PickerOpen() || !strings.Contains(m.View(), "lyrics.ovh") {
		t.Fatalf("picker not shown:\n%s", m.View())
	}
	cmd = m.Update(tea.KeyPressMsg{Code: '3', Text: "3"}) // 1 auto, 2 service, 3 LRCLIB
	sel, ok := cmd().(LyricsSourceSelectedMsg)
	if !ok || sel.Source != lyrics.SourceLRCLIB || m.PickerOpen() {
		t.Errorf("picking 3 = %+v, picker open %v", sel, m.PickerOpen())
	}

	// The picker works even when nothing was found.
	m.SetLyrics(nil, fmt.Errorf("lyrics not found"))
	_ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if !m.PickerOpen() {
		t.Error("picker should open when no lyrics were found")
	}
	if !m.Back() || m.PickerOpen() {
		t.Error("esc should close the picker")
	}
}

func TestLyricsModel_ActiveWordFillsProgressively(t *testing.T) {
	m := NewLyrics()
	m.SetSize(60, 10)
	line := lyrics.Line{
		Start: 0, Text: "abcd efgh",
		Words: []lyrics.Word{{Start: 0, Dur: time.Second, Text: "abcd "}, {Start: time.Second, Dur: time.Second, Text: "efgh"}},
	}
	m.SetLyrics(&lyrics.Result{Synced: true, WordSynced: true, Lines: []lyrics.Line{line}}, nil)

	for _, tc := range []struct {
		pos  time.Duration
		want float64
	}{
		{0, 0},
		{500 * time.Millisecond, 2},  // half of "abcd"
		{1000 * time.Millisecond, 5}, // "abcd " done (space lights with the word)
		{1500 * time.Millisecond, 7}, // half of "efgh"
		{2500 * time.Millisecond, 9}, // all sung
	} {
		m.pos = tc.pos
		if got := m.litChars(line, 3*time.Second); got != tc.want {
			t.Errorf("litChars at %v = %v, want %v", tc.pos, got, tc.want)
		}
	}

	// The fill advances continuously: a few ms later the edge has moved.
	m.pos = 1500 * time.Millisecond
	a := m.renderActiveLine(line, 3*time.Second)
	m.pos = 1540 * time.Millisecond
	if b := m.renderActiveLine(line, 3*time.Second); a == b && stripANSI(a) == "abcd efgh" {
		// Only meaningful when RGB blending is available; the palette
		// fallback moves in whole steps.
		if _, _, ok := styles.LyricColors(); ok {
			t.Error("highlight did not move between frames 40ms apart")
		}
	}
	if got := stripANSI(a); got != "abcd efgh" {
		t.Errorf("active line text = %q", got)
	}
}

func TestLyricsModel_LineSyncedHasNoWordFill(t *testing.T) {
	m := NewLyrics()
	m.SetSize(60, 10)
	m.SetLyrics(&lyrics.Result{Synced: true, Lines: []lyrics.Line{{Start: 0, Text: "whole line"}}}, nil)
	m.SetPosition(500 * time.Millisecond)
	if strings.Contains(m.View(), "word sync") {
		t.Error("line-synced lyrics must not claim word sync")
	}
	if len(m.lines[0].Words) != 0 {
		t.Error("no word timing should be invented")
	}
}

func TestLyricsModel_LanguagePicker(t *testing.T) {
	m := NewLyrics()
	m.SetSize(60, 12)
	m.SetTranslateLang("ru")
	m.SetLyrics(&lyrics.Result{Lines: []lyrics.Line{{Text: "hola"}}}, nil)

	_ = m.Update(tea.KeyPressMsg{Code: 'T', Text: "T"})
	view := m.View()
	if !m.PickerOpen() || !strings.Contains(view, "Translate into") || !strings.Contains(view, "Русский") {
		t.Fatalf("language picker not shown:\n%s", view)
	}
	// Scrolling to the end keeps the selection visible.
	_ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if last := TranslateLangs[len(TranslateLangs)-1].Name; !strings.Contains(m.View(), last) {
		t.Errorf("last language %q not visible after G:\n%s", last, m.View())
	}
	cmd := m.Update(tea.KeyPressMsg{Code: '5', Text: "5"})
	sel, ok := cmd().(LyricsLangSelectedMsg)
	if !ok || sel.Code != "de" {
		t.Fatalf("pick 5 = %+v", sel)
	}
	if m.PickerOpen() || !m.TranslationOn() || !m.NeedsTranslation() {
		t.Error("picking a language should close the picker and request a translation")
	}
}

func TestLyricsModel_InactiveLinesGreyAndFade(t *testing.T) {
	m := NewLyrics()
	now := time.Unix(1000, 0)
	m.now = func() time.Time { return now }
	m.SetSize(60, 12)
	m.SetLyrics(&lyrics.Result{Synced: true, Lines: []lyrics.Line{
		{Start: 0, Text: "past line"}, {Start: 2 * time.Second, Text: "now line"}, {Start: 4 * time.Second, Text: "next line"},
	}}, nil)
	m.SetPosition(500 * time.Millisecond)
	m.SetPosition(2500 * time.Millisecond)
	if m.prevIdx != 0 || m.currentIdx != 1 {
		t.Fatalf("prev=%d cur=%d", m.prevIdx, m.currentIdx)
	}
	_, dim, ok := styles.LyricColors()
	if !ok {
		t.Skip("no RGB lyric colours in this theme")
	}
	r, g, b, _ := dim.RGBA()
	greySeq := fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)

	// Long after the change, both the past and the upcoming line are grey.
	now = now.Add(time.Second)
	view := m.View()
	for l := range strings.SplitSeq(view, "\n") {
		if strings.Contains(l, "next line") && !strings.Contains(l, greySeq) {
			t.Errorf("upcoming line not grey: %q", l)
		}
		if strings.Contains(l, "past line") && !strings.Contains(l, greySeq) {
			t.Errorf("past line not grey: %q", l)
		}
	}
	// Right after a change the line just left is still mid-fade (not grey).
	now = now.Add(time.Second)
	m.SetPosition(4500 * time.Millisecond)
	now = now.Add(100 * time.Millisecond)
	for l := range strings.SplitSeq(m.View(), "\n") {
		if strings.Contains(l, "now line") && strings.Contains(l, greySeq) {
			t.Errorf("line just left should still be fading, got grey already: %q", l)
		}
	}
}

func TestLyricsModel_LongTranslationWrapsBelowRightAligned(t *testing.T) {
	m := NewLyrics()
	m.SetSize(30, 12)
	m.SetLyrics(&lyrics.Result{Synced: true, Lines: []lyrics.Line{
		{Start: 0, Text: "a fairly long lyric line"},
		{Start: time.Second, Text: "short"},
	}}, nil)
	m.SetPosition(0)
	_ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.SetTranslation([]string{"довольно длинная строка", "коротко"}, nil)

	rows := strings.Split(stripANSI(m.View()), "\n")
	for i, row := range rows {
		if strings.Contains(row, "a fairly long lyric line") {
			if strings.Contains(row, "довольно") {
				t.Fatalf("long translation should wrap below, got %q", row)
			}
			below := rows[i+1]
			if !strings.HasSuffix(below, "довольно длинная строка") || len([]rune(below)) != 30 {
				t.Errorf("wrapped translation not right-aligned below: %q", below)
			}
		}
		if strings.Contains(row, "short") && !strings.HasSuffix(row, "коротко") {
			t.Errorf("short line should keep its translation inline: %q", row)
		}
	}
	if m.lineRows(0) != 2 || m.lineRows(1) != 1 {
		t.Errorf("lineRows = %d,%d want 2,1", m.lineRows(0), m.lineRows(1))
	}
}
