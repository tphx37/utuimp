package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/i18n"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/styles"
	"github.com/tphx37/utuimp/internal/tui/views"
)

// The song menu (key i) is a small window over the UI with what can be
// done with one song: open its album, open each of its artists, copy its
// link.

type songMenuKind int

const (
	songMenuAlbum songMenuKind = iota
	songMenuArtist
	songMenuCopyLink
	songMenuAddToPlaylist
	songMenuAddToLibrary
	songMenuRemoveFromPlaylist
	songMenuSaveAlbum
	songMenuFollowArtist
)

type songMenuItem struct {
	kind   songMenuKind
	label  string
	artist provider.Artist
}

type songMenuState struct {
	track   provider.Track
	loading bool
	err     error
	details provider.SongDetails
	items   []songMenuItem
	idx     int

	// own are the items that need no lookup (playlists, library); they
	// follow the looked-up ones. inPlaylist is the open library playlist
	// the song was picked in, at inPlaylistIdx (nil: none).
	own           []songMenuItem
	inPlaylist    *provider.Playlist
	inPlaylistIdx int
}

type songDetailsMsg struct {
	key     string
	details provider.SongDetails
	err     error
}

type songMenuAlbumMsg struct {
	title  string
	tracks []provider.Track
	err    error
}

// songMenuTarget is the song the menu is about: the one highlighted in the
// open list (history, library tracks, main-screen queue), else the playing one.
func (m *Model) songMenuTarget() *provider.Track {
	if m.activePanel >= 0 {
		switch m.panels[m.activePanel] {
		case m.historyP:
			if _, t := m.historyP.m.SelectedTrack(); t != nil {
				return t
			}
		case m.library:
			if t := m.library.m.SelectedTrack(); t != nil {
				return t
			}
		}
	}
	if m.activePanel < 0 && m.queueMiniCursor >= 0 && m.queueMiniCursor < len(m.queueTracks) {
		t := m.queueTracks[m.queueMiniCursor]
		return &t
	}
	if m.playerState.Track != nil {
		t := *m.playerState.Track
		return &t
	}
	return nil
}

func songKey(t provider.Track) string { return views.PlaybackID(t) + "\x00" + t.Title }

// openSongMenu opens the menu for the target song and starts looking up its
// album, artists and link.
func (m *Model) openSongMenu() tea.Cmd {
	t := m.songMenuTarget()
	if t == nil {
		m.errMsg = i18n.T("nothing to show — play or select a song")
		m.errExpiry = time.Now().Add(3 * time.Second)
		return nil
	}
	m.songMenu = &songMenuState{track: *t, loading: true}
	m.songMenuOwnItems()
	sd, ok := m.provider.(provider.SongDetailer)
	if !ok {
		m.songMenu.loading = false
		if len(m.songMenu.items) == 0 {
			m.songMenu.err = errors.New(i18n.T("not available for this source"))
		}
		return nil
	}
	track, key := *t, songKey(*t)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d, err := sd.GetSongDetails(ctx, track)
		return songDetailsMsg{key: key, details: d, err: err}
	}
}

func (m *Model) handleSongDetails(msg songDetailsMsg) {
	sm := m.songMenu
	if sm == nil || songKey(sm.track) != msg.key {
		return
	}
	sm.loading = false
	sm.err = msg.err
	sm.details = msg.details
	sm.items = nil
	if a := msg.details.Album; a != nil {
		sm.items = append(sm.items, songMenuItem{kind: songMenuAlbum, label: i18n.T("Album · ") + a.Title})
	}
	for _, ar := range msg.details.Artists {
		sm.items = append(sm.items, songMenuItem{kind: songMenuArtist, label: i18n.T("Artist · ") + ar.Name, artist: ar})
	}
	if msg.details.URL != "" {
		sm.items = append(sm.items, songMenuItem{kind: songMenuCopyLink, label: i18n.T("Copy link")})
	}
	// Saving the album and following the main artist, where the service
	// keeps them in the library.
	if _, ok := m.provider.(provider.CollectionSaver); ok {
		if a := msg.details.Album; a != nil && !msg.details.LibraryAlbum {
			sm.items = append(sm.items, songMenuItem{kind: songMenuSaveAlbum, label: i18n.T("Save album to library")})
		}
		if len(msg.details.Artists) > 0 {
			ar := msg.details.Artists[0]
			sm.items = append(sm.items, songMenuItem{kind: songMenuFollowArtist,
				label: fmt.Sprintf(i18n.T("Follow %s"), ar.Name), artist: ar})
		}
	}
	sm.items = append(sm.items, sm.own...)
	sm.idx = 0
}

// songMenuOwnItems adds what the menu offers without a lookup: adding the
// song to a playlist, to the library, and taking it out of the playlist it
// was picked in.
func (m *Model) songMenuOwnItems() {
	sm := m.songMenu
	sm.own = append(sm.own, songMenuItem{kind: songMenuAddToPlaylist, label: i18n.T("Add to playlist…")})
	if m.canAddToLibrary(sm.track) {
		sm.own = append(sm.own, songMenuItem{kind: songMenuAddToLibrary, label: i18n.T("Add to library")})
	}
	if m.activePanel >= 0 && m.panels[m.activePanel] == m.library {
		if pl, t, idx := m.library.m.OpenPlaylist(); pl != nil && t != nil && t.ID == sm.track.ID && m.canDelete() {
			if _, ok := m.playlistEditor(); ok {
				sm.inPlaylist, sm.inPlaylistIdx = pl, idx
				sm.own = append(sm.own, songMenuItem{kind: songMenuRemoveFromPlaylist,
					label: fmt.Sprintf(i18n.T("Remove from “%s”"), pl.Name)})
			}
		}
	}
	sm.items = append(sm.items, sm.own...)
}

func (m *Model) handleSongMenuKey(k string) tea.Cmd {
	sm := m.songMenu
	switch k {
	case "esc", "i", "q":
		m.songMenu = nil
		return nil
	case "j", "down", "tab":
		if len(sm.items) > 0 {
			sm.idx = (sm.idx + 1) % len(sm.items)
		}
	case "k", "up", "shift+tab":
		if len(sm.items) > 0 {
			sm.idx = (sm.idx + len(sm.items) - 1) % len(sm.items)
		}
	case "enter":
		if sm.idx < len(sm.items) {
			return m.runSongMenuItem(sm.items[sm.idx])
		}
	default:
		if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			if i := int(k[0] - '1'); i < len(sm.items) {
				return m.runSongMenuItem(sm.items[i])
			}
		}
	}
	return nil
}

func (m *Model) runSongMenuItem(it songMenuItem) tea.Cmd {
	sm := m.songMenu
	m.songMenu = nil
	switch it.kind {
	case songMenuAlbum:
		album := *sm.details.Album
		library := sm.details.LibraryAlbum
		prov := m.provider
		m.setBusy("loading album “" + album.Title + "”…")
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var tracks []provider.Track
			var err error
			if library {
				tracks, err = prov.GetLibraryAlbumTracks(ctx, album.ID)
			} else {
				tracks, err = prov.GetAlbumTracks(ctx, album.ID)
			}
			return songMenuAlbumMsg{title: album.Title, tracks: tracks, err: err}
		}
	case songMenuArtist:
		m.activePanel = -1
		m.mode = modeSearch
		return m.openArtistCmd(it.artist)
	case songMenuSaveAlbum:
		return m.saveAlbumCmd(*sm.details.Album, true)
	case songMenuFollowArtist:
		return m.followArtistCmd(it.artist, true)
	case songMenuAddToPlaylist:
		t := sm.track
		return m.openPlaylistPicker(&t)
	case songMenuAddToLibrary:
		return m.addToLibraryCmd(sm.track)
	case songMenuRemoveFromPlaylist:
		if sm.inPlaylist == nil {
			return nil
		}
		return m.removeFromPlaylistCmd(*sm.inPlaylist, sm.inPlaylistIdx, sm.track)
	case songMenuCopyLink:
		url := sm.details.URL
		m.errMsg = i18n.T("✓ link copied")
		m.errExpiry = time.Now().Add(3 * time.Second)
		m.appendLog("[song] link: " + url)
		// OSC 52 reaches the terminal's clipboard (also over SSH); the
		// desktop tools cover terminals that don't allow it.
		return tea.Batch(tea.SetClipboard(url), copyWithDesktopTool(url))
	}
	return nil
}

func (m *Model) handleSongMenuAlbum(msg songMenuAlbumMsg) {
	m.busyLabel = ""
	if msg.err != nil || len(msg.tracks) == 0 {
		m.errMsg = i18n.T("could not load the album")
		if msg.err != nil {
			m.errMsg += ": " + msg.err.Error()
		}
		m.errExpiry = time.Now().Add(4 * time.Second)
		return
	}
	m.library.m.ShowTrackList(msg.title, msg.tracks)
	for i, p := range m.panels {
		if p == m.library {
			m.activePanel = i
		}
	}
	m.mode = modeNormal
}

// copyWithDesktopTool copies s with wl-copy / xclip / xsel / pbcopy when one
// is installed. Best-effort.
func copyWithDesktopTool(s string) tea.Cmd {
	return func() tea.Msg {
		for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}, {"pbcopy"}} {
			if _, err := exec.LookPath(c[0]); err != nil {
				continue
			}
			cmd := exec.Command(c[0], c[1:]...) //nolint:gosec // fixed tool names
			cmd.Stdin = strings.NewReader(s)
			if cmd.Run() == nil {
				break
			}
		}
		return nil
	}
}

// renderSongMenu draws the menu window.
func (m *Model) renderSongMenu() string {
	sm := m.songMenu
	innerW := min(52, max(20, m.width-10))
	muted := styles.QueueItemMuted
	sep := muted.Render(strings.Repeat("─", innerW))

	lines := []string{
		styles.NowPlayingTitlePlaying.Render(truncateStr(sm.track.Title, innerW)),
		muted.Render(truncateStr(sm.track.Artist, innerW)),
		sep,
	}
	if sm.loading {
		frames := [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		lines = append(lines, styles.Spinner.Render(frames[m.glowStep%len(frames)])+muted.Render(i18n.T(" looking up album and artists…")))
	}
	switch {
	case sm.err != nil && len(sm.items) == 0:
		lines = append(lines, styles.ErrorStyle.Render(truncateStr(sm.err.Error(), innerW-2)))
	case len(sm.items) == 0 && !sm.loading:
		lines = append(lines, muted.Render(i18n.T("nothing to show for this song")))
	default:
		for i, it := range sm.items {
			num := styles.KeyName.Render(fmt.Sprintf(i18n.T("%d "), i+1))
			label := truncateStr(it.label, innerW-4)
			if i == sm.idx {
				lines = append(lines, styles.Playing.Render("▸ ")+num+styles.Selected.Render(label))
			} else {
				lines = append(lines, "  "+num+styles.QueueItem.Render(label))
			}
		}
	}
	lines = append(lines, sep, muted.Render(i18n.T("↑↓ choose · enter open · esc close")))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorBorderFocus).
		Padding(0, 1).
		Width(innerW + 4).
		Render(strings.Join(lines, "\n"))
}

// overlay draws box centred over base.
func (m *Model) overlay(base, box string) string {
	x := max(0, (m.width-lipgloss.Width(box))/2)
	y := max(0, (m.height-lipgloss.Height(box))/2)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}

// songMenuBackdrop covers the pixel pictures underneath the menu. Terminal
// cells alone cannot obscure graphics placed above the terminal background.
func (m *Model) songMenuBackdrop() string {
	l := m.lines
	if m.songMenu == nil {
		if l.menuShown {
			l.menuShown = false
			return l.menuImg.Delete()
		}
		return ""
	}
	box := m.renderSongMenu()
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	x, y := max(0, (m.width-w)/2), max(0, (m.height-h)/2)
	if w <= 2 || h <= 2 {
		return ""
	}
	_, bg, _ := styles.ScreenColors()
	key := [7]int{x + 1, y + 1 + m.hostRows, w - 2, h - 2, int(bg.R), int(bg.G), int(bg.B)}
	if l.menuShown && key == l.menuKey {
		return ""
	}
	pw, ph := key[2]*l.cellW, key[3]*l.cellH
	if pw <= 0 || ph <= 0 {
		return ""
	}
	pix := make([]byte, pw*ph*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = bg.R, bg.G, bg.B, 255
	}
	seq, err := l.menuImg.Frame(l.mode, pix, pw, ph, key[0], key[1], key[2], key[3])
	if err != nil || seq == "" {
		return ""
	}
	l.menuShown, l.menuKey = true, key
	return seq
}
