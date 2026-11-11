package views

import (
	"cmp"
	"context"
	"fmt"
	"github.com/tphx37/utuimp/internal/i18n"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

type libraryPane int

const (
	paneSections libraryPane = iota
	paneItems
	paneTracks
)

const (
	libraryLoadTimeout = 60 * time.Second
	libraryTracksTTL   = 5 * time.Minute
)

type librarySection int

const (
	sectionSongs librarySection = iota
	sectionAlbums
	sectionArtists
	sectionPlaylists
)

type libraryRequestKind int

const (
	libraryRequestNone libraryRequestKind = iota
	libraryRequestTracks
	libraryRequestPlaylists
)

type libraryLoadedMsg struct {
	generation uint64
	section    librarySection
	kind       libraryRequestKind
	playlists  []provider.Playlist
	err        error
}

type libraryTracksLoadedMsg struct {
	generation uint64
	section    librarySection
	kind       libraryRequestKind
	tracks     []provider.Track
	err        error
}

type playlistRequestKind int

const (
	playlistRequestNone playlistRequestKind = iota
	playlistRequestTracks
)

type playlistTracksMsg struct {
	playlist   provider.Playlist
	generation uint64
	kind       playlistRequestKind
	tracks     []provider.Track
	err        error
}

type trackGroup struct {
	title  string
	desc   string
	tracks []provider.Track
}

type LibraryModel struct {
	provider provider.Provider
	loading  bool
	loadErr  error
	list     list.Model
	spinner  spinner.Model

	pane              libraryPane
	selectedSection   librarySection
	libraryTracks     []provider.Track
	tracksLoaded      bool
	libraryTracksTime time.Time
	playlists         []provider.Playlist
	playlistsLoaded   bool
	albums            []trackGroup
	artists           []trackGroup

	libraryRequestGeneration uint64
	libraryRequestSection    librarySection
	libraryRequestKind       libraryRequestKind

	drillTitle             string
	drillPlaylist          provider.Playlist
	drillTracks            []provider.Track
	drillLoading           bool
	drillErr               error
	drillList              list.Model
	drillRequestGeneration uint64
	drillRequestKind       playlistRequestKind
	tracksBackPane         libraryPane

	// keepPlaylistID is highlighted when the playlists come back loaded.
	keepPlaylistID string

	// The service's saved albums and followed artists (library_saved.go):
	// openArtist is the artist whose albums are listed (nil: none), from
	// artistIdx of the artists; drillAlbumID the album being loaded.
	savedAlbums  []provider.Album
	savedArtists []provider.Artist
	openArtist   *provider.Artist
	artistIdx    int
	drillAlbumID string

	width  int
	height int
}

func NewLibrary(prov provider.Provider) *LibraryModel {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(styles.ColorPrimary)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(styles.ColorSubtle)
	l := list.New(nil, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetFilteringEnabled(false)
	drill := list.New(nil, delegate, 0, 0)
	drill.SetShowTitle(false)
	drill.SetFilteringEnabled(false)
	sp := spinner.New()
	sp.Style = styles.Spinner
	sp.Spinner = spinner.Dot
	m := &LibraryModel{provider: prov, list: l, drillList: drill, spinner: sp, pane: paneSections}
	m.showSections()
	return m
}

func (m *LibraryModel) Init() tea.Cmd   { return m.spinner.Tick }
func (m *LibraryModel) Width() int      { return m.width }
func (m *LibraryModel) Height() int     { return m.height }
func (m *LibraryModel) DrillErr() error { return m.drillErr }
func (m *LibraryModel) LoadErr() error  { return m.loadErr }

func (m *LibraryModel) ResetTopLevel() {
	m.openArtist = nil
	m.invalidateLibraryRequest()
	m.invalidatePlaylistRequest()
	m.pane = paneSections
	m.showSections()
}

// InvalidateTracksCache marks the cached library tracks as stale so the next
// Songs or Favorites view reloads fresh data from the provider.
func (m *LibraryModel) InvalidateTracksCache() {
	m.tracksLoaded = false
	m.libraryTracksTime = time.Time{}
}

func (m *LibraryModel) Back() bool {
	switch m.pane {
	case paneSections:
		return false
	case paneItems:
		if m.closeSavedArtist() {
			return true
		}
		m.invalidateLibraryRequest()
		m.pane = paneSections
		m.showSections()
		return true
	case paneTracks:
		m.invalidatePlaylistRequest()
		m.drillAlbumID = ""
		if m.tracksBackPane == paneSections && m.drillTitle == "" {
			m.pane = paneItems
		} else {
			m.pane = m.tracksBackPane
		}
		if m.pane == paneSections {
			m.showSections()
		}
		return true
	default:
		return false
	}
}

func (m *LibraryModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	listH := max(0, h-3)
	m.list.SetSize(w, listH)
	m.drillList.SetSize(w, listH)
}

func (m *LibraryModel) loadLibraryTracks() tea.Cmd {
	m.libraryRequestGeneration++
	m.libraryRequestSection = m.selectedSection
	m.libraryRequestKind = libraryRequestTracks
	generation := m.libraryRequestGeneration
	section := m.libraryRequestSection
	kind := m.libraryRequestKind
	m.loading = true
	prov := m.provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), libraryLoadTimeout)
		defer cancel()
		tracks, err := prov.GetLibraryTracks(ctx)
		return libraryTracksLoadedMsg{generation: generation, section: section, kind: kind, tracks: tracks, err: err}
	}
}

func (m *LibraryModel) loadPlaylists() tea.Cmd {
	m.libraryRequestGeneration++
	m.libraryRequestSection = m.selectedSection
	m.libraryRequestKind = libraryRequestPlaylists
	generation := m.libraryRequestGeneration
	section := m.libraryRequestSection
	kind := m.libraryRequestKind
	m.loading = true
	prov := m.provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), libraryLoadTimeout)
		defer cancel()
		playlists, err := prov.GetLibraryPlaylists(ctx)
		return libraryLoadedMsg{generation: generation, section: section, kind: kind, playlists: playlists, err: err}
	}
}

func (m *LibraryModel) loadPlaylistTracks(pl provider.Playlist) tea.Cmd {
	m.drillRequestGeneration++
	m.drillRequestKind = playlistRequestTracks
	generation := m.drillRequestGeneration
	kind := m.drillRequestKind

	m.drillLoading = true
	prov := m.provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), libraryLoadTimeout)
		defer cancel()
		tracks, err := prov.GetPlaylistTracks(ctx, pl.ID)
		return playlistTracksMsg{playlist: pl, generation: generation, kind: kind, tracks: tracks, err: err}

	}
}

func (m *LibraryModel) Update(msg tea.Msg) (*LibraryModel, tea.Cmd) {
	if cmd, ok := m.updateSaved(msg); ok {
		return m, cmd
	}
	switch msg := msg.(type) {
	case libraryTracksLoadedMsg:
		if !m.currentLibraryRequest(msg.generation, msg.section, msg.kind) || msg.kind != libraryRequestTracks {
			return m, nil
		}
		m.loading = false
		m.loadErr = msg.err
		if msg.err == nil {
			m.libraryTracks = append([]provider.Track{}, msg.tracks...)
			m.tracksLoaded = true
			m.libraryTracksTime = time.Now()
			m.routeLoadedLibraryTracks()
		} else {
			m.pane = paneItems
			m.list.SetItems(nil)
		}
		return m, nil
	case libraryLoadedMsg:
		if !m.currentLibraryRequest(msg.generation, msg.section, msg.kind) || msg.kind != libraryRequestPlaylists {
			return m, nil
		}
		m.loading = false
		m.loadErr = msg.err
		if msg.err == nil {
			m.playlists = append([]provider.Playlist{}, msg.playlists...)
			m.playlistsLoaded = true
			m.showPlaylists()
			if id := m.keepPlaylistID; id != "" {
				m.keepPlaylistID = ""
				if i := slices.IndexFunc(m.playlists, func(p provider.Playlist) bool { return p.ID == id }); i >= 0 {
					m.list.Select(i)
				}
			}
		} else {
			m.pane = paneItems
			m.list.SetItems(nil)
		}
		return m, nil
	case playlistTracksMsg:
		if m.pane != paneTracks || m.drillPlaylist.ID != msg.playlist.ID || m.drillRequestGeneration != msg.generation || m.drillRequestKind != msg.kind || msg.kind != playlistRequestTracks {

			return m, nil
		}
		m.drillLoading = false
		m.drillErr = msg.err
		if msg.err == nil {
			m.showPlaylistTrackPane(msg.playlist.Name, msg.tracks, paneItems)
		}
		return m, nil
	case spinner.TickMsg:
		if m.loading || m.drillLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m.updateActiveList(msg)
}

func (m *LibraryModel) handleKey(msg tea.KeyPressMsg) (*LibraryModel, tea.Cmd) {
	switch m.pane {
	case paneSections:
		if msg.String() == "tab" {
			return m, nil
		}
		if msg.String() == "enter" {
			if item, ok := m.list.SelectedItem().(sectionItem); ok {
				m.selectedSection = item.section
				return m.openSelectedSection()

			}
			return m, nil
		}
	case paneItems:
		switch msg.String() {
		case "esc", "backspace", "b":
			if m.closeSavedArtist() {
				return m, nil
			}
			m.invalidatePlaylistRequest()
			m.loadErr = nil
			m.pane = paneSections
			m.showSections()
			return m, nil
		case "enter":
			return m.openSelectedItem()
		case "tab", "ctrl+q":
			return m.queueSelectedItem(true)
		case "shift+tab":
			return m.queueSelectedItem(false)
		}
	case paneTracks:
		switch msg.String() {
		case "esc", "backspace", "b":
			m.Back()
			return m, nil
		case "enter":
			return m, m.playTracksFromSelection()
		case "tab":
			return m, m.queueSelectedTrack(true)
		case "shift+tab":
			return m, m.queueSelectedTrack(false)
		case "ctrl+q":
			return m, m.queueWholeListNext()
		}
	}
	return m.updateActiveList(msg)
}

func (m *LibraryModel) updateActiveList(msg tea.Msg) (*LibraryModel, tea.Cmd) {
	var cmd tea.Cmd
	if m.pane == paneTracks {
		m.drillList, cmd = m.drillList.Update(msg)
		return m, cmd
	}
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *LibraryModel) openSelectedSection() (*LibraryModel, tea.Cmd) {
	m.invalidateLibraryRequest()
	if m.usesSaved(m.selectedSection) {
		return m.openSaved()
	}
	switch m.selectedSection {
	case sectionSongs, sectionAlbums, sectionArtists:
		if !m.tracksLoaded || m.libraryTracksExpired(time.Now()) {
			m.loading = true
			return m, tea.Batch(m.spinner.Tick, m.loadLibraryTracks())
		}
		m.routeLoadedLibraryTracks()
		return m, nil
	case sectionPlaylists:
		if !m.playlistsLoaded {
			m.loading = true
			return m, tea.Batch(m.spinner.Tick, m.loadPlaylists())
		}
		m.showPlaylists()
		return m, nil
	default:
		m.loadErr = fmt.Errorf("unknown library section: %d", m.selectedSection)
		m.pane = paneSections
		m.showSections()
		return m, nil
	}
}

func (m *LibraryModel) currentLibraryRequest(generation uint64, section librarySection, kind libraryRequestKind) bool {
	return m.libraryRequestGeneration == generation && m.libraryRequestSection == section && m.libraryRequestKind == kind && m.selectedSection == section
}

func (m *LibraryModel) invalidateLibraryRequest() {
	m.libraryRequestGeneration++
	m.libraryRequestSection = m.selectedSection
	m.libraryRequestKind = libraryRequestNone
	m.loading = false
}

func (m *LibraryModel) libraryTracksExpired(now time.Time) bool {
	if !m.tracksLoaded {
		return true
	}
	if m.libraryTracksTime.IsZero() {
		return true
	}
	return now.Sub(m.libraryTracksTime) >= libraryTracksTTL
}

func (m *LibraryModel) routeLoadedLibraryTracks() {
	switch m.selectedSection {
	case sectionSongs:
		m.showTrackPane("Songs", m.libraryTracks, paneSections)
	case sectionAlbums:
		m.albums = buildLibraryAlbums(m.libraryTracks)
		m.showGroups(m.albums)
	case sectionArtists:
		m.artists = buildLibraryArtists(m.libraryTracks)
		m.showGroups(m.artists)
	case sectionPlaylists:
		m.showPlaylists()
	default:
		m.loadErr = fmt.Errorf("unknown library section: %d", m.selectedSection)
		m.pane = paneSections
		m.showSections()
	}
}

func (m *LibraryModel) openSelectedItem() (*LibraryModel, tea.Cmd) {
	switch item := m.list.SelectedItem().(type) {
	case savedAlbumItem:
		return m, m.openSavedAlbum(item.a)
	case savedArtistItem:
		return m, m.openSavedArtist(item.a)
	case albumItem:
		m.showTrackPane(item.group.title, item.group.tracks, paneItems)
	case artistItem:
		m.showTrackPane(item.group.title, item.group.tracks, paneItems)
	case playlistItem:
		pl := provider.Playlist(item)
		m.drillPlaylist = pl
		m.drillTitle = pl.Name
		m.pane = paneTracks
		m.tracksBackPane = paneItems
		m.drillTracks = nil
		m.drillErr = nil
		m.drillList.SetItems(nil)
		return m, tea.Batch(m.spinner.Tick, m.loadPlaylistTracks(pl))
	}
	return m, nil
}

func (m *LibraryModel) queueSelectedItem(playNext bool) (*LibraryModel, tea.Cmd) {
	switch item := m.list.SelectedItem().(type) {
	case savedAlbumItem:
		return m, queueAlbumCmd(m.provider, item.a, playNext)
	case albumItem:
		return m, queueTracksCmd(item.group.title, item.group.tracks, playNext)
	case artistItem:
		return m, queueTracksCmd(item.group.title, item.group.tracks, playNext)
	case playlistItem:
		return m, queuePlaylistCmd(m.provider, provider.Playlist(item), playNext)
	default:
		return m, nil
	}
}

// queuePlaylistCmd loads a playlist's songs and queues them (after the
// playing song with playNext), without leaving the list.
func queuePlaylistCmd(prov provider.Provider, pl provider.Playlist, playNext bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), libraryLoadTimeout)
		defer cancel()
		tracks, err := prov.GetPlaylistTracks(ctx, pl.ID)
		if err != nil {
			return QueueTracksMsg{Label: pl.Name, Err: err}
		}
		ids := make([]string, len(tracks))
		for i, t := range tracks {
			ids[i] = PlaybackID(t)
		}
		return QueueTracksMsg{IDs: ids, Tracks: tracks, Label: pl.Name, PlayNext: playNext}
	}
}

func (m *LibraryModel) playTracksFromSelection() tea.Cmd {
	if selected := m.drillList.SelectedItem(); selected == nil {
		return nil
	}
	idx := m.drillList.Index()
	if idx < 0 || idx >= len(m.drillTracks) {
		return nil
	}
	// Queue the whole list and start at the selection, so the songs before
	// it stay in the queue (Previous reaches them, the list keeps its order).
	tracks := append([]provider.Track{}, m.drillTracks...)
	ids := make([]string, len(tracks))
	for i, t := range tracks {
		ids[i] = PlaybackID(t)
	}
	first := tracks[idx]
	return func() tea.Msg { return PlayTracksMsg{IDs: ids, Tracks: tracks, Track: &first, StartIdx: idx} }
}

// InAllSongs reports whether the track pane shows the library's list of all
// songs rather than one album or playlist.
func (m *LibraryModel) InAllSongs() bool {
	return m.tracksBackPane == paneSections && m.drillTitle == "Songs"
}

// queueSelectedTrack puts the highlighted song straight after the playing
// one (playNext) or at the end of the queue.
func (m *LibraryModel) queueSelectedTrack(playNext bool) tea.Cmd {
	if selected := m.drillList.SelectedItem(); selected == nil {
		return nil
	}
	idx := m.drillList.Index()
	if idx < 0 || idx >= len(m.drillTracks) {
		return nil
	}
	return queueTracksCmd(m.drillTracks[idx].Title, m.drillTracks[idx:idx+1], playNext)
}

// queueWholeListNext puts the whole album or playlist after the playing
// song. In the list of all the library's songs it is the highlighted song
// alone.
func (m *LibraryModel) queueWholeListNext() tea.Cmd {
	if len(m.drillTracks) == 0 {
		return nil
	}
	if m.InAllSongs() {
		idx := m.drillList.Index()
		if idx < 0 || idx >= len(m.drillTracks) {
			return nil
		}
		return queueTracksCmd(m.drillTracks[idx].Title, m.drillTracks[idx:idx+1], true)
	}
	return queueTracksCmd(m.drillTitle, m.drillTracks, true)
}

func queueTracksCmd(label string, tracks []provider.Track, playNext bool) tea.Cmd {
	if len(tracks) == 0 {
		return nil
	}
	queued := append([]provider.Track{}, tracks...)
	ids := make([]string, len(queued))
	for i, track := range queued {
		ids[i] = PlaybackID(track)
	}
	return func() tea.Msg { return QueueTracksMsg{IDs: ids, Tracks: queued, Label: label, PlayNext: playNext} }
}

func (m *LibraryModel) showSections() {
	items := []list.Item{sectionItem{sectionSongs}, sectionItem{sectionAlbums}, sectionItem{sectionArtists}, sectionItem{sectionPlaylists}}
	m.list.SetItems(items)
	m.list.Select(0)
}

func (m *LibraryModel) showPlaylists() {
	m.pane = paneItems
	items := make([]list.Item, len(m.playlists))
	for i, pl := range m.playlists {
		items[i] = playlistItem(pl)
	}
	m.list.SetItems(items)
	m.list.Select(0)
}

func (m *LibraryModel) showGroups(groups []trackGroup) {
	m.pane = paneItems
	items := make([]list.Item, len(groups))
	for i, group := range groups {
		if m.selectedSection == sectionAlbums {
			items[i] = albumItem{group}
		} else {
			items[i] = artistItem{group}
		}
	}
	m.list.SetItems(items)
	m.list.Select(0)
}

func (m *LibraryModel) invalidatePlaylistRequest() {
	m.drillRequestGeneration++
	m.drillRequestKind = playlistRequestNone
	m.drillPlaylist = provider.Playlist{}
	m.drillLoading = false
	m.drillErr = nil
}

func (m *LibraryModel) showTrackPane(title string, tracks []provider.Track, back libraryPane) {
	m.invalidatePlaylistRequest()
	m.showPlaylistTrackPane(title, tracks, back)
}

func (m *LibraryModel) showPlaylistTrackPane(title string, tracks []provider.Track, back libraryPane) {
	m.pane = paneTracks
	m.tracksBackPane = back
	m.drillTitle = title
	m.drillTracks = append([]provider.Track{}, tracks...)
	items := make([]list.Item, len(tracks))
	for i, t := range tracks {
		items[i] = trackListItem{t}
	}
	m.drillList.SetItems(items)
	m.drillList.Select(0)
}

func (m *LibraryModel) View() string {
	if m.pane == paneTracks {
		return m.renderDrillView()
	}
	header := m.renderHeader()
	if m.loading {
		return header + "\n\n  " + m.spinner.View() + " " + m.loadingText()
	}
	if m.loadErr != nil && m.pane != paneSections {
		return header + "\n\n" + centerLine(styles.QueueItemMuted.Render("Could not load: "+m.loadErr.Error()), m.width)
	}
	if len(m.list.Items()) == 0 {
		return header + "\n\n" + centerLine(styles.QueueItemMuted.Render(m.emptyText()), m.width)
	}
	return header + "\n" + m.list.View()
}

func (m *LibraryModel) renderDrillView() string {
	name := styles.SidebarActive.Render(m.drillTitle)
	if m.drillTitle == "" {
		name = styles.SidebarActive.Render(i18n.T("Tracks"))
	}
	hint := styles.QueueItemMuted.Render(i18n.T("  b/esc back · enter play · tab song next · shift+tab song to end · ctrl+q all next"))
	header := name + hint + "\n" + lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(strings.Repeat("─", m.width))
	if m.drillLoading {
		what := "Loading tracks…"
		if n := m.drillPlaylist.TrackCount; n > 0 {
			what = fmt.Sprintf("Loading %d tracks…", n)
		}
		return header + "\n\n  " + m.spinner.View() + " " + what
	}
	if m.drillErr != nil {
		return header + "\n\n" + centerLine(styles.QueueItemMuted.Render("Could not load tracks: "+m.drillErr.Error()), m.width)
	}
	if len(m.drillTracks) == 0 {
		return header + "\n\n" + centerLine(styles.QueueItemMuted.Render(i18n.T("No tracks found")), m.width)
	}
	m.drillList.SetSize(m.width, max(0, m.height-3))
	return header + "\n" + m.drillList.View()
}

func (m *LibraryModel) renderHeader() string {
	title := i18n.T("Library")
	if m.pane == paneItems {
		title = sectionTitle(m.selectedSection)
		if m.openArtist != nil {
			title += " · " + m.openArtist.Name
		}
	}
	return styles.TabActive.Render(title) + "\n" + lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(strings.Repeat("─", max(1, m.width)))
}

func (m *LibraryModel) loadingText() string {
	switch m.selectedSection {
	case sectionSongs:
		return i18n.T("Loading songs…")
	case sectionAlbums:
		return i18n.T("Loading albums…")
	case sectionArtists:
		if m.openArtist != nil {
			return i18n.T("Loading albums…")
		}
		return i18n.T("Loading artists…")
	case sectionPlaylists:
		return i18n.T("Loading playlists…")
	default:
		return i18n.T("Loading…")
	}
}

func (m *LibraryModel) emptyText() string {
	switch m.selectedSection {
	case sectionSongs:
		return i18n.T("No songs found")
	case sectionAlbums:
		return i18n.T("No albums found")
	case sectionArtists:
		return i18n.T("No artists found")
	case sectionPlaylists:
		return i18n.T("No playlists found")
	default:
		return i18n.T("No items found")
	}
}

func sectionTitle(section librarySection) string {
	switch section {
	case sectionSongs:
		return i18n.T("Songs")
	case sectionAlbums:
		return i18n.T("Albums")
	case sectionArtists:
		return i18n.T("Artists")
	case sectionPlaylists:
		return i18n.T("Playlists")
	default:
		return i18n.T("Library")
	}
}

func buildLibraryAlbums(tracks []provider.Track) []trackGroup {
	groups := make([]trackGroup, 0)
	index := make(map[string]int, len(tracks))
	for _, track := range tracks {
		title := track.Album
		if title == "" {
			title = "Unknown Album"
		}
		key := track.Artist + "\x00" + title
		if i, ok := index[key]; ok {
			groups[i].tracks = append(groups[i].tracks, track)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, trackGroup{title: title, desc: track.Artist, tracks: []provider.Track{track}})
	}
	// Albums keep the order their newest song arrives in (library songs come
	// most recently added first); put each album's songs back in disc and
	// track order. The sort is stable, so an album with no numbers
	// keeps the order it arrived in.
	for i := range groups {
		slices.SortStableFunc(groups[i].tracks, func(a, b provider.Track) int {
			return cmp.Or(cmp.Compare(a.DiscNumber, b.DiscNumber), cmp.Compare(a.TrackNumber, b.TrackNumber))
		})
	}
	return groups
}

func buildLibraryArtists(tracks []provider.Track) []trackGroup {
	groups := make([]trackGroup, 0)
	index := make(map[string]int, len(tracks))
	for _, track := range tracks {
		title := track.Artist
		if title == "" {
			title = "Unknown Artist"
		}
		if i, ok := index[title]; ok {
			groups[i].tracks = append(groups[i].tracks, track)
			continue
		}
		index[title] = len(groups)
		groups = append(groups, trackGroup{title: title, desc: fmt.Sprintf("%d tracks", 1), tracks: []provider.Track{track}})
	}
	for i := range groups {
		groups[i].desc = fmt.Sprintf("%d tracks", len(groups[i].tracks))
	}
	// Library songs arrive newest first, which is no order for a list of
	// names: sort artists alphabetically.
	slices.SortStableFunc(groups, func(a, b trackGroup) int {
		return strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title))
	})
	return groups
}

type sectionItem struct{ section librarySection }

func (s sectionItem) Title() string       { return sectionTitle(s.section) }
func (s sectionItem) Description() string { return i18n.T("enter to browse") }
func (s sectionItem) FilterValue() string { return s.Title() }

type albumItem struct{ group trackGroup }

func (a albumItem) Title() string       { return a.group.title }
func (a albumItem) Description() string { return a.group.desc }
func (a albumItem) FilterValue() string { return a.group.title + " " + a.group.desc }

type artistItem struct{ group trackGroup }

func (a artistItem) Title() string       { return a.group.title }
func (a artistItem) Description() string { return a.group.desc }
func (a artistItem) FilterValue() string { return a.group.title }

type playlistItem provider.Playlist

func (p playlistItem) Title() string { return p.Name }
func (p playlistItem) Description() string {
	if p.TrackCount > 0 {
		return fmt.Sprintf("%d tracks", p.TrackCount)
	}
	return i18n.T("playlist · enter to browse")
}
func (p playlistItem) FilterValue() string { return p.Name }

type trackListItem struct{ t provider.Track }

func (i trackListItem) Title() string       { return i.t.Title }
func (i trackListItem) Description() string { return fmt.Sprintf("%s — %s", i.t.Artist, i.t.Album) }
func (i trackListItem) FilterValue() string { return i.t.Title + " " + i.t.Artist }

// ShowTrackList opens a track list (e.g. an album reached from the song
// menu) in the library's track pane; back returns to the sections.
func (m *LibraryModel) ShowTrackList(title string, tracks []provider.Track) {
	m.invalidateLibraryRequest()
	m.loading = false
	m.loadErr = nil
	m.showTrackPane(title, tracks, paneSections)
}

// SelectedTrack returns the highlighted track in the track pane, or nil.
func (m *LibraryModel) SelectedTrack() *provider.Track {
	if m.pane != paneTracks {
		return nil
	}
	idx := m.drillList.Index()
	if idx < 0 || idx >= len(m.drillTracks) {
		return nil
	}
	t := m.drillTracks[idx]
	return &t
}

// ── Playlist editing ─────────────────────────────────────────────────────────

// SelectedPlaylist is the playlist highlighted in the list of playlists, or
// nil when that list isn't shown.
func (m *LibraryModel) SelectedPlaylist() *provider.Playlist {
	if m.pane != paneItems || m.selectedSection != sectionPlaylists || m.loading {
		return nil
	}
	if it, ok := m.list.SelectedItem().(playlistItem); ok {
		pl := provider.Playlist(it)
		return &pl
	}
	return nil
}

// ShowingPlaylists reports whether the list of playlists is shown.
func (m *LibraryModel) ShowingPlaylists() bool {
	return m.pane == paneItems && m.selectedSection == sectionPlaylists
}

// OpenPlaylist is the library playlist whose songs are shown, with the
// highlighted song and its place; nil when the track pane shows something
// else (an album, a preview).
func (m *LibraryModel) OpenPlaylist() (pl *provider.Playlist, t *provider.Track, idx int) {
	if m.pane != paneTracks || m.drillPlaylist.ID == "" || m.drillLoading {
		return nil, nil, -1
	}
	p := m.drillPlaylist
	idx = m.drillList.Index()
	if idx < 0 || idx >= len(m.drillTracks) {
		return &p, nil, -1
	}
	tr := m.drillTracks[idx]
	return &p, &tr, idx
}

// RemovedFromPlaylist drops the song at idx from the open playlist's list,
// after it was removed from the playlist itself.
func (m *LibraryModel) RemovedFromPlaylist(playlistID string, idx int, trackID string) {
	if m.pane != paneTracks || m.drillPlaylist.ID != playlistID {
		return
	}
	if idx < 0 || idx >= len(m.drillTracks) || m.drillTracks[idx].ID != trackID {
		idx = slices.IndexFunc(m.drillTracks, func(t provider.Track) bool { return t.ID == trackID })
	}
	if idx < 0 {
		return
	}
	m.drillTracks = slices.Delete(m.drillTracks, idx, idx+1)
	m.drillList.RemoveItem(idx)
	if n := len(m.drillTracks); n > 0 && m.drillList.Index() >= n {
		m.drillList.Select(n - 1)
	}
	for i := range m.playlists {
		if m.playlists[i].ID == playlistID && m.playlists[i].TrackCount > 0 {
			m.playlists[i].TrackCount--
		}
	}
}

// PlaylistsChanged forgets the loaded playlists (one was made, renamed or
// deleted): the list is loaded again, at once when it is shown, keeping the
// highlight on keepID when it is there.
func (m *LibraryModel) PlaylistsChanged(keepID string) tea.Cmd {
	m.playlistsLoaded = false
	if !m.ShowingPlaylists() {
		return nil
	}
	m.keepPlaylistID = keepID
	m.invalidateLibraryRequest()
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.loadPlaylists())
}

// RenamedPlaylist shows a playlist under its new name.
func (m *LibraryModel) RenamedPlaylist(id, name string) {
	for i := range m.playlists {
		if m.playlists[i].ID == id {
			m.playlists[i].Name = name
		}
	}
	if m.drillPlaylist.ID == id {
		m.drillPlaylist.Name = name
		m.drillTitle = name
	}
	if m.ShowingPlaylists() {
		idx := m.list.Index()
		m.showPlaylists()
		m.list.Select(idx)
	}
}
