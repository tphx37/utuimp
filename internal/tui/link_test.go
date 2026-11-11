package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tphx37/utuimp/internal/provider"
)

type linkMockProvider struct {
	mockProvider
	looked []string
}

func (p *linkMockProvider) Lookup(_ context.Context, id string) (provider.Track, error) {
	p.looked = append(p.looked, id)
	return provider.Track{ID: id, Title: "Song " + id, Artist: "Someone"}, nil
}

func (p *linkMockProvider) GetAlbumTracks(_ context.Context, id string) ([]provider.Track, error) {
	return []provider.Track{{ID: "a1", Title: "One", Album: "The Album", Artist: "Band"}, {ID: "a2", Title: "Two", Album: "The Album", Artist: "Band"}}, nil
}

// runAll runs cmd and the parts of a batch, dropping their messages.
func runAll(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if b, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range b {
			runAll(c)
		}
	}
}

// firstMsg runs cmd and returns its message, unpacking a batch to the first
// message of type T.
func firstMsg[T any](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	var zero T
	if cmd == nil {
		t.Fatalf("no command, want %T", zero)
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, c := range b {
			if c == nil {
				continue
			}
			if v, ok := c().(T); ok {
				return v
			}
		}
		t.Fatalf("batch has no %T", zero)
	}
	v, ok := msg.(T)
	if !ok {
		t.Fatalf("got %T, want %T", msg, zero)
	}
	return v
}

func TestLinkCommand_OffersPlayAndQueue(t *testing.T) {
	plyr := newMockPlayer()
	m := newModel(plyr)
	prov := &linkMockProvider{}
	m.provider = prov
	m.serviceKey = "spotify"

	cmd := m.executeCommand("link https://open.spotify.com/track/AbC123?si=x")
	if m.linkOffer == nil || !m.linkOffer.loading {
		t.Fatal(":link should open the offer window, loading")
	}
	m.Update(firstMsg[linkResolvedMsg](t, cmd))
	if len(prov.looked) != 1 || prov.looked[0] != "AbC123" {
		t.Fatalf("looked up %v, want the link's track ID (case kept)", prov.looked)
	}
	lo := m.linkOffer
	if lo.loading || lo.err != nil || len(lo.items) != 3 {
		t.Fatalf("offer = %+v", lo)
	}

	// 3: add to queue.
	cmd = m.handleKey(tea.KeyPressMsg{Code: '3', Text: "3"})
	if m.linkOffer != nil {
		t.Error("the window should close after a choice")
	}
	_, cmd = m.Update(firstMsg[searchCollectionTracksMsg](t, cmd))
	if cmd != nil {
		cmd()
	}
	if len(m.queueTracks) != 1 || m.queueTracks[0].Title != "Song AbC123" {
		t.Fatalf("queue = %+v", m.queueTracks)
	}
	if len(plyr.appendQueueIDs) != 1 || plyr.appendQueueIDs[0][0] != "AbC123" {
		t.Errorf("player AppendQueue = %v", plyr.appendQueueIDs)
	}
}

func TestLinkCommand_AlbumPlaysAll(t *testing.T) {
	plyr := newMockPlayer()
	m := newModel(plyr)
	m.provider = &linkMockProvider{}
	m.serviceKey = "yandex"
	cmd := m.executeCommand("link https://music.yandex.ru/album/42")
	m.Update(firstMsg[linkResolvedMsg](t, cmd))
	if title, _ := m.linkOffer.title(); title != "The Album" {
		t.Errorf("title = %q", title)
	}
	cmd = m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, cmd = m.Update(firstMsg[searchCollectionTracksMsg](t, cmd))
	runAll(cmd)
	if len(plyr.setQueueIDs) != 2 {
		t.Errorf("SetQueue = %v, want the album's two songs", plyr.setQueueIDs)
	}
}

func TestLinkCommand_OtherServiceRoutes(t *testing.T) {
	m := newModel(newMockPlayer())
	m.serviceKey = "apple"
	cmd := m.executeCommand("link https://music.yandex.ru/album/1/track/2")
	r := firstMsg[linkRouteMsg](t, cmd)
	if r.link.Service != "yandex" || r.link.ID != "2:1" || r.search {
		t.Errorf("route = %+v", r)
	}
	if m.linkOffer != nil {
		t.Error("the other tab shows the offer, not this one")
	}
}

func TestSearchLink_ResolvesInPlace(t *testing.T) {
	m := newModel(newMockPlayer())
	m.provider = &linkMockProvider{}
	m.serviceKey = "youtube"
	m.mode = modeSearch
	m.insertSearchText("https://youtu.be/xyz", true)
	_, cmd := m.Update(searchDebounceMsg{query: m.searchQuery, gen: m.searchGen})
	res := firstMsg[searchResultMsg](t, cmd)
	m.Update(res)
	if res.err != nil || res.result == nil || len(res.result.Tracks) != 1 || res.result.Tracks[0].ID != "xyz" {
		t.Fatalf("result = %+v, %v", res.result, res.err)
	}
	if tr := m.search.SelectedTrack(); tr == nil || tr.ID != "xyz" {
		t.Errorf("search should show the linked song, selected = %+v", tr)
	}
}

func TestApp_RoutesLinkToItsTab(t *testing.T) {
	app, services := testApp(t)
	services[1].Model.provider = &linkMockProvider{}
	l, _ := provider.ParseLink("https://music.apple.com/us/song/x/123")
	app.Update(sessionMsg{idx: 0, msg: linkRouteMsg{link: l}})
	if app.Active() != 1 {
		t.Fatalf("active tab = %d, want Apple Music", app.Active())
	}
	if services[1].Model.linkOffer == nil {
		t.Error("Apple Music tab should show the :link window")
	}

	// A service without a tab here: the origin tab says so.
	l, _ = provider.ParseLink("https://soundcloud.com/a/b")
	app.Update(sessionMsg{idx: 1, msg: linkRouteMsg{link: l}})
	if app.Active() != 1 || services[1].Model.errMsg == "" {
		t.Errorf("missing service: active=%d note=%q", app.Active(), services[1].Model.errMsg)
	}
}
