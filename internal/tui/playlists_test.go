package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/views"
)

// editProvider records playlist edits.
type editProvider struct {
	mockProvider
	playlists []provider.Playlist
	tracks    map[string][]provider.Track
	created   []string
	createIDs [][]string
	renamed   map[string]string
	deleted   []string
	removed   []string // "playlist/index/track"
	library   []string
}

func newEditProvider() *editProvider {
	return &editProvider{
		playlists: []provider.Playlist{{ID: "fav", Name: "Favorites", ReadOnly: true}, {ID: "p1", Name: "Chill"}},
		tracks: map[string][]provider.Track{"p1": {
			{ID: "a", Title: "Alpha"}, {ID: "b", Title: "Beta"}, {ID: "c", Title: "Gamma"},
		}},
		renamed: map[string]string{},
	}
}

func (p *editProvider) GetLibraryPlaylists(context.Context) ([]provider.Playlist, error) {
	return p.playlists, nil
}
func (p *editProvider) GetPlaylistTracks(_ context.Context, id string) ([]provider.Track, error) {
	return p.tracks[id], nil
}
func (p *editProvider) CreatePlaylist(_ context.Context, name string, ids []string) (provider.Playlist, error) {
	p.created = append(p.created, name)
	p.createIDs = append(p.createIDs, ids)
	pl := provider.Playlist{ID: "new-" + name, Name: name}
	p.playlists = append(p.playlists, pl)
	return pl, nil
}
func (p *editProvider) RemoveFromPlaylist(_ context.Context, id string, idx int, t provider.Track) error {
	p.removed = append(p.removed, id+"/"+string(rune('0'+idx))+"/"+t.ID)
	return nil
}
func (p *editProvider) RenamePlaylist(_ context.Context, id, name string) error {
	p.renamed[id] = name
	return nil
}
func (p *editProvider) DeletePlaylist(_ context.Context, id string) error {
	p.deleted = append(p.deleted, id)
	return nil
}
func (p *editProvider) AddToLibrary(_ context.Context, t provider.Track) error {
	p.library = append(p.library, t.ID)
	return nil
}

// run feeds cmd's messages back into the model, batches included. Timers
// (spinners, animation ticks) are not run: they wait and come back forever.
func run(m *Model, cmd tea.Cmd) { runDepth(m, cmd, 0) }

func runDepth(m *Model, cmd tea.Cmd, depth int) {
	if cmd == nil || depth > 6 {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(20 * time.Millisecond):
		return // a timer
	}
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			runDepth(m, c, depth+1)
		}
	default:
		if strings.HasSuffix(fmt.Sprintf("%T", msg), "TickMsg") {
			return
		}
		_, next := m.Update(msg)
		runDepth(m, next, depth+1)
	}
}

func key(m *Model, k string) {
	var msg tea.KeyPressMsg
	switch k {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		msg = tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+q":
		msg = tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
	case "shift+tab":
		msg = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		r := []rune(k)
		msg = tea.KeyPressMsg{Code: r[0], Text: k}
	}
	run(m, m.handleKey(msg))
}

func typeText(m *Model, s string) {
	for _, r := range s {
		if r == ' ' {
			run(m, m.handleKey(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}))
			continue
		}
		key(m, string(r))
	}
}

func editModel(t *testing.T) (*Model, *editProvider) {
	t.Helper()
	prov := newEditProvider()
	cfg := testCfg()
	cfg.LibraryDelete, cfg.LibraryEdit = true, true
	m := New(cfg, prov, newMockPlayer(), Options{})
	m.noAnim = true
	m.historyPath, m.history = "", nil
	m.width, m.height = 120, 40
	m.introStep = introDone
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m, prov
}

// openLibraryPlaylists opens the library's list of playlists.
func openLibraryPlaylists(t *testing.T, m *Model) {
	t.Helper()
	key(m, "L")
	for range 3 { // Songs, Albums, Artists, Playlists
		key(m, "down")
	}
	key(m, "enter")
	if !m.library.m.ShowingPlaylists() {
		t.Fatal("the playlists should be shown")
	}
}

func TestLibraryPlaylistCreateRenameDelete(t *testing.T) {
	m, prov := editModel(t)
	openLibraryPlaylists(t, m)

	// c: a new playlist, named in the prompt.
	key(m, "c")
	if m.prompt == nil {
		t.Fatal("c should ask for the new playlist's name")
	}
	typeText(m, "Road trip")
	key(m, "enter")
	if len(prov.created) != 1 || prov.created[0] != "Road trip" {
		t.Fatalf("created = %v", prov.created)
	}
	if pl := m.library.m.SelectedPlaylist(); pl == nil || pl.Name != "Road trip" {
		t.Errorf("the new playlist should be listed and highlighted, got %+v", pl)
	}

	// r: renamed.
	key(m, "r")
	if m.prompt == nil || string(m.prompt.buf) != "Road trip" {
		t.Fatal("r should ask for the new name, starting from the old one")
	}
	run(m, m.handleKey(tea.KeyPressMsg{Code: tea.KeyBackspace}))
	typeText(m, "P!")
	key(m, "enter")
	if prov.renamed["new-Road trip"] != "Road triP!" {
		t.Fatalf("renamed = %v", prov.renamed)
	}

	// d once only asks; d d deletes.
	key(m, "d")
	if len(prov.deleted) != 0 || !strings.Contains(m.errMsg, "again") {
		t.Fatalf("one d must only ask (deleted=%v, status=%q)", prov.deleted, m.errMsg)
	}
	key(m, "d")
	if len(prov.deleted) != 1 || prov.deleted[0] != "new-Road trip" {
		t.Fatalf("deleted = %v", prov.deleted)
	}

	// The favourites can't be renamed or deleted.
	m.library.m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	key(m, "d")
	key(m, "d")
	if len(prov.deleted) != 1 {
		t.Errorf("a read-only playlist was deleted: %v", prov.deleted)
	}
}

func TestLibraryRemoveSongFromPlaylistNeedsTwoPresses(t *testing.T) {
	m, prov := editModel(t)
	openLibraryPlaylists(t, m)
	key(m, "down") // Chill
	key(m, "enter")
	key(m, "down") // Beta
	if pl, tr, idx := m.library.m.OpenPlaylist(); pl == nil || tr == nil || tr.ID != "b" || idx != 1 {
		t.Fatalf("open playlist = %v %v %d", pl, tr, idx)
	}
	key(m, "d")
	if len(prov.removed) != 0 {
		t.Fatal("one d must only ask")
	}
	key(m, "d")
	if len(prov.removed) != 1 || prov.removed[0] != "p1/1/b" {
		t.Fatalf("removed = %v", prov.removed)
	}
	if _, tr, _ := m.library.m.OpenPlaylist(); tr == nil || tr.ID == "b" {
		t.Errorf("the song should be gone from the list, selected now: %v", tr)
	}
}

// Inside a playlist tab puts the highlighted song alone right after the
// playing one, shift+tab at the end; ctrl+q the whole playlist, after the
// playing song.
func TestLibraryTabNextShiftTabEndCtrlQWholeListNext(t *testing.T) {
	m, _ := editModel(t)
	plyr := m.player.(*mockPlayer)
	cur := provider.Track{ID: "now", Title: "Now"}
	m.queueTracks = []provider.Track{cur, {ID: "later", Title: "Later"}}
	m.queueIDs = []string{"now", "later"}
	m.queue.SetTracks(m.queueTracks)
	m.playerState.Track = &cur

	openLibraryPlaylists(t, m)
	key(m, "down")
	key(m, "enter")
	key(m, "down") // Beta
	key(m, "tab")
	if got := m.queueIDs; strings.Join(got, ",") != "now,b,later" {
		t.Fatalf("tab: queue = %v, want the song alone right after the playing one", got)
	}
	key(m, "ctrl+q")
	if got := m.queueIDs; strings.Join(got, ",") != "now,a,b,c,b,later" {
		t.Fatalf("ctrl+q: queue = %v, want the whole playlist after the playing song", got)
	}
	key(m, "shift+tab")
	if got := m.queueIDs; strings.Join(got, ",") != "now,a,b,c,b,later,b" {
		t.Fatalf("shift+tab: queue = %v, want the song alone at the end", got)
	}
	if len(plyr.appendQueueIDs) == 0 {
		t.Error("the player should have been given the songs")
	}
}

// In the queue, shift+tab moves the selected song to play next.
func TestQueueShiftTabPlaysNext(t *testing.T) {
	m := newModel(newMockPlayer())
	m.width, m.height = 120, 40
	plyr := m.player.(*mockPlayer)
	tracks := []provider.Track{{ID: "1", Title: "One"}, {ID: "2", Title: "Two"}, {ID: "3", Title: "Three"}, {ID: "4", Title: "Four"}}
	m.queueTracks = append([]provider.Track{}, tracks...)
	m.queueIDs = []string{"1", "2", "3", "4"}
	m.queue.SetTracks(m.queueTracks)
	m.playerState.Track = &tracks[1] // Two plays
	m.queueMiniCursor = 3            // Four selected
	cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if got := strings.Join(m.queueIDs, ","); got != "1,2,4,3" {
		t.Fatalf("queue = %s, want Four right after Two", got)
	}
	if m.queueMiniCursor != 2 {
		t.Errorf("the selection should follow the song, at %d", m.queueMiniCursor)
	}
	run(m, cmd)
	if len(plyr.moveInQueueCalls) != 1 || plyr.moveInQueueCalls[0].From != 3 || plyr.moveInQueueCalls[0].To != 2 {
		t.Errorf("player moves = %+v", plyr.moveInQueueCalls)
	}
}

// "Play next" lands after the playing song even when the player reports
// it under another ID than the queue's (library vs catalog), instead of at
// the top of the queue where it couldn't be seen.
func TestPlayNextAfterSongReportedUnderAnotherID(t *testing.T) {
	m := newModel(newMockPlayer())
	m.queueTracks = []provider.Track{{ID: "i.1", Title: "One"}, {ID: "i.2", Title: "Two"}, {ID: "i.3", Title: "Three"}}
	m.queueIDs = []string{"i.1", "i.2", "i.3"}
	m.queue.SetTracks(m.queueTracks)
	m.playerState.Track = &provider.Track{ID: "999", Title: "Two"}
	nt := provider.Track{ID: "x", Title: "New"}
	m.playNextCmd("New", []provider.Track{nt}, []string{"x"})
	if got := strings.Join(m.queueIDs, ","); got != "i.1,i.2,x,i.3" {
		t.Fatalf("queue = %s", got)
	}
	if got := views.TrackLabel(m.queue.m.Tracks()[2]); !strings.Contains(got, "New") {
		t.Errorf("the queue view should show it, got %q", got)
	}
}

// P opens the playlist picker for the playing song from the main screen,
// and its last row makes a new playlist with the song in it.
func TestAddToPlaylistFromAnywhereAndNewPlaylist(t *testing.T) {
	m, prov := editModel(t)
	cur := provider.Track{ID: "s1", CatalogID: "s1", Title: "Song"}
	m.playerState = player.State{Track: &cur, Playing: true}
	for _, cover := range []bool{false, true} {
		if cover != m.coverView {
			m.toggleCoverView()
		}
		key(m, "P")
		if m.mode != modePlaylistPicker || m.playlistPickerTrack == nil || m.playlistPickerTrack.ID != "s1" {
			t.Fatalf("cover=%v: P should open the picker for the playing song", cover)
		}
		key(m, "esc")
	}
	key(m, "P")
	for range len(m.playlistPickerItems) {
		key(m, "j")
	}
	key(m, "enter") // "+ New playlist…"
	if m.prompt == nil {
		t.Fatal("the picker's last row should ask for a new playlist's name")
	}
	typeText(m, "Mix")
	key(m, "enter")
	if len(prov.created) != 1 || prov.created[0] != "Mix" || len(prov.createIDs[0]) != 1 || prov.createIDs[0][0] != "s1" {
		t.Fatalf("created %v with %v", prov.created, prov.createIDs)
	}
}

func TestSongMenuAddToLibrary(t *testing.T) {
	m, prov := editModel(t)
	cur := provider.Track{ID: "123", CatalogID: "123", Title: "Song"}
	m.playerState = player.State{Track: &cur, Playing: true}
	key(m, "i")
	if m.songMenu == nil {
		t.Fatal("no song menu")
	}
	n := -1
	for i, it := range m.songMenu.items {
		if it.kind == songMenuAddToLibrary {
			n = i
		}
	}
	if n < 0 {
		t.Fatal("the song menu should offer adding the song to the library")
	}
	key(m, string(rune('1'+n)))
	if len(prov.library) != 1 || prov.library[0] != "123" {
		t.Fatalf("library adds = %v", prov.library)
	}

	// A library song is in the library already.
	lib := provider.Track{ID: "i.5", Title: "Mine"}
	m.playerState.Track = &lib
	key(m, "i")
	for _, it := range m.songMenu.items {
		if it.kind == songMenuAddToLibrary {
			t.Error("a library song shouldn't be offered to the library")
		}
	}
}

// Opening and closing the song menu doesn't bring up what is behind it again.
func TestSongMenuDoesNotReplayPanelFade(t *testing.T) {
	m, _ := editModel(t)
	m.noAnim = false
	m.cfg.UIMotion = "lively"
	cur := provider.Track{ID: "1", Title: "Song"}
	m.playerState = player.State{Track: &cur, Playing: true}
	key(m, "L")
	m.Update(fxTickMsg{})
	before := m.fxState().wipeAt
	m.Update(m.handleKey(tea.KeyPressMsg{Code: 'i', Text: "i"}))
	m.Update(m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape}))
	if !m.fxState().wipeAt.Equal(before) {
		t.Error("the song menu opening or closing replayed the panel's fade")
	}
}

// With the setup's switches off (the default) nothing is deleted or
// renamed, and the status line says where to turn it on.
func TestLibraryDeleteAndEditNeedTheSwitches(t *testing.T) {
	m, prov := editModel(t)
	m.cfg.LibraryDelete, m.cfg.LibraryEdit = false, false
	openLibraryPlaylists(t, m)
	key(m, "down") // Chill
	key(m, "d")
	key(m, "d")
	if len(prov.deleted) != 0 || !strings.Contains(m.errMsg, "setup") {
		t.Fatalf("deleted %v with deleting off (status %q)", prov.deleted, m.errMsg)
	}
	key(m, "r")
	if m.prompt != nil {
		t.Fatal("renaming should be off")
	}
	key(m, "enter")
	key(m, "d")
	key(m, "d")
	if len(prov.removed) != 0 {
		t.Fatalf("removed %v with deleting off", prov.removed)
	}
	key(m, "c") // making a playlist is adding: always allowed
	if m.prompt == nil {
		// c works in the list of playlists only; go back there.
		key(m, "esc")
		key(m, "c")
	}
	if m.prompt == nil {
		t.Error("making a new playlist should stay allowed")
	}
}

// Taking a like back (f twice on a favourite) is deleting: it needs the
// switch too, while liking always works.
func TestUnlikeNeedsDeleteSwitch(t *testing.T) {
	m, _ := editModel(t)
	m.cfg.LibraryDelete = false
	cur := provider.Track{ID: "s", CatalogID: "s", Title: "Song"}
	m.playerState = player.State{Track: &cur, Playing: true}
	key(m, "f")
	if !m.isTrackFavorite(&cur) {
		t.Fatal("liking should always work")
	}
	key(m, "f")
	key(m, "f")
	if !m.isTrackFavorite(&cur) || !strings.Contains(m.errMsg, "setup") {
		t.Fatalf("the like was taken back with deleting off (status %q)", m.errMsg)
	}
	m.cfg.LibraryDelete = true
	key(m, "f")
	key(m, "f")
	if m.isTrackFavorite(&cur) {
		t.Error("with deleting on, f twice should take the like back")
	}
}
