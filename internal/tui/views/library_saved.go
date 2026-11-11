package views

import (
	"context"
	"fmt"
	"slices"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/provider"
)

// A service whose library keeps albums and artists of its own (saved
// albums, followed artists: provider.LibraryCollections) shows those in the
// Albums and Artists sections, instead of grouping the liked songs. An album
// opens with all its songs; an artist opens their albums.

const libraryRequestSaved libraryRequestKind = 100

type savedListMsg struct {
	generation uint64
	section    librarySection
	albums     []provider.Album
	artists    []provider.Artist
	err        error
}

type artistAlbumsMsg struct {
	generation uint64
	artist     provider.Artist
	albums     []provider.Album
	err        error
}

type albumTracksMsg struct {
	generation uint64
	album      provider.Album
	tracks     []provider.Track
	err        error
}

type savedAlbumItem struct {
	a provider.Album
}

func (i savedAlbumItem) Title() string { return i.a.Title }
func (i savedAlbumItem) Description() string {
	d := i.a.Artist
	if y := releaseYear(i.a.ReleaseDate); y != "" {
		if d != "" {
			d += " · "
		}
		d += y
	}
	if i.a.TrackCount > 0 {
		if d != "" {
			d += " · "
		}
		d += fmt.Sprintf(i18n.T("%d tracks"), i.a.TrackCount)
	}
	return d
}
func (i savedAlbumItem) FilterValue() string { return i.a.Title + " " + i.a.Artist }

type savedArtistItem struct{ a provider.Artist }

func (i savedArtistItem) Title() string       { return i.a.Name }
func (i savedArtistItem) Description() string { return i18n.T("artist · enter for albums") }
func (i savedArtistItem) FilterValue() string { return i.a.Name }

func releaseYear(d string) string {
	if len(d) >= 4 {
		return d[:4]
	}
	return ""
}

// collections is the provider's own albums and artists, or nil.
func (m *LibraryModel) collections() provider.LibraryCollections {
	c, _ := m.provider.(provider.LibraryCollections)
	return c
}

// usesSaved reports whether the section shows the service's saved albums or
// followed artists.
func (m *LibraryModel) usesSaved(s librarySection) bool {
	return (s == sectionAlbums || s == sectionArtists) && m.collections() != nil
}

// openSaved loads the saved albums or followed artists.
func (m *LibraryModel) openSaved() (*LibraryModel, tea.Cmd) {
	m.libraryRequestGeneration++
	m.libraryRequestSection = m.selectedSection
	m.libraryRequestKind = libraryRequestSaved
	m.openArtist = nil
	gen, section := m.libraryRequestGeneration, m.selectedSection
	coll := m.collections()
	m.loading = true
	m.loadErr = nil
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), libraryLoadTimeout)
		defer cancel()
		msg := savedListMsg{generation: gen, section: section}
		if section == sectionAlbums {
			msg.albums, msg.err = coll.GetSavedAlbums(ctx)
		} else {
			msg.artists, msg.err = coll.GetSavedArtists(ctx)
		}
		return msg
	})
}

func (m *LibraryModel) currentSaved(gen uint64) bool {
	return m.libraryRequestGeneration == gen && m.libraryRequestKind == libraryRequestSaved
}

// updateSaved handles the messages of this file; ok is false for others.
func (m *LibraryModel) updateSaved(msg tea.Msg) (cmd tea.Cmd, ok bool) {
	switch msg := msg.(type) {
	case savedListMsg:
		if !m.currentSaved(msg.generation) || msg.section != m.selectedSection {
			return nil, true
		}
		m.loading = false
		m.loadErr = msg.err
		m.pane = paneItems
		if msg.err != nil {
			m.list.SetItems(nil)
			return nil, true
		}
		if msg.section == sectionAlbums {
			m.savedAlbums = msg.albums
			m.showSavedAlbums(msg.albums)
		} else {
			m.savedArtists = msg.artists
			m.showSavedArtists()
		}
		return nil, true
	case artistAlbumsMsg:
		if !m.currentSaved(msg.generation) || m.openArtist == nil || m.openArtist.ID != msg.artist.ID {
			return nil, true
		}
		m.loading = false
		m.loadErr = msg.err
		if msg.err != nil {
			m.list.SetItems(nil)
			return nil, true
		}
		m.showSavedAlbums(msg.albums)
		return nil, true
	case albumTracksMsg:
		if m.pane != paneTracks || m.drillAlbumID != msg.album.ID || m.drillRequestGeneration != msg.generation {
			return nil, true
		}
		m.drillLoading = false
		m.drillErr = msg.err
		if msg.err == nil {
			m.showPlaylistTrackPane(msg.album.Title, msg.tracks, paneItems)
		}
		return nil, true
	}
	return nil, false
}

func (m *LibraryModel) showSavedAlbums(albums []provider.Album) {
	m.pane = paneItems
	items := make([]list.Item, len(albums))
	for i, a := range albums {
		items[i] = savedAlbumItem{a}
	}
	m.list.SetItems(items)
	m.list.Select(0)
}

func (m *LibraryModel) showSavedArtists() {
	m.pane = paneItems
	items := make([]list.Item, len(m.savedArtists))
	for i, a := range m.savedArtists {
		items[i] = savedArtistItem{a}
	}
	m.list.SetItems(items)
	m.list.Select(0)
}

// openSavedAlbum shows all the songs of the album.
func (m *LibraryModel) openSavedAlbum(a provider.Album) tea.Cmd {
	m.invalidatePlaylistRequest()
	m.drillRequestGeneration++
	gen := m.drillRequestGeneration
	m.drillAlbumID = a.ID
	m.drillTitle = a.Title
	m.pane = paneTracks
	m.tracksBackPane = paneItems
	m.drillTracks = nil
	m.drillErr = nil
	m.drillLoading = true
	m.drillList.SetItems(nil)
	prov := m.provider
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), libraryLoadTimeout)
		defer cancel()
		tracks, err := prov.GetAlbumTracks(ctx, a.ID)
		return albumTracksMsg{generation: gen, album: a, tracks: tracks, err: err}
	})
}

// openSavedArtist shows the artist's albums, in place of the artists.
func (m *LibraryModel) openSavedArtist(a provider.Artist) tea.Cmd {
	m.libraryRequestGeneration++
	m.libraryRequestKind = libraryRequestSaved
	gen := m.libraryRequestGeneration
	ar := a
	m.openArtist = &ar
	m.artistIdx = m.list.Index()
	m.loading = true
	m.loadErr = nil
	m.list.SetItems(nil)
	prov := m.provider
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), libraryLoadTimeout)
		defer cancel()
		albums, err := prov.GetArtistAlbums(ctx, a.ID)
		return artistAlbumsMsg{generation: gen, artist: a, albums: albums, err: err}
	})
}

// closeSavedArtist goes back from an artist's albums to the artists.
func (m *LibraryModel) closeSavedArtist() bool {
	if m.openArtist == nil {
		return false
	}
	m.openArtist = nil
	m.libraryRequestGeneration++
	m.loading = false
	m.loadErr = nil
	m.showSavedArtists()
	m.list.Select(m.artistIdx)
	return true
}

// queueAlbumCmd loads all of an album's songs and queues them.
func queueAlbumCmd(prov provider.Provider, a provider.Album, playNext bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), libraryLoadTimeout)
		defer cancel()
		tracks, err := prov.GetAlbumTracks(ctx, a.ID)
		if err != nil {
			return QueueTracksMsg{Label: a.Title, Err: err}
		}
		ids := make([]string, len(tracks))
		for i, t := range tracks {
			ids[i] = PlaybackID(t)
		}
		return QueueTracksMsg{IDs: ids, Tracks: tracks, Label: a.Title, PlayNext: playNext}
	}
}

// SelectedSavedAlbum is the saved album highlighted in the Albums section
// (not an artist's album list), or nil.
func (m *LibraryModel) SelectedSavedAlbum() *provider.Album {
	if m.pane != paneItems || m.selectedSection != sectionAlbums || m.loading || m.openArtist != nil {
		return nil
	}
	if it, ok := m.list.SelectedItem().(savedAlbumItem); ok {
		a := it.a
		return &a
	}
	return nil
}

// SelectedSavedArtist is the followed artist highlighted in the Artists
// section, or nil.
func (m *LibraryModel) SelectedSavedArtist() *provider.Artist {
	if m.pane != paneItems || m.selectedSection != sectionArtists || m.loading || m.openArtist != nil {
		return nil
	}
	if it, ok := m.list.SelectedItem().(savedArtistItem); ok {
		a := it.a
		return &a
	}
	return nil
}

// RemovedSaved drops an album or artist taken out of the library from the
// list shown.
func (m *LibraryModel) RemovedSaved(id string) {
	m.savedAlbums = slices.DeleteFunc(m.savedAlbums, func(a provider.Album) bool { return a.ID == id })
	m.savedArtists = slices.DeleteFunc(m.savedArtists, func(a provider.Artist) bool { return a.ID == id })
	if m.pane != paneItems || m.openArtist != nil {
		return
	}
	idx := m.list.Index()
	switch m.selectedSection {
	case sectionAlbums:
		m.showSavedAlbums(m.savedAlbums)
	case sectionArtists:
		m.showSavedArtists()
	default:
		return
	}
	if n := len(m.list.Items()); n > 0 {
		m.list.Select(min(idx, n-1))
	}
}

// SavedChanged forgets nothing but reloads the section when it shows the
// saved albums or artists (one was added from the song menu).
func (m *LibraryModel) SavedChanged() tea.Cmd {
	if m.pane == paneItems && m.openArtist == nil && m.usesSaved(m.selectedSection) && !m.loading {
		_, cmd := m.openSaved()
		return cmd
	}
	return nil
}
