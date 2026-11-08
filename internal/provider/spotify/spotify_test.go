package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const trackJSON = `{"id":"t1","uri":"spotify:track:t1","name":"Song","duration_ms":180000,"explicit":true,
 "disc_number":1,"track_number":3,"artists":[{"id":"a1","name":"Artist"},{"id":"a2","name":"Guest"}],
 "album":{"id":"al1","name":"Album","release_date":"2020-01-02","images":[{"url":"big","width":640},{"url":"mid","width":300},{"url":"small","width":64}]},
 "external_urls":{"spotify":"https://open.spotify.com/track/t1"}}`

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient("cid", Token{AccessToken: "at"}, nil)
	c.base = srv.URL
	return c
}

func TestSearchAndConversion(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at" {
			t.Error("missing bearer token")
		}
		_, _ = w.Write([]byte(`{"tracks":{"items":[` + trackJSON + `]},"albums":{"items":[]},"artists":{"items":[{"id":"a1","name":"Artist"}]},"playlists":{"items":[null,{"id":"p1","name":"Mix","tracks":{"total":5}}]}}`))
	})
	r, err := New(c).Search(context.Background(), "song")
	if err != nil {
		t.Fatal(err)
	}
	tr := r.Tracks[0]
	if tr.ID != "t1" || tr.Artist != "Artist, Guest" || tr.Album != "Album" || tr.ArtworkURL != "mid" ||
		tr.Duration != 3*time.Minute || tr.ContentRating != "explicit" || tr.TrackNumber != 3 {
		t.Errorf("track = %+v", tr)
	}
	if len(r.Playlists) != 1 || r.Playlists[0].TrackCount != 5 {
		t.Errorf("playlists = %+v (nulls must be skipped)", r.Playlists)
	}
}

func TestPaginationAndLiked(t *testing.T) {
	var srvURL string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") == "" {
			_, _ = w.Write([]byte(`{"items":[{"track":` + trackJSON + `}],"next":"` + srvURL + `/me/tracks?offset=50"}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"track":` + strings.Replace(trackJSON, `"t1"`, `"t2"`, 1) + `}],"next":null}`))
	})
	srvURL = c.base
	ts, err := New(c).GetLibraryTracks(context.Background())
	if err != nil || len(ts) != 2 || ts[1].ID != "t2" {
		t.Fatalf("liked = %+v, %v", ts, err)
	}
}

func TestRefreshOn401(t *testing.T) {
	var calls atomic.Int32
	accounts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("refresh_token") != "rt" {
			t.Errorf("refresh form = %v", r.PostForm)
		}
		_, _ = w.Write([]byte(`{"access_token":"new","expires_in":3600}`))
	}))
	defer accounts.Close()
	accountsURL = accounts.URL
	defer func() { accountsURL = "https://accounts.spotify.com" }()

	var saved Token
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer new" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"display_name":"me"}`))
	}))
	defer srv.Close()
	c := NewClient("cid", Token{AccessToken: "old", RefreshToken: "rt"}, func(t Token) { saved = t })
	c.base = srv.URL
	var me struct {
		DisplayName string `json:"display_name"`
	}
	if err := c.Do(context.Background(), http.MethodGet, "/me", nil, nil, &me); err != nil || me.DisplayName != "me" {
		t.Fatalf("me=%+v err=%v", me, err)
	}
	if saved.AccessToken != "new" || saved.RefreshToken != "rt" {
		t.Errorf("refreshed token not saved: %+v", saved)
	}
}

func TestPremiumAndDeviceErrors(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"status":403,"message":"Player command failed: Premium required","reason":"PREMIUM_REQUIRED"}}`))
	})
	if err := c.Do(context.Background(), http.MethodPut, "/me/player/play", nil, nil, nil); err != ErrPremiumRequired {
		t.Errorf("err = %v", err)
	}
}

func TestLoginPKCE(t *testing.T) {
	accounts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("code") != "the-code" || r.PostForm.Get("code_verifier") == "" || r.PostForm.Get("client_id") != "cid" {
			t.Errorf("token form = %v", r.PostForm)
		}
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_in":3600}`))
	}))
	defer accounts.Close()
	accountsURL = accounts.URL
	redirectURI = "http://127.0.0.1:18898/callback"
	defer func() { accountsURL = "https://accounts.spotify.com"; redirectURI = RedirectURI }()

	// The "browser": follow the consent URL's redirect back with a code.
	open := func(consent string) error {
		u, _ := url.Parse(consent)
		q := u.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || !strings.Contains(q.Get("scope"), "user-modify-playback-state") {
			t.Errorf("consent URL = %s", consent)
		}
		go func() {
			resp, err := http.Get(q.Get("redirect_uri") + "?code=the-code&state=" + url.QueryEscape(q.Get("state")))
			if err == nil {
				_, _ = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tok, err := Login(ctx, "cid", open)
	if err != nil || tok.AccessToken != "at" || tok.RefreshToken != "rt" {
		t.Fatalf("tok=%+v err=%v", tok, err)
	}
	_ = json.Marshal
}

func TestRateLimitIsWaitedOut(t *testing.T) {
	var n atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"display_name":"me"}`))
	})
	start := time.Now()
	if err := c.Do(context.Background(), http.MethodGet, "/me", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Error("should have waited for Retry-After")
	}

	c2 := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	var rl ErrRateLimited
	err := c2.Do(context.Background(), http.MethodGet, "/me", nil, nil, nil)
	if !errors.As(err, &rl) || rl.Wait < time.Hour-time.Minute {
		t.Fatalf("long limit should be reported, got %v", err)
	}
	if strings.Count(err.Error(), "spotify:") != 1 {
		t.Errorf("message = %q", err)
	}
}

func TestClientLimitedOn429WithoutRetryAfter(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	if err := c.Do(context.Background(), http.MethodGet, "/me", nil, nil, nil); !errors.Is(err, ErrClientLimited) {
		t.Errorf("err = %v, want ErrClientLimited", err)
	}
}

func TestOwnClientUsesPublicScopes(t *testing.T) {
	internal := map[string]bool{"playlist-modify": true, "playlist-read": true, "user-modify": true,
		"user-modify-private": true, "user-personalized": true, "user-read-play-history": true, "app-remote-control": true}
	for _, s := range scopesFor("my-own-client") {
		if internal[s] {
			t.Errorf("own client asks for internal scope %q", s)
		}
	}
	if len(scopesFor(DefaultClientID)) != len(scopes) {
		t.Error("shared client should keep librespot's scopes")
	}
}
