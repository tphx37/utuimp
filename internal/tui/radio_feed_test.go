package tui

import (
	"path/filepath"
	"testing"

	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/localstore"
	radioProvider "github.com/tphx37/utuimp/internal/provider/radio"
)

func TestRadioFeedUsesPersistedRecentLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	saveHistory(path, []provider.Track{
		{ID: "https://example.org/one", Title: "Latest song", Artist: "Artist"},
		{ID: "https://example.org/two", Title: "Second station"},
		{ID: "https://example.org/one", Title: "Previous song"},
		{ID: "not-a-link", Title: "Invalid"},
	})
	store, err := localstore.Open("")
	if err != nil {
		t.Fatal(err)
	}
	prov := radioProvider.New(store)
	m := newModel(newMockPlayer())
	m.serviceKey = "radio"
	m.provider = prov
	m.history = loadHistory(path)
	msg, ok := m.fetchFeedCmd()().(feedResultMsg)
	if !ok || msg.err != nil || len(msg.groups) != 1 {
		t.Fatalf("feed: %+v", msg)
	}
	items := msg.groups[0].Items
	if len(items) != 2 || items[0].ID != "https://example.org/one" || items[0].Title != "Latest song" || items[1].ID != "https://example.org/two" {
		t.Fatalf("items: %+v", items)
	}
	if tr, ok := prov.Lookup(items[0].ID); !ok || tr.Title != "Latest song" {
		t.Fatalf("station not restored for playback: %+v", tr)
	}
	m.Update(msg)
	if item := m.feedP.m.SelectedItem(); item == nil || item.ID != items[0].ID {
		t.Fatalf("selection: %+v", item)
	}
}

func TestRadioFeedEmptyAndConnecting(t *testing.T) {
	m := newModel(newMockPlayer())
	m.serviceKey = "radio"
	m.provider = provider.NotReady{}
	if msg := m.fetchFeedCmd()().(feedResultMsg); msg.err != provider.ErrNotReady {
		t.Fatalf("pending: %+v", msg)
	}
	store, _ := localstore.Open("")
	m.provider = radioProvider.New(store)
	if msg := m.fetchFeedCmd()().(feedResultMsg); msg.err != nil || len(msg.groups) != 0 {
		t.Fatalf("empty: %+v", msg)
	}
}
