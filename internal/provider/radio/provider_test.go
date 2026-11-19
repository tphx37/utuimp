package radio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/localstore"
)

func TestSearchSaveAndReload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/stations/search" || r.URL.Query().Get("name") != "jazz" || r.Header.Get("User-Agent") == "" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		_, _ = fmt.Fprint(w, `[{"name":"Jazz FM","url":"http://original.example/live","url_resolved":"https://stream.example/live.mp3","country":"UK"}]`)
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "radio.json")
	st, err := localstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p := New(st)
	p.endpoint = srv.URL
	p.client = srv.Client()
	r, err := p.Search(context.Background(), "jazz")
	if err != nil || len(r.Tracks) != 1 {
		t.Fatalf("search: %+v %v", r, err)
	}
	track := r.Tracks[0]
	if track.ID != "https://stream.example/live.mp3" || track.Title != "Jazz FM" || track.Artist != "" || track.Album != "" {
		t.Fatalf("station: %+v", track)
	}
	if err = p.LoveSong(context.Background(), track.ID, true); err != nil {
		t.Fatal(err)
	}
	pl, err := p.CreatePlaylist(context.Background(), "My stations", []string{track.ID})
	if err != nil {
		t.Fatal(err)
	}
	st, err = localstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p = New(st)
	favorites, _ := p.GetPlaylistTracks(context.Background(), provider.FavoritesPlaylistID)
	tracks, err := p.GetPlaylistTracks(context.Background(), pl.ID)
	if err != nil || len(favorites) != 1 || len(tracks) != 1 || tracks[0].ID != track.ID {
		t.Fatalf("saved: %+v %+v %v", favorites, tracks, err)
	}
	if _, ok := p.Lookup(track.ID); !ok {
		t.Fatal("saved station cannot be played after restart")
	}
}
func TestDirectStream(t *testing.T) {
	st, _ := localstore.Open("")
	p := New(st)
	r, e := p.Search(context.Background(), "https://example.org/live.mp3")
	if e != nil || len(r.Tracks) != 1 || r.Tracks[0].ID != "https://example.org/live.mp3" {
		t.Fatalf("direct: %+v %v", r, e)
	}
}

func TestLinkMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/live" {
			w.Header().Set("icy-name", "Station FM")
			w.Header().Set("Content-Type", "audio/mpeg")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<title>Fallback</title><meta property="og:title" content="Night &amp; Day"><meta name="author" content="The Artist"><meta name="album" content="The Album">`)
	}))
	defer srv.Close()
	store, _ := localstore.Open("")
	p := New(store)
	p.client = srv.Client()
	for _, tc := range []struct{ path, title, artist, album string }{{"/live", "Station FM", "Internet radio", ""}, {"/page", "Night & Day", "The Artist", "The Album"}} {
		got, err := p.Search(context.Background(), srv.URL+tc.path)
		if err != nil || len(got.Tracks) != 1 {
			t.Fatalf("search: %+v %v", got, err)
		}
		t1 := got.Tracks[0]
		if t1.Title != tc.title || t1.Artist != tc.artist || t1.Album != tc.album || t1.ID != srv.URL+tc.path {
			t.Errorf("metadata: %+v", t1)
		}
	}
}

func TestRenameSavedStationSurvivesRestartAndSearch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "radio.json")
	store, err := localstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p := New(store)
	station := provider.Track{ID: "https://example.org/live.mp3", Title: "Original"}
	p.RememberTrack(station)
	if err = p.LoveSong(context.Background(), station.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = p.CreatePlaylist(context.Background(), "Playlist", []string{station.ID}); err != nil {
		t.Fatal(err)
	}
	if err = p.RenameStation(context.Background(), station.ID, "My radio"); err != nil {
		t.Fatal(err)
	}
	store, err = localstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p = New(store)
	track, ok := p.Lookup(station.ID)
	if !ok || track.Title != "My radio" {
		t.Fatalf("restored: %+v", track)
	}
	p.RememberTrack(station)
	track, _ = p.Lookup(station.ID)
	if track.Title != "My radio" {
		t.Fatalf("history overwrote name: %+v", track)
	}
	fav, _ := p.GetLibraryTracks(context.Background())
	if len(fav) != 1 || fav[0].Title != "My radio" || fav[0].ID != station.ID {
		t.Fatalf("favorites: %+v", fav)
	}
	if err = p.RenameStation(context.Background(), "https://example.org/unknown", "Name"); err == nil {
		t.Fatal("renaming an unsaved station must fail")
	}
}
