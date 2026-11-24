package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tphx37/utuimp/internal/gfx"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
)

func TestSongMenuBackdropCoversGraphics(t *testing.T) {
	m := coverModel(t, 100, 40)
	m.coverView = true
	m.lines.mode = gfx.File
	m.lines.cellW, m.lines.cellH = 9, 18
	m.songMenu = &songMenuState{track: *m.playerState.Track, items: []songMenuItem{{label: "Album"}}}
	seq := m.songMenuBackdrop()
	if seq == "" || !m.lines.menuShown || m.lines.menuImg.Z <= m.lines.strips[stripLabels].img.Z {
		t.Fatal("menu needs an opaque layer above the player graphics")
	}
	if m.lines.strips[stripFrame].img.Z <= m.lines.menuImg.Z {
		t.Fatal("the popup frame must be above its backdrop")
	}
	m.linesPlace(m.renderMain())
	mark := m.lines.strips[stripSel]
	if !mark.seen || mark.img.Z <= m.lines.menuImg.Z {
		t.Fatalf("song menu selection must appear above backdrop: seen=%v z=%d", mark.seen, mark.img.Z)
	}
	w, h := m.lines.menuKey[2]*m.lines.cellW, m.lines.menuKey[3]*m.lines.cellH
	if !strings.Contains(seq, fmt.Sprintf("s=%d,v=%d", w, h)) {
		t.Fatalf("backdrop must have the menu's full pixel dimensions: %q", seq)
	}
	m.songMenu = nil
	if m.songMenuBackdrop() == "" || m.lines.menuShown {
		t.Fatal("closing the menu should remove its backdrop")
	}
}

type detailProvider struct {
	*mockProvider
}

func (detailProvider) GetSongDetails(context.Context, provider.Track) (provider.SongDetails, error) {
	return provider.SongDetails{
		Album:   &provider.Album{ID: "al1", Title: "In the Zone"},
		Artists: []provider.Artist{{ID: "a1", Name: "Britney Spears"}, {ID: "a2", Name: "Madonna"}},
		URL:     "https://music.apple.com/x/album/y/1?i=2",
	}, nil
}

func TestSongMenu(t *testing.T) {
	m := newModel(newMockPlayer())
	m.provider = detailProvider{&mockProvider{}}
	m.width, m.height = 100, 40
	m.introStep = introDone
	m.playerState = player.State{Playing: true, Track: &provider.Track{ID: "s1", Title: "Me Against the Music", Artist: "Britney Spears"}}

	cmd := m.handleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if m.songMenu == nil || cmd == nil {
		t.Fatal("i should open the song menu and look the song up")
	}
	m.Update(cmd())
	if n := len(m.songMenu.items); n != 5 {
		t.Fatalf("items = %d, want album + 2 artists + copy link + add to playlist", n)
	}
	strip := regexp.MustCompile(`\x1b\[[0-9;:]*m`)
	screen := strip.ReplaceAllString(m.View().Content, "")
	for _, want := range []string{"Album · In the Zone", "Artist · Britney Spears", "Artist · Madonna", "Copy link", "Add to playlist", "Now Playing"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen missing %q (menu should sit over the UI):\n%s", want, screen)
		}
	}

	if !strings.Contains(screen, "│ 1 ") {
		t.Errorf("selected song menu item has no cursor:\n%s", screen)
	}

	// Each artist opens separately.
	m.handleKey(tea.KeyPressMsg{Code: '3', Text: "3"})
	if m.songMenu != nil || m.mode != modeSearch || !m.search.InArtist() {
		t.Errorf("artist item should open the artist page (mode=%v)", m.mode)
	}

	// Copy link.
	m.mode = modeNormal
	m.Update(m.handleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})())
	if cmd := m.handleKey(tea.KeyPressMsg{Code: '4', Text: "4"}); cmd == nil || !strings.Contains(m.errMsg, "copied") {
		t.Errorf("copy link: cmd=%v status=%q", cmd != nil, m.errMsg)
	}

	// Album opens in the library track pane.
	m.Update(m.handleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})())
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(songMenuAlbumMsg{title: "In the Zone", tracks: []provider.Track{{ID: "t1", Title: "Toxic"}}})
	if m.activePanel < 0 || m.panels[m.activePanel] != m.library || m.library.m.SelectedTrack() == nil {
		t.Error("album should open in the library")
	}

	// esc closes.
	m.handleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.songMenu != nil {
		t.Error("esc should close the menu")
	}
}
