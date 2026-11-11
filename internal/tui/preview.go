package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/provider"
)

// An album or playlist picked in the feed or in search opens as a preview
// first: its songs in the library's track pane, where enter plays from the
// chosen one and esc goes back to where it was opened from. (The library's
// own albums and playlists, and the song menu's album, already work so.)

// previewOrigin is where esc leaves a collection preview for.
type previewOrigin struct {
	panel  int  // the panel open before (-1: none)
	search bool // the preview was opened from search
}

// collectionPreviewMsg carries the songs of a collection to preview.
type collectionPreviewMsg struct {
	title  string
	tracks []provider.Track
	err    error
	from   previewOrigin
}

// previewCmd loads a collection for a preview with fetch (one of the feed's
// or search's track loaders) and turns its result into a preview.
func (m *Model) previewCmd(fetch tea.Cmd) tea.Cmd {
	if fetch == nil {
		return nil
	}
	from := previewOrigin{panel: m.activePanel, search: m.mode == modeSearch}
	return func() tea.Msg {
		switch msg := fetch().(type) {
		case feedTracksMsg:
			return collectionPreviewMsg{title: msg.item.Title, tracks: msg.tracks, err: msg.err, from: from}
		case searchCollectionTracksMsg:
			return collectionPreviewMsg{title: msg.label, tracks: msg.tracks, err: msg.err, from: from}
		}
		return nil
	}
}

// handleCollectionPreview opens the loaded collection in the library.
func (m *Model) handleCollectionPreview(msg collectionPreviewMsg) {
	m.busyLabel = ""
	if msg.err != nil || len(msg.tracks) == 0 {
		m.errMsg = i18n.T("no playable tracks found")
		if msg.err != nil {
			m.errMsg = fmt.Sprintf(i18n.T("could not load %q: %v"), msg.title, msg.err)
			m.appendLog(fmt.Sprintf("[preview] %q: %v", msg.title, msg.err))
		}
		m.errExpiry = time.Now().Add(4 * time.Second)
		return
	}
	m.library.m.ShowTrackList(msg.title, msg.tracks)
	m.activePanel = m.libraryPanelIdx()
	m.mode = modeNormal
	m.coverQueueOpen = false
	from := msg.from
	m.previewBack = &from
	m.appendLog(fmt.Sprintf("[preview] %q (%d tracks)", msg.title, len(msg.tracks)))
}

// leavePreview closes a collection preview, back to where it was opened.
func (m *Model) leavePreview() tea.Cmd {
	from := *m.previewBack
	m.previewBack = nil
	m.library.m.ResetTopLevel()
	m.activePanel = from.panel
	if from.search {
		m.mode = modeSearch
		m.searchCursor = len([]rune(m.searchQuery))
		return nil
	}
	if m.activePanel < 0 && m.coverView {
		return m.coverHome()
	}
	return nil
}

func (m *Model) libraryPanelIdx() int {
	for i, p := range m.panels {
		if p == m.library {
			return i
		}
	}
	return -1
}

// showPlaybackHome is where starting something to play lands: in the cover
// view its home screen (the lyrics, or the song with them hidden) rather
// than the queue; in the boxed view the main screen.
func (m *Model) showPlaybackHome() tea.Cmd {
	m.mode = modeNormal
	m.previewBack = nil
	m.queueMiniCursor = -1
	if m.coverView {
		return m.coverHome()
	}
	m.activePanel = -1
	return nil
}
