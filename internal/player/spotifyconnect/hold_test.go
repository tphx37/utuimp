package spotifyconnect

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/provider/spotify"
)

type fakeOut struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeOut) add(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }
func (f *fakeOut) Play()        { f.add("play") }
func (f *fakeOut) Pause()       { f.add("pause") }
func (f *fakeOut) Flush()       { f.add("flush") }
func (f *fakeOut) Hold()        { f.add("hold") }
func (f *fakeOut) Release(play bool, skip int) {
	if play {
		f.add("release-play")
	} else {
		f.add("release-paused")
	}
}
func (f *fakeOut) log() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, ",")
}

func newHoldPlayer(t *testing.T, playing *bool, mu *sync.Mutex) (*Player, *fakeOut, chan Event) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/me/player" {
			mu.Lock()
			pl := *playing
			mu.Unlock()
			body := `{"is_playing":false,"progress_ms":1000,"device":{"id":"d"},"item":{"id":"t1","name":"One","duration_ms":100000,"artists":[{"id":"a","name":"A"}]}}`
			if pl {
				body = strings.Replace(body, `"is_playing":false`, `"is_playing":true`, 1)
			}
			_, _ = w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c := spotify.NewClient("cid", spotify.Token{AccessToken: "at"}, nil)
	c.SetBaseURL(srv.URL)
	p := New(spotify.New(c), nil)
	t.Cleanup(func() { _ = p.Close() })
	p.SetDevice("d")
	out := &fakeOut{}
	p.SetOutput(out)
	ev := make(chan Event, 4)
	p.FollowEvents(ev)
	return p, out, ev
}

// A skip silences the output until librespot says the new song plays,
// instead of letting the old one run on (or jump ahead) meanwhile.
func TestSkipHoldsUntilLibrespotPlays(t *testing.T) {
	var mu sync.Mutex
	playing := true
	p, out, ev := newHoldPlayer(t, &playing, &mu)
	p.mu.Lock()
	p.state.Playing = true
	p.mu.Unlock()
	_ = p.Next()
	time.Sleep(50 * time.Millisecond)
	if got := out.log(); got != "hold" {
		t.Fatalf("after Next the output got %q, want just a hold", got)
	}
	ev <- Event{Name: "loading"}
	ev <- Event{Name: "playing", Pending: 4096}
	time.Sleep(50 * time.Millisecond)
	if got := out.log(); got != "hold,release-play" {
		t.Fatalf("after librespot played the output got %q", got)
	}
}

// A pause given here isn't undone by Spotify's late report that it still
// plays: the output must not be started again.
func TestPauseIgnoresStalePlayingReport(t *testing.T) {
	var mu sync.Mutex
	playing := true // Spotify hasn't applied the pause yet
	p, out, ev := newHoldPlayer(t, &playing, &mu)
	p.mu.Lock()
	p.state.Playing = true
	p.state.Track = nil
	p.mu.Unlock()
	_ = p.Pause()
	ev <- Event{Name: "playing"}        // an event from before the pause
	time.Sleep(1200 * time.Millisecond) // the poll after the command
	if strings.Contains(out.log(), "play") {
		t.Errorf("the output was started again after a pause: %q", out.log())
	}
	if st, _ := p.GetState(); st.Playing {
		t.Error("the stale report showed the song as playing again")
	}
}
