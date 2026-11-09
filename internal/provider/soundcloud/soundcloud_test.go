package soundcloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const trackJSON = `{"urn":"soundcloud:tracks:1","title":"Song","duration":180000,"access":"playable",
 "artwork_url":"https://i1.sndcdn.com/artworks-x-large.jpg","permalink_url":"https://soundcloud.com/a/song",
 "user":{"urn":"soundcloud:users:9","username":"Artist"}}`

func testClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient(App{ClientID: "id", ClientSecret: "secret"}, Token{AccessToken: "at"}, nil)
	c.base = srv.URL
	return c, srv
}

func TestSearch(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth at" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/tracks":
			_, _ = w.Write([]byte(`{"collection":[` + trackJSON + `,{"urn":"soundcloud:tracks:2","title":"Snippet","access":"preview"}]}`))
		case "/playlists":
			_, _ = w.Write([]byte(`{"collection":[{"urn":"soundcloud:playlists:5","title":"LP","playlist_type":"album","user":{"username":"Artist"}},{"urn":"soundcloud:playlists:6","title":"Mix"}]}`))
		case "/users":
			_, _ = w.Write([]byte(`{"collection":[{"urn":"soundcloud:users:9","username":"Artist"}]}`))
		}
	})
	r, err := New(c, "").Search(context.Background(), "song")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tracks) != 1 {
		t.Fatalf("tracks = %+v (previews must be dropped)", r.Tracks)
	}
	tr := r.Tracks[0]
	if tr.ID != "soundcloud:tracks:1" || tr.Artist != "Artist" || tr.Duration != 3*time.Minute ||
		!strings.Contains(tr.ArtworkURL, "-t500x500.") {
		t.Errorf("track = %+v", tr)
	}
	if len(r.Albums) != 1 || len(r.Playlists) != 1 || len(r.Artists) != 1 {
		t.Errorf("albums %v playlists %v artists %v", r.Albums, r.Playlists, r.Artists)
	}
}

func TestRefreshOn401(t *testing.T) {
	secure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("client_secret") != "secret" {
			t.Errorf("refresh form = %v", r.PostForm)
		}
		_, _ = w.Write([]byte(`{"access_token":"new","refresh_token":"rt2","expires_in":3600}`))
	}))
	defer secure.Close()
	secureURL = secure.URL
	defer func() { secureURL = "https://secure.soundcloud.com" }()

	var saved Token
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth new" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"username":"me"}`))
	}))
	defer srv.Close()
	c := NewClient(App{"id", "secret"}, Token{AccessToken: "old", RefreshToken: "rt"}, func(t Token) { saved = t })
	c.base = srv.URL
	var me struct{ Username string }
	if err := c.Do(context.Background(), http.MethodGet, "/me", nil, nil, &me); err != nil || me.Username != "me" {
		t.Fatalf("me = %+v, %v", me, err)
	}
	if saved.RefreshToken != "rt2" {
		t.Errorf("the new single-use refresh token must be saved: %+v", saved)
	}
}

func TestHLSJoinsMP3Segments(t *testing.T) {
	var srvURL string
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tracks/soundcloud:tracks:1/streams":
			_, _ = w.Write([]byte(`{"hls_mp3_128_url":"` + srvURL + `/stream/pl"}`))
		case "/stream/pl":
			// Redirect to the "CDN" playlist, as the API does.
			http.Redirect(w, r, "/cdn/media.m3u8", http.StatusFound)
		case "/cdn/media.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:10,\nseg0.mp3\n#EXTINF:10,\nseg1.mp3\n#EXT-X-ENDLIST\n"))
		case "/cdn/seg0.mp3":
			_, _ = w.Write([]byte("AAA"))
		case "/cdn/seg1.mp3":
			_, _ = w.Write([]byte("BBB"))
		default:
			http.NotFound(w, r)
		}
	})
	srvURL = srv.URL
	rc, codec, _, err := New(c, "").OpenStream(context.Background(), "soundcloud:tracks:1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	b, _ := io.ReadAll(rc)
	if codec != "mp3" || string(b) != "AAABBB" {
		t.Errorf("codec %s, data %q", codec, b)
	}
}

func TestAddToPlaylistKeepsTracks(t *testing.T) {
	var body map[string]map[string][]map[string]string
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/playlists/soundcloud:playlists:5/tracks":
			_, _ = w.Write([]byte(`{"collection":[` + trackJSON + `]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/playlists/soundcloud:playlists:5":
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	if err := New(c, "").AddToPlaylist(context.Background(), "soundcloud:playlists:5", "soundcloud:tracks:7"); err != nil {
		t.Fatal(err)
	}
	ts := body["playlist"]["tracks"]
	if len(ts) != 2 || ts[0]["urn"] != "soundcloud:tracks:1" || ts[1]["urn"] != "soundcloud:tracks:7" {
		t.Errorf("PUT tracks = %v", ts)
	}
}

func TestLikesAndRating(t *testing.T) {
	var liked bool
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/me/likes/tracks":
			_, _ = w.Write([]byte(`{"collection":[` + trackJSON + `]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/likes/tracks/soundcloud:tracks:7":
			liked = true
			w.WriteHeader(http.StatusCreated)
		}
	})
	p := New(c, "")
	ctx := context.Background()
	if ok, err := p.GetSongRating(ctx, "soundcloud:tracks:1"); !ok || err != nil {
		t.Errorf("liked track: %v %v", ok, err)
	}
	if err := p.LoveSong(ctx, "soundcloud:tracks:7", true); err != nil || !liked {
		t.Fatalf("like: %v", err)
	}
	if ok, _ := p.GetSongRating(ctx, "soundcloud:tracks:7"); !ok {
		t.Error("a new like should count at once")
	}
}
