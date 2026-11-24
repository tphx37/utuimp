package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/styles"
	"github.com/tphx37/utuimp/internal/tui/views"
)

// Playlists are made, renamed and deleted in the library's list of
// playlists (c, r, d d), songs are taken out of an open playlist with d d,
// and P (or the song menu) adds the selected or playing song to one, where
// the picker can also make a new one. Deleting asks for d twice in a row.

// ── Name prompt ──────────────────────────────────────────────────────────────

// textPrompt is a small window asking for one line of text: a playlist's
// name.
type textPrompt struct {
	title      string
	buf        []rune
	submit     func(text string) tea.Cmd
	allowEmpty bool
}

const promptMaxRunes = 4096

// openPrompt asks for a line of text, starting from initial; submit gets it
// trimmed (never empty).
func (m *Model) openPrompt(title, initial string, submit func(string) tea.Cmd) {
	m.prompt = &textPrompt{title: title, buf: []rune(initial), submit: submit}
}

func (m *Model) handlePromptKey(k string, msg tea.KeyPressMsg) tea.Cmd {
	p := m.prompt
	switch k {
	case "esc":
		m.prompt = nil
	case "enter":
		text := strings.TrimSpace(string(p.buf))
		if text == "" && !p.allowEmpty {
			return nil
		}
		m.prompt = nil
		return p.submit(text)
	case "backspace":
		if len(p.buf) > 0 {
			p.buf = p.buf[:len(p.buf)-1]
		}
	case "ctrl+u":
		p.buf = nil
	case "ctrl+w":
		end := len(p.buf)
		for end > 0 && p.buf[end-1] == ' ' {
			end--
		}
		for end > 0 && p.buf[end-1] != ' ' {
			end--
		}
		p.buf = p.buf[:end]
	case "space":
		m.promptInsert(" ")
	default:
		if s := extractPrintableKeyText(k, msg); s != "" {
			m.promptInsert(s)
		}
	}
	return nil
}

func (m *Model) promptInsert(raw string) {
	p := m.prompt
	if p == nil {
		return
	}
	add := sanitizeSingleLineInput(raw, promptMaxRunes-len(p.buf), true)
	p.buf = append(p.buf, add...)
}

func (m *Model) renderPrompt() string {
	p := m.prompt
	innerW := min(46, max(20, m.width-10))
	muted := styles.QueueItemMuted
	text := string(p.buf)
	// Keep the end (where typing happens) in view.
	for lipgloss.Width(text) > innerW-1 {
		r := []rune(text)
		text = string(r[1:])
	}
	lines := []string{
		styles.NowPlayingTitlePlaying.Render(truncateStr(p.title, innerW)),
		muted.Render(strings.Repeat("─", innerW)),
		styles.Header.Render(text) + styles.KeyName.Render("█"),
		muted.Render(strings.Repeat("─", innerW)),
		muted.Render(i18n.T("⏎ save · esc cancel")),
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorBorderFocus).
		Padding(0, 1).
		Width(innerW + 4).
		Render(strings.Join(lines, "\n"))
}

// ── Messages ─────────────────────────────────────────────────────────────────

// playlistEditedMsg reports a change to the playlists made here.
type playlistEditedMsg struct {
	what  string // "created", "renamed", "deleted", "removed", "library"
	pl    provider.Playlist
	track provider.Track
	idx   int
	err   error
	// id and name of a saved album or followed artist
	id, name string
}

func (m *Model) note(s string) {
	m.errMsg = s
	m.errExpiry = time.Now().Add(3 * time.Second)
}

func (m *Model) handlePlaylistEdited(msg playlistEditedMsg) tea.Cmd {
	if msg.err != nil {
		m.appendLog(fmt.Sprintf("[playlist] %s: %v", msg.what, msg.err))
		m.note(fmt.Sprintf(i18n.T("✗ %v"), msg.err))
		return nil
	}
	lib := m.library.m
	switch msg.what {
	case "created":
		m.appendLog(fmt.Sprintf("[playlist] created %q", msg.pl.Name))
		if msg.track.ID != "" {
			m.note(fmt.Sprintf(i18n.T("✓ Playlist \"%s\" created with the song"), msg.pl.Name))
		} else {
			m.note(fmt.Sprintf(i18n.T("✓ Playlist \"%s\" created"), msg.pl.Name))
		}
		return lib.PlaylistsChanged(msg.pl.ID)
	case "renamed":
		m.appendLog(fmt.Sprintf("[playlist] renamed to %q", msg.pl.Name))
		m.note(fmt.Sprintf(i18n.T("✓ Renamed to \"%s\""), msg.pl.Name))
		lib.RenamedPlaylist(msg.pl.ID, msg.pl.Name)
	case "deleted":
		m.appendLog(fmt.Sprintf("[playlist] deleted %q", msg.pl.Name))
		m.note(fmt.Sprintf(i18n.T("✓ Playlist \"%s\" deleted"), msg.pl.Name))
		return lib.PlaylistsChanged("")
	case "removed":
		m.appendLog(fmt.Sprintf("[playlist] removed %s from %q", views.TrackLabel(msg.track), msg.pl.Name))
		m.note(fmt.Sprintf(i18n.T("✓ Removed from \"%s\""), msg.pl.Name))
		lib.RemovedFromPlaylist(msg.pl.ID, msg.idx, msg.track.ID)
		if provider.IsFavoritesPlaylist(msg.pl) || msg.pl.ReadOnly {
			m.setTrackFavorite(&msg.track, false)
		}
	case "library":
		m.appendLog("[library] added " + views.TrackLabel(msg.track))
		m.note(i18n.T("✓ Added to your library"))
		lib.InvalidateTracksCache()
	case "album+", "artist+":
		m.appendLog("[library] saved " + msg.name)
		if msg.what == "album+" {
			m.note(fmt.Sprintf(i18n.T("✓ Album \"%s\" saved to the library"), msg.name))
		} else {
			m.note(fmt.Sprintf(i18n.T("✓ Following \"%s\""), msg.name))
		}
		return lib.SavedChanged()
	case "album-", "artist-":
		m.appendLog("[library] removed " + msg.name)
		m.note(fmt.Sprintf(i18n.T("✓ \"%s\" removed from the library"), msg.name))
		lib.RemovedSaved(msg.id)
	}
	return nil
}

// ── Actions ──────────────────────────────────────────────────────────────────

func (m *Model) playlistEditor() (provider.PlaylistEditor, bool) {
	ed, ok := m.provider.(provider.PlaylistEditor)
	return ed, ok
}

// trackIDForPlaylist is the ID a song is added to a playlist by: the
// catalog one for a song that has both (the library one stays for songs
// only in the library).
func trackIDForPlaylist(t provider.Track) string {
	if !strings.HasPrefix(t.ID, "i.") && t.CatalogID != "" {
		return t.CatalogID
	}
	return t.ID
}

// promptNewPlaylist asks for a name and makes a playlist, with t in it
// when t is not nil.
func (m *Model) promptNewPlaylist(t *provider.Track) {
	var track provider.Track
	if t != nil {
		track = *t
	}
	prov := m.provider
	m.openPrompt(i18n.T("New playlist"), "", func(name string) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var ids []string
			if track.ID != "" {
				ids = []string{trackIDForPlaylist(track)}
			}
			pl, err := prov.CreatePlaylist(ctx, name, ids)
			if pl.Name == "" {
				pl.Name = name
			}
			return playlistEditedMsg{what: "created", pl: pl, track: track, err: err}
		}
	})
}

func (m *Model) promptRenamePlaylist(pl provider.Playlist) tea.Cmd {
	ed, ok := m.playlistEditor()
	if !ok {
		m.note(i18n.T("playlists can't be renamed here"))
		return nil
	}
	m.openPrompt(i18n.T("Rename playlist"), pl.Name, func(name string) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			err := ed.RenamePlaylist(ctx, pl.ID, name)
			renamed := pl
			renamed.Name = name
			return playlistEditedMsg{what: "renamed", pl: renamed, err: err}
		}
	})
	return nil
}

func (m *Model) deletePlaylistCmd(pl provider.Playlist) tea.Cmd {
	if !m.canDelete() {
		m.refuse("Deleting")
		return nil
	}
	ed, ok := m.playlistEditor()
	if !ok {
		m.note(i18n.T("playlists can't be deleted here"))
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return playlistEditedMsg{what: "deleted", pl: pl, err: ed.DeletePlaylist(ctx, pl.ID)}
	}
}

func (m *Model) removeFromPlaylistCmd(pl provider.Playlist, idx int, t provider.Track) tea.Cmd {
	if !m.canDelete() {
		m.refuse("Deleting")
		return nil
	}
	ed, ok := m.playlistEditor()
	if !ok {
		m.note(i18n.T("songs can't be removed from playlists here"))
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := ed.RemoveFromPlaylist(ctx, pl.ID, idx, t)
		return playlistEditedMsg{what: "removed", pl: pl, track: t, idx: idx, err: err}
	}
}

func (m *Model) addToLibraryCmd(t provider.Track) tea.Cmd {
	la, ok := m.provider.(provider.LibraryAdder)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return playlistEditedMsg{what: "library", track: t, err: la.AddToLibrary(ctx, t)}
	}
}

// canAddToLibrary reports whether the song menu offers "Add to library"
// for t: the service has a library apart from the favourites and t isn't
// a library song already.
func (m *Model) canAddToLibrary(t provider.Track) bool {
	_, ok := m.provider.(provider.LibraryAdder)
	return ok && !strings.HasPrefix(t.ID, "i.")
}

// confirmTwice reports whether key was pressed a moment ago for the same
// thing (what); otherwise it asks for it again.
func (m *Model) confirmTwice(what, ask string) bool {
	if m.confirmWhat == what && time.Since(m.confirmAt) <= favRemoveWindow {
		m.confirmWhat = ""
		m.errMsg = ""
		return true
	}
	m.confirmWhat, m.confirmAt = what, time.Now()
	m.errMsg = ask
	m.errExpiry = time.Now().Add(favRemoveWindow)
	return false
}

// libraryPlaylistKey handles the playlist keys of the library: in the list
// of playlists c (new), r (rename), d d (delete); in an open playlist d d
// (take the song out). ok is false for other keys and places.
func (m *Model) libraryPlaylistKey(k string) (tea.Cmd, bool) {
	lib := m.library.m
	if m.serviceKey == "radio" && k == "r" {
		track := lib.SelectedTrack()
		pl, _, _ := lib.OpenPlaylist()
		if track != nil && (lib.InAllSongs() || pl != nil && provider.IsFavoritesPlaylist(*pl)) {
			return m.promptRenameStation(*track), true
		}
	}
	if (k == "d" || k == "delete") && !m.canDelete() &&
		(lib.SelectedSavedAlbum() != nil || lib.SelectedSavedArtist() != nil || lib.SelectedPlaylist() != nil) {
		m.refuse("Deleting")
		return nil, true
	}
	if k == "d" || k == "delete" {
		if a := lib.SelectedSavedAlbum(); a != nil {
			if !m.confirmTwice("unsave:"+a.ID, fmt.Sprintf(i18n.T("press d again to remove \"%s\" from the library"), a.Title)) {
				return nil, true
			}
			return m.saveAlbumCmd(*a, false), true
		}
		if a := lib.SelectedSavedArtist(); a != nil {
			if !m.confirmTwice("unfollow:"+a.ID, fmt.Sprintf(i18n.T("press d again to unfollow \"%s\""), a.Name)) {
				return nil, true
			}
			return m.followArtistCmd(*a, false), true
		}
	}
	if lib.ShowingPlaylists() {
		pl := lib.SelectedPlaylist()
		switch k {
		case "c":
			m.promptNewPlaylist(nil)
			return nil, true
		case "r":
			if pl == nil {
				return nil, true
			}
			if !m.canEdit() {
				m.refuse("Editing")
				return nil, true
			}
			if pl.ReadOnly {
				m.note(i18n.T("this playlist can't be renamed"))
				return nil, true
			}
			return m.promptRenamePlaylist(*pl), true
		case "d", "delete":
			if pl == nil {
				return nil, true
			}
			if pl.ReadOnly {
				m.note(i18n.T("this playlist can't be deleted"))
				return nil, true
			}
			if !m.confirmTwice("delete:"+pl.ID, fmt.Sprintf(i18n.T("press d again to delete \"%s\""), pl.Name)) {
				return nil, true
			}
			return m.deletePlaylistCmd(*pl), true
		}
		return nil, false
	}
	if pl, t, idx := lib.OpenPlaylist(); pl != nil && t != nil {
		switch k {
		case "d", "delete":
			if !m.canDelete() {
				m.refuse("Deleting")
				return nil, true
			}
			if !m.confirmTwice(fmt.Sprintf("remove:%s:%d:%s", pl.ID, idx, t.ID),
				fmt.Sprintf(i18n.T("press d again to remove it from \"%s\""), pl.Name)) {
				return nil, true
			}
			return m.removeFromPlaylistCmd(*pl, idx, *t), true
		}
	}
	return nil, false
}

// addToPlaylistTarget opens the playlist picker for the song the song menu
// would be about (the selected one, else the playing one).
func (m *Model) addToPlaylistTarget() tea.Cmd {
	t := m.songMenuTarget()
	if t == nil {
		m.note(i18n.T("nothing to add — play or select a song"))
		return nil
	}
	return m.openPlaylistPicker(t)
}

// libraryHints are the status line's keys while the library is open.
func (m *Model) libraryHints() []string {
	muted, accent := styles.QueueItemMuted, styles.KeyName
	key := func(k, what string) string { return accent.Render(i18n.T(k)) + muted.Render(i18n.T(what)) }
	lib := m.library.m
	parts := []string{styles.ModeNormal.Render(i18n.T("LIBRARY")), key("Enter", " browse/play")}
	_, editable := m.playlistEditor()
	_, saver := m.provider.(provider.CollectionSaver)
	switch {
	case saver && (lib.SelectedSavedAlbum() != nil || lib.SelectedSavedArtist() != nil):
		parts = append(parts, key("Tab", " play next"), key("Shift+Tab", " play last"))
		if m.canDelete() {
			parts = append(parts, key("d d", " remove"))
		}
	case lib.ShowingPlaylists():
		parts = append(parts, key("Tab", " play next"), key("Shift+Tab", " play last"), key("c", " new playlist"))
		if editable && m.canEdit() {
			parts = append(parts, key("r", " rename"))
		}
		if editable && m.canDelete() {
			parts = append(parts, key("d d", " delete"))
		}
	case lib.SelectedTrack() != nil:
		if m.serviceKey == "radio" {
			pl, _, _ := lib.OpenPlaylist()
			if lib.InAllSongs() || pl != nil && provider.IsFavoritesPlaylist(*pl) {
				parts = append(parts, key("r", " rename station"))
			}
		}
		parts = append(parts, key("Tab", " play next"), key("Shift+Tab", " play last"), key("Ctrl+Q", " all next"), key("P", " add to playlist"))
		if pl, _, _ := lib.OpenPlaylist(); pl != nil && editable && m.canDelete() {
			parts = append(parts, key("d d", " remove"))
		}
		parts = append(parts, key("i", " song menu"))
	default:
		parts = append(parts, key("Tab", " play next"), key("Shift+Tab", " play last"))
	}
	if m.serviceKey == "local" {
		parts = append(parts, key("Ctrl+R", " refresh"))
	}
	return append(parts, key("esc", " back/close"))
}

// saveAlbumCmd saves the album to the library, or takes it out.
func (m *Model) saveAlbumCmd(a provider.Album, save bool) tea.Cmd {
	if !save && !m.canDelete() {
		m.refuse("Deleting")
		return nil
	}
	cs, ok := m.provider.(provider.CollectionSaver)
	if !ok {
		return nil
	}
	what := "album+"
	if !save {
		what = "album-"
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return playlistEditedMsg{what: what, id: a.ID, name: a.Title, err: cs.SaveAlbum(ctx, a, save)}
	}
}

// followArtistCmd follows the artist, or stops.
func (m *Model) followArtistCmd(a provider.Artist, follow bool) tea.Cmd {
	if !follow && !m.canDelete() {
		m.refuse("Deleting")
		return nil
	}
	cs, ok := m.provider.(provider.CollectionSaver)
	if !ok {
		return nil
	}
	what := "artist+"
	if !follow {
		what = "artist-"
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return playlistEditedMsg{what: what, id: a.ID, name: a.Name, err: cs.FollowArtist(ctx, a, follow)}
	}
}

// canDelete and canEdit are the setup's switches for deleting from and
// changing the library (utuimp setup → Library); both are off by default.
// Deleting covers taking likes back too: songs, albums, artists.
func (m *Model) canDelete() bool { return m.cfg != nil && m.cfg.LibraryDelete }
func (m *Model) canEdit() bool   { return m.cfg != nil && m.cfg.LibraryEdit }

// refuse says that what was asked is switched off, and where to turn it on.
func (m *Model) refuse(what string) {
	m.note(fmt.Sprintf(i18n.T("%s is off — turn it on in utuimp setup (Library)"), i18n.T(what)))
}

type stationRenamedMsg struct {
	id, name string
	err      error
}

func (m *Model) promptRenameStation(t provider.Track) tea.Cmd {
	renamer, ok := m.provider.(interface {
		RenameStation(context.Context, string, string) error
	})
	if !ok {
		m.note(i18n.T("Station cannot be renamed"))
		return nil
	}
	m.openPrompt(i18n.T("Rename station"), t.Title, func(name string) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return stationRenamedMsg{id: t.ID, name: name, err: renamer.RenameStation(ctx, t.ID, name)}
		}
	})
	return nil
}
