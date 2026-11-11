package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
)

// savedProvider keeps albums and artists of its own, like Spotify.
type savedProvider struct {
	editProvider
	albumTracks  map[string][]provider.Track
	artistAlbums map[string][]provider.Album
	unsaved      []string
	followed     []string
	saved        []string
}

func newSavedProvider() *savedProvider {
	return &savedProvider{
		editProvider: *newEditProvider(),
		albumTracks: map[string][]provider.Track{
			"al1": {{ID: "x1", Title: "One"}, {ID: "x2", Title: "Two"}, {ID: "x3", Title: "Three"}},
			"al9": {{ID: "y1", Title: "Deep Cut"}},
		},
		artistAlbums: map[string][]provider.Album{"ar1": {{ID: "al9", Title: "Early Work"}}},
	}
}

func (p *savedProvider) GetSavedAlbums(context.Context) ([]provider.Album, error) {
	return []provider.Album{{ID: "al1", Title: "Saved Record", Artist: "Band"}, {ID: "al2", Title: "Other"}}, nil
}
func (p *savedProvider) GetSavedArtists(context.Context) ([]provider.Artist, error) {
	return []provider.Artist{{ID: "ar1", Name: "Followed Band"}}, nil
}
func (p *savedProvider) GetAlbumTracks(_ context.Context, id string) ([]provider.Track, error) {
	return p.albumTracks[id], nil
}
func (p *savedProvider) GetArtistAlbums(_ context.Context, id string) ([]provider.Album, error) {
	return p.artistAlbums[id], nil
}
func (p *savedProvider) GetLibraryTracks(context.Context) ([]provider.Track, error) {
	// A liked song from an album that isn't saved: it must not show up as
	// an album.
	return []provider.Track{{ID: "z", Title: "Liked", Album: "Not Saved", Artist: "Somebody"}}, nil
}
func (p *savedProvider) SaveAlbum(_ context.Context, a provider.Album, save bool) error {
	if save {
		p.saved = append(p.saved, a.ID)
	} else {
		p.unsaved = append(p.unsaved, a.ID)
	}
	return nil
}
func (p *savedProvider) FollowArtist(_ context.Context, a provider.Artist, follow bool) error {
	if follow {
		p.followed = append(p.followed, a.ID)
	}
	return nil
}
func (p *savedProvider) GetSongDetails(context.Context, provider.Track) (provider.SongDetails, error) {
	return provider.SongDetails{Album: &provider.Album{ID: "al5", Title: "Their Album"},
		Artists: []provider.Artist{{ID: "ar5", Name: "Them"}}}, nil
}

func savedModel(t *testing.T) (*Model, *savedProvider) {
	t.Helper()
	prov := newSavedProvider()
	cfg := testCfg()
	cfg.LibraryDelete, cfg.LibraryEdit = true, true
	m := New(cfg, prov, newMockPlayer(), Options{})
	m.noAnim = true
	m.historyPath, m.history = "", nil
	m.width, m.height = 120, 40
	m.introStep = introDone
	m.Update(windowSize(120, 40))
	return m, prov
}

func openSection(m *Model, downs int) {
	key(m, "L")
	for range downs {
		key(m, "down")
	}
	key(m, "enter")
}

func TestLibraryShowsSavedAlbumsWhole(t *testing.T) {
	m, prov := savedModel(t)
	openSection(m, 1) // Albums
	view := m.library.m.View()
	if !strings.Contains(view, "Saved Record") || strings.Contains(view, "Not Saved") {
		t.Fatalf("Albums should list the saved albums, not the liked songs' albums:\n%s", view)
	}
	key(m, "enter")
	if got := len(m.library.m.SelectedTrack().ID); got == 0 {
		t.Fatal("the album should open")
	}
	key(m, "enter")
	if strings.Join(m.queueIDs, ",") != "x1,x2,x3" {
		t.Errorf("the whole album (3 songs) should play, queue = %v", m.queueIDs)
	}

	// d d takes the album out of the library.
	m.queueIDs = nil
	key(m, "L")
	openSection(m, 1)
	key(m, "d")
	key(m, "d")
	if len(prov.unsaved) != 1 || prov.unsaved[0] != "al1" {
		t.Fatalf("unsaved = %v", prov.unsaved)
	}
	if strings.Contains(m.library.m.View(), "Saved Record") {
		t.Error("the removed album should leave the list")
	}
}

func TestLibraryFollowedArtistOpensAlbums(t *testing.T) {
	m, _ := savedModel(t)
	openSection(m, 2) // Artists
	if !strings.Contains(m.library.m.View(), "Followed Band") {
		t.Fatalf("Artists should list the followed artists:\n%s", m.library.m.View())
	}
	key(m, "enter")
	if !strings.Contains(m.library.m.View(), "Early Work") {
		t.Fatalf("the artist should open their albums:\n%s", m.library.m.View())
	}
	key(m, "enter")
	if tr := m.library.m.SelectedTrack(); tr == nil || tr.Title != "Deep Cut" {
		t.Fatalf("the album should open with its songs, got %v", tr)
	}
	key(m, "esc")
	key(m, "esc")
	if !strings.Contains(m.library.m.View(), "Followed Band") {
		t.Errorf("esc should come back to the artists:\n%s", m.library.m.View())
	}
}

func TestSongMenuSavesAlbumAndFollowsArtist(t *testing.T) {
	m, prov := savedModel(t)
	cur := provider.Track{ID: "s", Title: "Song"}
	m.playerState = player.State{Track: &cur, Playing: true}
	for _, want := range []songMenuKind{songMenuSaveAlbum, songMenuFollowArtist} {
		key(m, "i")
		n := -1
		for i, it := range m.songMenu.items {
			if it.kind == want {
				n = i
			}
		}
		if n < 0 {
			t.Fatalf("song menu lacks item %d", want)
		}
		key(m, string(rune('1'+n)))
	}
	if len(prov.saved) != 1 || prov.saved[0] != "al5" || len(prov.followed) != 1 || prov.followed[0] != "ar5" {
		t.Fatalf("saved %v followed %v", prov.saved, prov.followed)
	}
}

func windowSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }
