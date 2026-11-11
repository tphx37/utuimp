package spotifyconnect

import "testing"

func TestFindLibrespotStream(t *testing.T) {
	list := []byte(`[
	 {"index":2892,"properties":{"application.name":"utuimp","application.process.binary":"utuimp"}},
	 {"index":2898,"properties":{"application.name":"PipeWire ALSA [librespot]","node.name":"alsa_playback.librespot"}},
	 {"index":2901,"properties":{"application.name":"Firefox"}}]`)
	if got := findLibrespotStream(list); got != "2898" {
		t.Errorf("stream = %q, want 2898", got)
	}
	if got := findLibrespotStream([]byte(`[{"index":1,"properties":{"application.name":"mpv"}}]`)); got != "" {
		t.Errorf("stream = %q, want none", got)
	}
	if got := findLibrespotStream([]byte("Failure: Connection refused")); got != "" {
		t.Errorf("stream = %q from an error message", got)
	}
}
