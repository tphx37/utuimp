package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/views"
)

type previewProvider struct{ mockProvider }

func (p *previewProvider) GetCatalogPlaylistTracks(_ context.Context, _ string) ([]provider.Track, error) {
	return []provider.Track{
		{ID: "p1", Title: "Mix One", Artist: "A"},
		{ID: "p2", Title: "Mix Two", Artist: "B"},
	}, nil
}

// msgOf runs cmd and returns the first message of type T it (or a batch of
// it) produces.
func msgOf[T any](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	var zero T
	if cmd == nil {
		t.Fatalf("no command, want %T", zero)
	}
	switch msg := cmd().(type) {
	case T:
		return msg
	case tea.BatchMsg:
		for _, c := range msg {
			if c == nil {
				continue
			}
			if m, ok := c().(T); ok {
				return m
			}
		}
	}
	t.Fatalf("command produced no %T", zero)
	return zero
}

func feedModel(t *testing.T) *Model {
	t.Helper()
	m := coverModel(t, 120, 36)
	m.provider = &previewProvider{}
	m.toggleCoverView()
	m.feedP.m.SetRecommendations([]provider.RecommendationGroup{{
		Title: "For you",
		Items: []provider.RecommendationItem{{ID: "pl.1", Kind: "playlist", Title: "Daily Mix"}},
	}})
	for i, p := range m.panels {
		if p == m.feedP {
			m.activePanel = i
		}
	}
	return m
}

// A playlist picked in the feed opens as a preview of its songs instead of
// playing; esc goes back to the feed.
func TestFeedPlaylistOpensPreview(t *testing.T) {
	m := feedModel(t)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	msg := msgOf[collectionPreviewMsg](t, m.handleNormalKey(enter, "enter"))
	if len(m.queueTracks) != 2 || m.queueTracks[0].ID != "1" {
		t.Fatal("opening a feed playlist must not touch the queue")
	}
	m.Update(msg)
	if m.activePanel != m.libraryPanelIdx() {
		t.Fatalf("preview should open in the library, active panel %d", m.activePanel)
	}
	frame := ansi.Strip(m.View().Content)
	for _, want := range []string{"Daily Mix", "Mix One", "Mix Two"} {
		if !strings.Contains(frame, want) {
			t.Errorf("preview lacks %q:\n%s", want, frame)
		}
	}
	m.handleNormalKey(tea.KeyPressMsg{Code: tea.KeyEscape}, "esc")
	if m.activePanel < 0 || m.panels[m.activePanel] != m.feedP {
		t.Fatalf("esc should go back to the feed, active panel %d", m.activePanel)
	}
}

// Picking a song in the preview plays it and lands on the cover view's
// lyrics, not on the queue.
func TestPreviewTrackPlaysAndShowsLyrics(t *testing.T) {
	m := feedModel(t)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	m.Update(msgOf[collectionPreviewMsg](t, m.handleNormalKey(enter, "enter")))
	play := msgOf[views.PlayTracksMsg](t, m.handleNormalKey(enter, "enter"))
	m.Update(play)
	if m.activePanel != m.lyricsPanelIdx() || m.previewBack != nil {
		t.Fatalf("playing from a preview should show the lyrics, active panel %d", m.activePanel)
	}
	if len(m.queueTracks) != 2 || m.queueTracks[0].ID != "p1" {
		t.Errorf("the playlist should be queued, got %v", m.queueTracks)
	}

	// With the lyrics hidden it lands on the song beside the cover.
	m = feedModel(t)
	m.toggleCoverLyrics()
	m.activePanel = 3 // the feed
	m.Update(msgOf[collectionPreviewMsg](t, m.handleNormalKey(enter, "enter")))
	m.Update(msgOf[views.PlayTracksMsg](t, m.handleNormalKey(enter, "enter")))
	if m.activePanel != -1 || m.coverQueueOpen || m.coverPanelFramed() {
		t.Fatalf("plain cover view should show its home screen, active panel %d", m.activePanel)
	}
}

// My Wave plays straight away and shows the lyrics too.
func TestFeedWaveShowsLyrics(t *testing.T) {
	m := feedModel(t)
	m.feedP.m.SetRecommendations([]provider.RecommendationGroup{{
		Title: "Wave",
		Items: []provider.RecommendationItem{{ID: "wave:user:onyourwave", Kind: "playlist", Title: "My Wave"}},
	}})
	m.handleNormalKey(tea.KeyPressMsg{Code: tea.KeyEnter}, "enter")
	if !m.radio.enabled || m.activePanel != m.lyricsPanelIdx() {
		t.Fatalf("My Wave should start radio and show the lyrics, active panel %d", m.activePanel)
	}
}

// ctrl+c quits only when pressed twice in a row.
func TestCtrlCTwiceQuits(t *testing.T) {
	m := newModel(newMockPlayer())
	ctrlC := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	if cmd := m.handleKey(ctrlC); cmd != nil {
		t.Fatal("the first ctrl+c should only warn")
	}
	if !strings.Contains(m.errMsg, "ctrl+c") {
		t.Errorf("no hint after the first ctrl+c: %q", m.errMsg)
	}
	if _, ok := m.handleKey(ctrlC)().(tea.QuitMsg); !ok {
		t.Fatal("the second ctrl+c should quit")
	}

	// Another key in between starts over.
	m = newModel(newMockPlayer())
	m.handleKey(ctrlC)
	m.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if cmd := m.handleKey(ctrlC); cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("ctrl+c after another key should warn again, not quit")
		}
	}
}
