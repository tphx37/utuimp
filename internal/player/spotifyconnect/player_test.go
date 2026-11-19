package spotifyconnect

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/provider/spotify"
)

func TestLocalPlaybackRestrictionSuggestsLibrespotUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"status":403,"message":"Player command failed: Restriction violated","reason":"UNKNOWN"}}`))
	}))
	defer srv.Close()
	c := spotify.NewClient("cid", spotify.Token{AccessToken: "at"}, nil)
	c.SetBaseURL(srv.URL)
	p := New(spotify.New(c), nil)
	defer func() { _ = p.Close() }()
	p.SetDevice("local")
	err := p.SetQueue([]string{"t1"})
	if !errors.Is(err, spotify.ErrPlaybackRestricted) || !strings.Contains(err.Error(), "librespot may need an update") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetQueuePlaysOnTheComputerAndMirrorsState(t *testing.T) {
	var mu sync.Mutex
	var played []string
	var playDevice string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /me/player/devices":
			_, _ = w.Write([]byte(`{"devices":[{"id":"phone","name":"Phone","type":"Smartphone"},{"id":"pc","name":"PC","type":"Computer"}]}`))
		case "PUT /me/player/play":
			var body struct {
				URIs []string `json:"uris"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			played, playDevice = body.URIs, r.URL.Query().Get("device_id")
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case "GET /me/player":
			_, _ = w.Write([]byte(`{"is_playing":true,"progress_ms":5000,"repeat_state":"track","device":{"id":"pc","volume_percent":40},"item":{"id":"t2","name":"Second","duration_ms":1000,"artists":[{"id":"a","name":"A"}]}}`))
		case "GET /tracks/t2":
			_, _ = w.Write([]byte(`{"id":"t2","name":"Second","duration_ms":1000,"artists":[{"id":"a","name":"A"}]}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c := spotify.NewClient("cid", spotify.Token{AccessToken: "at"}, nil)
	c.SetBaseURL(srv.URL)
	p := New(spotify.New(c), nil)
	defer func() { _ = p.Close() }()

	if err := p.SetQueue([]string{"t1", "t2"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if playDevice != "pc" || len(played) != 2 || played[0] != "spotify:track:t1" {
		t.Errorf("play device=%q uris=%v (want the computer, both tracks)", playDevice, played)
	}
	mu.Unlock()

	time.Sleep(1500 * time.Millisecond) // one poll
	st, _ := p.GetState()
	if st.Track == nil || st.Track.Title != "Second" || !st.Playing || st.Position != 5*time.Second || st.Volume != 0.4 || st.RepeatMode != 1 {
		t.Errorf("state = %+v", st)
	}
	p.mu.Lock()
	if p.idx != 1 {
		t.Errorf("queue index should follow the device, got %d", p.idx)
	}
	p.mu.Unlock()
}

func TestLocalDeviceSurvivesPollOfAnotherActiveDevice(t *testing.T) {
	polled := make(chan struct{}, 1)
	var mu sync.Mutex
	var target string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/me/player":
			_, _ = w.Write([]byte(`{"is_playing":true,"device":{"id":"phone"},"item":{"id":"t1","name":"Song","artists":[]}}`))
			select {
			case polled <- struct{}{}:
			default:
			}
		case "/me/player/play":
			mu.Lock()
			target = r.URL.Query().Get("device_id")
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c := spotify.NewClient("cid", spotify.Token{AccessToken: "at"}, nil)
	c.SetBaseURL(srv.URL)
	p := New(spotify.New(c), nil)
	defer func() { _ = p.Close() }()
	p.SetDevice("local")
	select {
	case <-polled:
	case <-time.After(3 * time.Second):
		t.Fatal("poll did not run")
	}
	// Wait until the poll has applied its response.
	deadline := time.Now().Add(time.Second)
	for {
		st, _ := p.GetState()
		if st.Track != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("poll response was not applied")
		}
		time.Sleep(time.Millisecond)
	}
	if err := p.SetQueue([]string{"t1"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if target != "local" {
		t.Fatalf("play targeted %q, want local device", target)
	}
}

func TestEnsureDeviceSkipsRestrictedDevices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"devices":[{"id":"restricted","name":"Phone","is_active":true,"is_restricted":true},{"id":"pc","name":"PC","type":"Computer"}]}`))
	}))
	defer srv.Close()
	c := spotify.NewClient("cid", spotify.Token{AccessToken: "at"}, nil)
	c.SetBaseURL(srv.URL)
	p := New(spotify.New(c), nil)
	defer func() { _ = p.Close() }()
	ctx, cancel := ctx10()
	defer cancel()
	id, err := p.ensureDevice(ctx)
	if err != nil || id != "pc" {
		t.Fatalf("device = %q, error = %v", id, err)
	}
}

// A burst of volume changes reaches Spotify as one request with the last
// value (rounded, not truncated), and the device's late report of the old
// level doesn't pull the volume back.
func TestVolumeBurstAndStaleReport(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "PUT /me/player/volume":
			mu.Lock()
			sent = append(sent, r.URL.Query().Get("volume_percent"))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case "GET /me/player":
			// Spotify still reports the level from before the change.
			_, _ = w.Write([]byte(`{"is_playing":true,"device":{"id":"pc","volume_percent":20},"item":{"id":"t1","name":"S","duration_ms":1000,"artists":[{"id":"a","name":"A"}]}}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c := spotify.NewClient("cid", spotify.Token{AccessToken: "at"}, nil)
	c.SetBaseURL(srv.URL)
	p := New(spotify.New(c), nil)
	defer func() { _ = p.Close() }()
	p.SetDevice("pc")

	for _, v := range []float64{0.25, 0.30, 0.35} {
		_ = p.SetVolume(v)
	}
	time.Sleep(1500 * time.Millisecond) // the send and a poll
	mu.Lock()
	if len(sent) != 1 || sent[0] != "35" {
		t.Errorf("sent %v, want one request for 35", sent)
	}
	mu.Unlock()
	if st, _ := p.GetState(); st.Volume != 0.35 {
		t.Errorf("volume = %v after a stale report, want 0.35", st.Volume)
	}
}
