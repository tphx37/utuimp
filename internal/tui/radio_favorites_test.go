package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/localstore"
	radioProvider "github.com/tphx37/utuimp/internal/provider/radio"
)

func TestRadioSaveSelectedAndRename(t *testing.T) {
	store, err := localstore.Open("")
	if err != nil {
		t.Fatal(err)
	}
	prov := radioProvider.New(store)
	track := provider.Track{ID: "https://example.org/live", Title: "Station"}
	prov.RememberTrack(track)
	m := newModel(newMockPlayer())
	m.provider = prov
	m.serviceKey = "radio"
	m.mode = modeSearch
	m.search.SetResults(&provider.SearchResult{Tracks: []provider.Track{track}}, false, nil)
	cmd := m.handleSearchKey("ctrl+f", tea.KeyPressMsg{})
	if cmd == nil {
		t.Fatal("save should start")
	}
	msg, ok := cmd().(loveSongMsg)
	if !ok || msg.err != nil {
		t.Fatalf("save: %+v", msg)
	}
	m.Update(msg)
	fav, _ := prov.GetLibraryTracks(context.Background())
	if len(fav) != 1 {
		t.Fatalf("favorites: %+v", fav)
	}
	m.mode = modeNormal
	m.library.m.ShowTrackList("Songs", fav)
	_, handled := m.libraryPlaylistKey("r")
	if !handled || m.prompt == nil {
		t.Fatal("rename prompt not opened")
	}
	cmd = m.prompt.submit("Custom station")
	result, ok := cmd().(stationRenamedMsg)
	if !ok || result.err != nil {
		t.Fatalf("rename: %+v", result)
	}
	m.prompt = nil
	m.Update(result)
	if got := m.library.m.SelectedTrack(); got == nil || got.Title != "Custom station" || got.ID != track.ID {
		t.Fatalf("renamed selection: %+v", got)
	}
	fav, _ = prov.GetLibraryTracks(context.Background())
	if fav[0].Title != "Custom station" {
		t.Fatalf("renamed favorite: %+v", fav)
	}
}

func TestRadioFeedSavesSelectedStation(t *testing.T) {
	store, _ := localstore.Open("")
	prov := radioProvider.New(store)
	selected := provider.Track{ID: "https://example.org/selected", Title: "Selected"}
	prov.RememberTrack(selected)
	m := newModel(newMockPlayer())
	m.provider = prov
	m.serviceKey = "radio"
	m.playerState.Track = &provider.Track{ID: "https://example.org/playing", Title: "Playing"}
	m.feedP.m.SetRecommendations([]provider.RecommendationGroup{{Title: "Recent", Items: []provider.RecommendationItem{{ID: selected.ID, Kind: "track", Title: selected.Title}}}})
	for i, p := range m.panels {
		if p == m.feedP {
			m.activePanel = i
			break
		}
	}
	cmd := m.handleNormalKey(tea.KeyPressMsg{}, "f")
	if cmd == nil {
		t.Fatal("save command missing")
	}
	msg, ok := cmd().(loveSongMsg)
	if !ok || msg.err != nil {
		t.Fatalf("save: %+v", msg)
	}
	fav := store.Favorites()
	if len(fav) != 1 || fav[0].ID != selected.ID {
		t.Fatalf("wrong favorite: %+v", fav)
	}
}
