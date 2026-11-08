package spotifyconnect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/provider/spotify"
)

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
	defer p.Close()

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
