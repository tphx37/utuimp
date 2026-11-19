package tui

import (
	"testing"

	"github.com/tphx37/utuimp/internal/provider"
)

func TestQueueFirstMoveStartsNextToPlayingTrack(t *testing.T) {
	m := newModel(newMockPlayer())
	m.queueTracks = []provider.Track{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	m.playerState.Track = &m.queueTracks[1]
	for _, tc := range []struct{ delta, want int }{{1, 2}, {-1, 0}} {
		m.queueMiniCursor = -1
		m.moveQueueMiniCursor(tc.delta)
		if m.queueMiniCursor != tc.want {
			t.Errorf("first move %d selected %d, want %d", tc.delta, m.queueMiniCursor, tc.want)
		}
	}
	m.playerState.Track = nil
	m.queueMiniCursor = -1
	m.moveQueueMiniCursor(1)
	if m.queueMiniCursor != 0 {
		t.Errorf("no playing track: selected %d, want 0", m.queueMiniCursor)
	}
}
