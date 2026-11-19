//go:build linux

package local

import (
	"testing"

	"github.com/tphx37/utuimp/internal/provider"
)

func TestRefreshLibraryPreservesQueue(t *testing.T) {
	p := &Player{allTracks: []provider.Track{{ID: "old"}}, queue: []provider.Track{{ID: "playing"}}, idx: 0}
	p.RefreshLibrary([]provider.Track{{ID: "new"}})
	if len(p.queue) != 1 || p.queue[0].ID != "playing" || len(p.allTracks) != 1 || p.allTracks[0].ID != "new" {
		t.Fatalf("queue=%+v library=%+v", p.queue, p.allTracks)
	}
}
