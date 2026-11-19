//go:build linux && cgo

package radio

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
)

// A real GStreamer pipeline must decode a stream selected through the radio player.
func TestRadioPlaysStream(t *testing.T) {
	t.Setenv("UTUIMP_NULL_AUDIO", "1")
	frame := []byte{0xff, 0xf1, 0x50, 0x80, 0x01, 0xbf, 0xfc, 0x21, 0x10, 0x04, 0x60, 0x8c, 0x1c}
	audio := bytes.Repeat(frame, 70)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/aac")
		_, _ = w.Write(audio)
	}))
	defer srv.Close()
	id := srv.URL + "/stream.aac"
	src := testSource{tracks: map[string]provider.Track{id: {ID: id, Title: "Test station"}}}
	p, err := New(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	updates := p.Subscribe()
	if err = p.SetQueue([]string{id}); err != nil {
		t.Fatal(err)
	}
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case s := <-updates:
			if s.Error != "" {
				t.Fatalf("playback error: %s", s.Error)
			}
			if s.Track != nil && !s.Playing {
				return
			}
		case <-timeout.C:
			t.Fatal("stream did not finish playing")
		}
	}
}
