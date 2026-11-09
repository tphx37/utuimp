package tui

import (
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/tui/art"
	"github.com/tphx37/utuimp/internal/tui/views"
)

// Cover view (v): a second, frameless layout in the manner of a fullscreen
// player. The cover sits on the left with the song under it, the lyrics fill
// the right; the queue, feed, library and every other panel open in a frame
// in place of the lyrics.
//
//	 ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄      we were young
//	 █   cover      █      ▸ and the night was long
//	 ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀      nobody told us
//	  Title — Artist
//	  Album • 1:02 / 3:40
//	  ━━━━━━━━━━──────
//	  ↺   ⇄   ⏸   ♡
//	  spc play/pause · …   (status rows)

// LayoutCover is the config value of the cover view.
const LayoutCover = "cover"

// coverSideBySideMinW is the narrowest terminal that fits the cover beside
// the lyrics; narrower ones stack the song above them without a cover.
const coverSideBySideMinW = 90

// coverTextRows is the song block under the cover (nowPlayingTextLines).
const coverTextRows = 9

// coverDims returns the layout: the left column's width (0 when stacked),
// the right column's width and the height of the area above the status rows.
func (m *Model) coverDims() (leftW, rightW, h int) {
	h = max(4, m.height-len(m.statusLines(m.width-4)))
	if m.width < coverSideBySideMinW {
		return 0, m.width, h - coverTextRows
	}
	leftW = min(70, max(36, m.width*2/5))
	return leftW, m.width - leftW - 1, h
}

// coverPanelFramed reports whether the right column shows a framed panel
// (anything but the lyrics).
func (m *Model) coverPanelFramed() bool {
	if m.debugView || m.mode == modeSearch || m.mode == modeCommand {
		return true
	}
	return m.activePanel < 0 || m.panels[m.activePanel] != m.lyricsP
}

// coverArtLines renders the current cover at size, from the per-size cache.
// It returns nil while the cover is loading.
func (m *Model) coverArtLines(size art.Size) []string {
	if m.artwork.rendered == nil {
		m.artwork.rendered = map[art.Size][]string{}
	}
	lines := m.artwork.rendered[size]
	if lines == nil && m.artwork.img != nil && m.renderArt != nil {
		const maxRenderedArtworkSizes = 16
		if len(m.artwork.rendered) >= maxRenderedArtworkSizes {
			m.artwork.rendered = map[art.Size][]string{}
		}
		lines = m.renderArt(m.artwork.img, size)
		m.artwork.rendered[size] = lines
	}
	return lines
}

// coverWanted reports whether the current track has a cover the terminal can
// show (loaded or still loading).
func (m *Model) coverWanted() bool {
	if m.supportsArtColor == nil || !m.supportsArtColor() {
		return false
	}
	t := m.playerState.Track
	return t != nil && t.ArtworkURL != "" && m.artwork.url == t.ArtworkURL && !m.artwork.failed
}

// coverLeftLines renders the left column: cover, then the song block,
// centred vertically in h rows.
func (m *Model) coverLeftLines(w, h int) []string {
	text := m.nowPlayingTextLines(w-2, coverTextRows)
	var block []string
	if m.coverWanted() {
		aspect := m.artCellAsp
		if aspect <= 0 {
			aspect = 2.0
		}
		rows := h - coverTextRows - 1
		cols := int(math.Round(float64(rows) * aspect))
		for rows > 2 && cols > w-4 {
			rows--
			cols = int(math.Round(float64(rows) * aspect))
		}
		if rows >= 4 {
			art := m.coverArtLines(art.Size{Width: cols, Height: rows})
			for i := range rows {
				block = append(block, centerStr(safeIdx(art, i), w-2))
			}
			block = append(block, "")
		}
	}
	block = append(block, text...)
	top := max(0, (h-len(block))/2)
	out := make([]string, h)
	for i, l := range block {
		if top+i < h {
			out[top+i] = " " + l
		}
	}
	return out
}

// coverRightLines renders the right column: the lyrics bare, or the open
// panel (the queue when none is) in a frame.
func (m *Model) coverRightLines(w, h int) []string {
	if !m.coverPanelFramed() {
		// One blank row on top, level with the frame of a panel.
		m.lyricsP.m.SetBare(true)
		m.lyricsP.SetSize(w-4, h-1)
		lyr := toLines(m.lyricsP.View(), h-1)
		out := make([]string, h)
		for i := 1; i < h; i++ {
			out[i] = "  " + padRight(safeIdx(lyr, i-1), w-4)
		}
		return out
	}
	inner := w - 4
	panelH := max(1, h-2)
	var lines []string
	switch {
	case m.debugView:
		lines = m.debugLogLines(inner, panelH)
	case m.mode == modeSearch:
		lines = m.searchLines(inner, panelH)
	case m.mode == modeCommand:
		lines = m.commandLines(inner, panelH)
	case m.activePanel >= 0:
		p := m.panels[m.activePanel]
		p.SetSize(inner, panelH)
		lines = toLines(p.View(), panelH)
	default:
		lines = m.queuePanelLines(inner, panelH)
	}
	focused := m.activePanel >= 0 || m.mode != modeNormal || m.debugView || m.queueMiniCursor >= 0
	out := []string{boxTop(w, "", focused)}
	for i := range panelH {
		out = append(out, boxRow(safeIdx(lines, i), w, focused))
	}
	return append(out, boxBottom(w, m.volumeLabel(), focused))
}

// renderCoverLayout renders the whole cover view.
func (m *Model) renderCoverLayout() string {
	leftW, rightW, h := m.coverDims()
	var rows []string
	if leftW > 0 {
		left := m.coverLeftLines(leftW, h)
		right := m.coverRightLines(rightW, h)
		for i := range h {
			rows = append(rows, padRight(left[i], leftW)+" "+padRight(safeIdx(right, i), rightW))
		}
	} else {
		for _, l := range m.nowPlayingTextLines(m.width-2, coverTextRows) {
			rows = append(rows, " "+l)
		}
		rows = append(rows, m.coverRightLines(rightW, h)...)
	}
	var sb strings.Builder
	for _, r := range rows {
		sb.WriteString(padRight(r, m.width) + "\n")
	}
	status := m.statusLines(m.width - 4)
	for i, line := range status {
		sb.WriteString("  " + padRight(line, m.width-2))
		if i < len(status)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// coverPanelHeight is the framed panel's inner height in the cover view.
func (m *Model) coverPanelHeight() int {
	_, _, h := m.coverDims()
	return max(3, h-2)
}

// lyricsPanelIdx is the lyrics panel's position in m.panels.
func (m *Model) lyricsPanelIdx() int {
	for i, p := range m.panels {
		if p == m.lyricsP {
			return i
		}
	}
	return -1
}

// openLyricsPanel shows the lyrics, fetching them when the current song's
// aren't loaded yet.
func (m *Model) openLyricsPanel() tea.Cmd {
	m.activePanel = m.lyricsPanelIdx()
	if m.lastLyricsTrackID == "" && m.playerState.Track != nil {
		m.lastLyricsTrackID = views.PlaybackID(*m.playerState.Track)
		m.lyricsP.m.SetLoading()
		return m.fetchLyricsCmd(m.playerState.Track)
	}
	return nil
}

// toggleCoverView switches between the boxed layout and the cover view and
// remembers the choice.
func (m *Model) toggleCoverView() tea.Cmd {
	m.coverView = !m.coverView
	m.cfg.Layout = ""
	if m.coverView {
		m.cfg.Layout = LayoutCover
	}
	if err := m.cfg.Save(""); err != nil {
		m.appendLog(fmt.Sprintf("[layout] config save error: %v", err))
	}
	if !m.coverView {
		m.lyricsP.m.SetBare(false)
		if m.activePanel == m.lyricsPanelIdx() {
			m.activePanel = -1
		}
		return nil
	}
	cmds := []tea.Cmd{m.openLyricsPanel()}
	if t := m.playerState.Track; t != nil && t.ArtworkURL != "" {
		cmds = append(cmds, m.fetchArtworkCmd(t.ArtworkURL, m.artworkGen))
	}
	return tea.Batch(cmds...)
}

// layoutHint names what v switches to.
func (m *Model) layoutHint() string {
	if m.coverView {
		return i18n.T(" boxed view")
	}
	return i18n.T(" cover view")
}
