package radio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
)

type testSource struct{ tracks map[string]provider.Track }

func (s testSource) Lookup(id string) (provider.Track, bool) { t, ok := s.tracks[id]; return t, ok }
func (s testSource) GetPlaylistTracks(context.Context, string) ([]provider.Track, error) {
	return []provider.Track{s.tracks["https://example.org/live.mp3"]}, nil
}

type testSink struct {
	mu  sync.Mutex
	uri string
}

func (s *testSink) PlayURI(u string)         { s.mu.Lock(); s.uri = u; s.mu.Unlock() }
func (*testSink) SetNextURI(string)          {}
func (*testSink) Play()                      {}
func (*testSink) Pause()                     {}
func (*testSink) Stop()                      {}
func (*testSink) Seek(time.Duration)         {}
func (*testSink) SetVolume(float64)          {}
func (*testSink) SetEqualizer([]float64)     {}
func (*testSink) Position() time.Duration    { return 0 }
func (*testSink) OnEOS(func())               {}
func (*testSink) OnError(func(error))        {}
func (*testSink) OnStreamStart(func(string)) {}
func (*testSink) Destroy()                   {}

func TestLaterStationCancelsPendingPlaylist(t *testing.T) {
	entered := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer srv.Close()
	first := srv.URL + "/station.m3u"
	second := "https://example.org/live.mp3"
	source := testSource{map[string]provider.Track{first: {ID: first, Title: "First"}, second: {ID: second, Title: "Second"}}}
	sink := &testSink{}
	p := &Player{sink: sink, source: source, idx: -1, state: player.State{Volume: 1}}
	done := make(chan error, 1)
	go func() { done <- p.SetQueue([]string{first}) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("playlist request did not start")
	}
	if err := p.SetQueue([]string{second}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("old playlist request was not canceled")
	}
	sink.mu.Lock()
	got := sink.uri
	sink.mu.Unlock()
	if got != second {
		t.Fatalf("playing %q, want %q", got, second)
	}
	state, _ := p.GetState()
	if state.Track == nil || state.Track.ID != second {
		t.Fatalf("state: %+v", state)
	}
	_ = p.Close()
}

func TestStreamMetadataUpdatesTrackWithoutChangingStationID(t *testing.T) {
	id := "https://example.org/live"
	p := &Player{sink: &testSink{}, state: player.State{Playing: true, Track: &provider.Track{ID: id, Title: id, Artist: "Internet radio"}}}
	updates := p.Subscribe()
	defer func() { _ = p.Close() }()
	p.updateMetadata("Artist - Track", "", "Album", "Station")
	select {
	case s := <-updates:
		if s.Track == nil || s.Track.ID != id || s.Track.Title != "Track" || s.Track.Artist != "Artist" || s.Track.Album != "Album" {
			t.Fatalf("metadata: %+v", s.Track)
		}
	case <-time.After(time.Second):
		t.Fatal("no metadata update")
	}
	p.updateMetadata("Next track", "Other artist", "", "")
	state, _ := p.GetState()
	if state.Track.Title != "Next track" || state.Track.Artist != "Other artist" || state.Track.Album != "" || state.Track.ID != id {
		t.Fatalf("second update: %+v", state.Track)
	}
}
