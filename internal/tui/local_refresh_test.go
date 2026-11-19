package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/provider"
	localProvider "github.com/tphx37/utuimp/internal/provider/local"
)

type refreshPlayer struct {
	*mockPlayer
	index []provider.Track
}

func (p *refreshPlayer) RefreshLibrary(ts []provider.Track) {
	p.index = append([]provider.Track(nil), ts...)
}

func TestLocalRefreshUpdatesIndexAndPreservesQueue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.mp3"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	prov, err := localProvider.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &refreshPlayer{mockPlayer: newMockPlayer()}
	m := newModel(p)
	m.serviceKey = "local"
	m.provider = prov
	m.cfg = &config.Config{MusicDirs: []string{dir}}
	m.queueTracks = []provider.Track{{ID: "old-queue"}}
	if err = os.WriteFile(filepath.Join(dir, "two.mp3"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := m.executeCommand("refresh")
	if cmd == nil {
		t.Fatal("missing refresh command")
	}
	msg, ok := cmd().(localRefreshMsg)
	if !ok || msg.err != nil {
		t.Fatalf("refresh result: %+v", msg)
	}
	m.Update(msg)
	if len(p.index) != 2 || len(m.queueTracks) != 1 || m.queueTracks[0].ID != "old-queue" {
		t.Fatalf("index=%+v queue=%+v", p.index, m.queueTracks)
	}
}
