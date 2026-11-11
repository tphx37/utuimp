package tui

import (
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
)

// Between two slow player reports (Spotify's come every 3 s) the lyrics
// clock keeps running instead of stalling and snapping back.
func TestLyricsClockRunsBetweenSlowReports(t *testing.T) {
	m := newModel(newMockPlayer())
	tr := &provider.Track{ID: "1", Title: "A", Duration: 3 * time.Minute}
	m.playerState.Track = tr
	m.playerState.Playing = true
	m.playerState.Position = 10 * time.Second
	now := time.Now()
	m.stateAt = now.Add(-2800 * time.Millisecond)
	m.lyricsClock = 12760 * time.Millisecond
	m.lyricsClockAt = now.Add(-40 * time.Millisecond)
	m.advanceLyricsClock()
	if m.lyricsClock < 12760*time.Millisecond {
		t.Fatalf("lyrics clock went back to %v between reports", m.lyricsClock)
	}
}
