package yandex

import (
	"context"
	"crypto/md5" //nolint:gosec // test mirrors the protocol
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testProvider(t *testing.T, routes map[string]string) *Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth tok" || r.Header.Get("X-Yandex-Music-Client") == "" {
			t.Errorf("%s: missing auth/client headers", r.URL.Path)
		}
		body, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewClient("tok")
	c.base = srv.URL
	return New(c, "42")
}

const trackJSON = `{"id":"558818","title":"J.B.G.","version":"Live","durationMs":236800,"explicit":true,
 "coverUri":"avatars.yandex.net/get-music-content/1/x/%%",
 "artists":[{"id":1,"name":"Alizée"},{"id":2,"name":"Guest"}],
 "albums":[{"id":59874,"title":"Gourmandises","year":2000,"genre":"pop","trackPosition":{"volume":1,"index":4}}]}`

func TestTrackConversion(t *testing.T) {
	p := testProvider(t, map[string]string{
		"GET /search": `{"result":{"tracks":{"results":[` + trackJSON + `]},"artists":{"results":[{"id":1,"name":"Alizée"}]}}}`,
	})
	r, err := p.Search(context.Background(), "jbg")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tracks) != 1 || len(r.Artists) != 1 {
		t.Fatalf("got %+v", r)
	}
	tr := r.Tracks[0]
	if tr.ID != "558818:59874" || tr.Title != "J.B.G. (Live)" || tr.Artist != "Alizée, Guest" || tr.Album != "Gourmandises" {
		t.Errorf("track = %+v", tr)
	}
	if tr.Duration != 236800*time.Millisecond || tr.ContentRating != "explicit" || tr.TrackNumber != 4 || tr.ReleaseDate != "2000" {
		t.Errorf("track details = %+v", tr)
	}
	if tr.ArtworkURL != "https://avatars.yandex.net/get-music-content/1/x/400x400" {
		t.Errorf("artwork = %q", tr.ArtworkURL)
	}
	// The player can look it up without another request.
	if got, err := p.Lookup(context.Background(), "558818"); err != nil || got.Title != tr.Title {
		t.Errorf("lookup = %+v, %v", got, err)
	}
}

func TestLikedTracksAndRating(t *testing.T) {
	p := testProvider(t, map[string]string{
		"GET /users/42/likes/tracks": `{"result":{"library":{"tracks":[{"id":"558818","albumId":"59874","timestamp":"x"}]}}}`,
		"POST /tracks":               `{"result":[` + trackJSON + `]}`,
	})
	ts, err := p.GetLibraryTracks(context.Background())
	if err != nil || len(ts) != 1 {
		t.Fatalf("liked = %+v, %v", ts, err)
	}
	if liked, _ := p.GetSongRating(context.Background(), "558818:59874"); !liked {
		t.Error("liked track should rate as loved")
	}
}

func TestPlaylistWithTrackStubs(t *testing.T) {
	p := testProvider(t, map[string]string{
		"GET /users/7/playlists/3": `{"result":{"uid":7,"kind":3,"title":"Mix","tracks":[{"id":"558818","albumId":59874}]}}`,
		"POST /tracks":             `{"result":[` + trackJSON + `]}`,
	})
	ts, err := p.GetPlaylistTracks(context.Background(), "7:3")
	if err != nil || len(ts) != 1 || ts[0].Title != "J.B.G. (Live)" {
		t.Fatalf("tracks = %+v, %v", ts, err)
	}
}

func TestErrorsAndUnauthorized(t *testing.T) {
	p := testProvider(t, map[string]string{
		"GET /account/status": `{"result":{"account":{}}}`,
		"GET /search":         `{"error":{"name":"session-expired","message":"session-expired"}}`,
	})
	if _, err := p.c.AccountUID(context.Background()); err != ErrUnauthorized {
		t.Errorf("anonymous account should be unauthorized, got %v", err)
	}
	if _, err := p.Search(context.Background(), "x"); err != ErrUnauthorized {
		t.Errorf("session-expired should map to ErrUnauthorized, got %v", err)
	}
}

func TestPickDownloadAndSigning(t *testing.T) {
	infos := []downloadInfo{
		{Codec: "mp3", BitrateInKbps: 320},
		{Codec: "aac", BitrateInKbps: 64},
		{Codec: "aac", BitrateInKbps: 192, Preview: true},
		{Codec: "flac", BitrateInKbps: 1000},
	}
	if got := pickDownload(infos); got.Codec != "aac" || got.BitrateInKbps != 64 {
		t.Errorf("picked %+v, want full aac", got)
	}
	if got := pickDownload([]downloadInfo{{Codec: "mp3", Preview: true}}); got == nil {
		t.Error("a preview is better than nothing")
	}
	sum := md5.Sum([]byte(downloadSalt + "get-mp3/abc" + "s1")) //nolint:gosec // protocol
	want := "https://h.example/get-mp3/" + hex.EncodeToString(sum[:]) + "/ts1/get-mp3/abc"
	if got := signedDownloadURL("h.example", "/get-mp3/abc", "ts1", "s1"); got != want {
		t.Errorf("signed = %q, want %q", got, want)
	}
}

func TestAuthDeviceFlow(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/device/code":
			if r.PostForm.Get("client_id") == "" || r.PostForm.Get("device_id") == "" {
				t.Error("device code request missing fields")
			}
			_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"ABCD","verification_url":"https://ya.ru/device","interval":1,"expires_in":30}`))
		case "/token":
			polls++
			if polls < 2 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_in":3600}`))
		}
	}))
	defer srv.Close()
	oauthURL = srv.URL
	defer func() { oauthURL = oauthBase }()
	dc, err := RequestDeviceCode(context.Background())
	if err != nil || dc.UserCode != "ABCD" {
		t.Fatalf("dc=%+v err=%v", dc, err)
	}
	tok, err := WaitForToken(context.Background(), dc)
	if err != nil || tok.AccessToken != "at" || tok.RefreshToken != "rt" || tok.Expiry.IsZero() {
		t.Fatalf("tok=%+v err=%v", tok, err)
	}
	if !strings.Contains(dc.VerificationURL, "ya.ru") {
		t.Error("verification URL")
	}
}

func TestStationSeedIsTheBareTrackID(t *testing.T) {
	var seeds []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rotor/session/new" {
			var body struct {
				Seeds []string `json:"seeds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			seeds = body.Seeds
			_, _ = w.Write([]byte(`{"result":{"radioSessionId":"s1","sequence":[{"track":` + trackJSON + `}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()
	c := NewClient("tok")
	c.base = srv.URL
	p := New(c, "42")
	// "id:albumId" as a seed silently gives the generic My Wave; the bare
	// track ID gives the wave by that song.
	if _, err := p.GetStationTracks(context.Background(), "558818:59874"); err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 1 || seeds[0] != "track:558818" {
		t.Errorf("seeds = %v, want [track:558818]", seeds)
	}
}
