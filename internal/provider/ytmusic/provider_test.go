package ytmusic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/localstore"
	"github.com/tphx37/utuimp/internal/provider/ytdlp"
)

// fakeRun answers yt-dlp calls by the URL (the last argument).
func fakeRun(t *testing.T, pages map[string]string) ytdlp.Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		u := args[len(args)-1]
		for k, v := range pages {
			if strings.Contains(u, k) {
				return []byte(v), nil
			}
		}
		t.Errorf("unexpected yt-dlp call %v", args)
		return []byte(`{}`), nil
	}
}

func testProvider(t *testing.T, pages map[string]string) *Provider {
	st, err := localstore.Open(filepath.Join(t.TempDir(), "lib.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := New("yt-dlp", "ffmpeg", "", st)
	p.run = fakeRun(t, pages)
	return p
}

func TestSearchResolvesNames(t *testing.T) {
	p := testProvider(t, map[string]string{
		"#songs":   `{"entries":[{"_type":"url","id":"v1","title":"Get Lucky"}]}`,
		"#albums":  `{"entries":[{"_type":"url","id":"MPREb_a"}]}`,
		"#artists": `{"entries":[{"_type":"url","id":"UCx"}]}`,
		"browse/MPREb_a": `{"id":"OLAK_a","title":"Album - Random Access Memories",
			"entries":[{"id":"v1","title":"Give Life Back","channel":"Daft Punk"}]}`,
		"browse/UCx": `{"id":"UCx","title":"Daft Punk - Topic","channel":"Daft Punk - Topic"}`,
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct {
			Params string `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if strings.Contains(body.Params, "QIYA") {
			_, _ = w.Write([]byte(`{"contents":[{"musicResponsiveListItemRenderer":{"navigationEndpoint":{"browseEndpoint":{"browseId":"MPREb_a"}},"flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Random Access Memories"}]}}},{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Daft Punk","navigationEndpoint":{"browseEndpoint":{"browseId":"UCx"}}},{"text":"2013"}]}}}]}}]}`))
			return
		}
		if strings.Contains(body.Params, "QIgA") {
			_, _ = w.Write([]byte(`{"contents":[{"musicResponsiveListItemRenderer":{"navigationEndpoint":{"browseEndpoint":{"browseId":"UCx"}},"flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Daft Punk"}]}}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"contents":[{"musicResponsiveListItemRenderer":{"flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Get Lucky"}]}}},{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Daft Punk","navigationEndpoint":{"browseEndpoint":{"browseId":"UCx"}}},{"text":" • "},{"text":"Random Access Memories","navigationEndpoint":{"browseEndpoint":{"browseId":"MPREb_a"}}},{"text":" • "},{"text":"4:08"}]}}}],"playlistItemData":{"videoId":"v1"}}}]}`))
	}))
	defer server.Close()
	p.searchEndpoint, p.searchClient = server.URL, server.Client()
	r, err := p.Search(context.Background(), "daft punk")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tracks) != 1 || r.Tracks[0].Title != "Get Lucky" || r.Tracks[0].Artist != "Daft Punk" || r.Tracks[0].Album != "Random Access Memories" || r.Tracks[0].Duration != 248*time.Second || !strings.Contains(r.Tracks[0].ArtworkURL, "v1") {
		t.Errorf("tracks = %+v", r.Tracks)
	}
	if len(r.Albums) != 1 || r.Albums[0].ID != "MPREb_a" || r.Albums[0].Title != "Random Access Memories" || r.Albums[0].Artist != "Daft Punk" {
		t.Errorf("albums = %+v", r.Albums)
	}
	if len(r.Artists) != 1 || r.Artists[0].Name != "Daft Punk" {
		t.Errorf("artists = %+v", r.Artists)
	}
}

func TestStationDropsSeedAndFavorites(t *testing.T) {
	p := testProvider(t, map[string]string{
		"list=RDAMVMv1": `{"entries":[{"id":"v1","title":"Seed"},{"id":"v2","title":"Next","channel":"Band","duration":200}]}`,
	})
	ts, err := p.GetStationTracks(context.Background(), "v1")
	if err != nil || len(ts) != 1 || ts[0].ID != "v2" || ts[0].Artist != "Band" || ts[0].Duration != 200*time.Second {
		t.Fatalf("station = %+v, %v", ts, err)
	}
	ctx := context.Background()
	if err := p.LoveSong(ctx, "v2", true); err != nil {
		t.Fatal(err)
	}
	if ok, _ := p.GetSongRating(ctx, "v2"); !ok {
		t.Error("v2 should be a favourite")
	}
	fav, _ := p.GetPlaylistTracks(ctx, provider.FavoritesPlaylistID)
	if len(fav) != 1 || fav[0].Title != "Next" {
		t.Errorf("favourites = %+v", fav)
	}
	groups, _ := p.GetRecommendations(ctx)
	if len(groups[0].Items) != 1 || groups[0].Items[0].ID != "RDAMVMv2" {
		t.Errorf("feed = %+v", groups)
	}
	pl, err := p.CreatePlaylist(ctx, "Mine", []string{"v2"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := p.GetPlaylistTracks(ctx, pl.ID); len(got) != 1 {
		t.Errorf("playlist = %+v", got)
	}
}

func TestAccountLibraryFromCookies(t *testing.T) {
	p := testProvider(t, map[string]string{
		"feed/playlists": `{"entries":[{"id":"PLone","title":"Mine"},{"id":"LL","title":"Likes"}]}`,
		"list=LM":        `{"entries":[{"id":"v1","title":"Liked","channel":"Artist"}]}`,
		"list=PLone":     `{"entries":[{"id":"v2","title":"Playlist song"}]}`,
	})
	p.account = true
	ctx := context.Background()
	playlists, err := p.GetLibraryPlaylists(ctx)
	if err != nil || len(playlists) != 2 || playlists[1].ID != "PLone" || !playlists[1].ReadOnly {
		t.Fatalf("playlists: %+v, %v", playlists, err)
	}
	likes, err := p.GetLibraryTracks(ctx)
	if err != nil || len(likes) != 1 || likes[0].ID != "v1" {
		t.Fatalf("likes: %+v, %v", likes, err)
	}
	tracks, err := p.GetPlaylistTracks(ctx, "PLone")
	if err != nil || len(tracks) != 1 || tracks[0].ID != "v2" {
		t.Fatalf("playlist: %+v, %v", tracks, err)
	}
}
