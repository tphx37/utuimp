package tui

import (
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/tui/art"
	"github.com/tphx37/utuimp/internal/tui/styles"
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

// coverTextRows is the song block under the cover: nowPlayingTextLines'
// nine rows with the volume added under the controls.
const coverTextRows = 10

// coverPads returns the empty rows kept above and below the content, so the
// view breathes like a fullscreen player instead of touching the edges.
func coverPads(h int) (top, bottom int) {
	return min(3, max(1, h/12)), min(2, max(1, h/16))
}

// coverDims returns the layout: the left column's width (0 when stacked),
// the right column's width and the content height between the paddings.
func (m *Model) coverDims() (leftW, rightW, h int) {
	full := max(6, m.height-len(m.statusLines(m.width-4)))
	top, bottom := coverPads(full)
	h = max(4, full-top-bottom)
	if m.width < coverSideBySideMinW {
		return 0, m.width, max(4, h-coverTextRows)
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
	// The lyrics' source and language pickers (c, T) are panels too.
	return m.activePanel < 0 || m.panels[m.activePanel] != m.lyricsP || m.lyricsP.m.PickerOpen()
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

// coverPlaceholder is a frame the size of the cover with a note in the
// middle: it holds the cover's place when there is none (nothing playing, no
// artwork, a terminal without colours), so the layout keeps its shape.
func coverPlaceholder(cols, rows int) []string {
	if cols < 4 || rows < 2 {
		return nil
	}
	bs := borderStyle(false)
	out := []string{bs.Render("╭" + strings.Repeat("─", cols-2) + "╮")}
	mid := (rows - 2) / 2
	for i := range rows - 2 {
		inner := strings.Repeat(" ", cols-2)
		if i == mid {
			inner = padRight(centerStr(styles.QueueItemMuted.Render("♪"), cols-2), cols-2)
		}
		out = append(out, bs.Render("│")+inner+bs.Render("│"))
	}
	return append(out, bs.Render("╰"+strings.Repeat("─", cols-2)+"╯"))
}

// coverIdleText is the song block while nothing plays: the same rows as a
// playing song, greyed, so the screen keeps its shape.
func (m *Model) coverIdleText(w int) []string {
	muted := styles.QueueItemMuted
	barW := max(10, w-8)
	return []string{
		"",
		centerStr(muted.Render(i18n.T("Nothing playing")), w),
		centerStr(muted.Render("— • 0:00 / 0:00"), w),
		"",
		centerStr(muted.Render(strings.Repeat("─", barW)), w),
		"",
		centerStr(muted.Render("↺   ⇄   ▶   ♡"), w),
		"",
		m.coverVolumeLine(w),
		m.statusLine(w),
	}
}

// coverVolumeLine is the volume as a short slider under the controls:
// "♪ ━━━━━━━──── 70%", or "muted".
func (m *Model) coverVolumeLine(w int) string {
	muted := styles.QueueItemMuted
	if m.preMuteVol >= 0 {
		return centerStr(styles.Playing.Render(i18n.T("🔇 muted")), w)
	}
	vol := min(max(m.playerState.Volume, 0), 1)
	barW := min(24, max(8, w/3))
	filled := int(vol*float64(barW) + 0.5)
	bar := styles.ProgressBar.Render(strings.Repeat("━", filled)) + muted.Render(strings.Repeat("─", barW-filled))
	return centerStr(muted.Render("♪ ")+bar+muted.Render(fmt.Sprintf(" %3d%%", int(vol*100+0.5))), w)
}

// coverSongText is the song block: the playing song or the idle one.
func (m *Model) coverSongText(w int) []string {
	if m.playerState.Track == nil {
		return m.coverIdleText(w)
	}
	// Rows: "", title, album, "", progress, "", controls, "", status.
	lines := m.nowPlayingTextLines(w, 9)
	out := append([]string{}, lines[:8]...)
	return append(out, m.coverVolumeLine(w), lines[8])
}

// coverLeftLines renders the left column: the cover (or its placeholder),
// then the song block, centred vertically in h rows.
func (m *Model) coverLeftLines(w, h int) []string {
	rows, cols, top := m.coverGeom(w, h)
	var block []string
	if rows > 0 {
		var cover []string
		if m.coverWanted() {
			cover = m.coverArtLines(art.Size{Width: cols, Height: rows})
			if cover == nil {
				cover = make([]string, rows) // loading: keep the space
			}
		} else {
			cover = coverPlaceholder(cols, rows)
		}
		for i := range rows {
			block = append(block, centerStr(safeIdx(cover, i), w-2))
		}
		block = append(block, "")
	}
	block = append(block, m.coverSongText(w-2)...)
	out := make([]string, h)
	for i, l := range block {
		if top+i < h {
			out[top+i] = " " + l
		}
	}
	return out
}

// coverGeom places the cover in a left column w wide and h high: its size
// in cells (rows 0 when there is no room for one) and the row where the
// column's block (cover, gap, song) starts, centred vertically.
func (m *Model) coverGeom(w, h int) (rows, cols, top int) {
	aspect := m.artCellAsp
	if aspect <= 0 {
		aspect = 2.0
	}
	rows = min(h-coverTextRows-1, 24)
	cols = int(math.Round(float64(rows) * aspect))
	for rows > 2 && cols > w-6 {
		rows--
		cols = int(math.Round(float64(rows) * aspect))
	}
	block := coverTextRows
	if rows < 4 {
		rows, cols = 0, 0
	} else {
		block += rows + 1
	}
	return rows, cols, max(0, (h-block)/2)
}

// coverLyricsBand is the rows the lyrics may use in a column h high: from
// the cover's top edge to its bottom edge, or all of it without a cover or
// in the stacked layout.
func (m *Model) coverLyricsBand(h int) (top, height int) {
	leftW, _, _ := m.coverDims()
	if leftW == 0 {
		return 0, h
	}
	rows, _, top := m.coverGeom(leftW, h)
	if rows == 0 {
		return 0, h
	}
	return top, rows
}

// coverIdleRight fills the lyrics side while nothing plays: the start-up
// error if the service failed, else where to find music.
func (m *Model) coverIdleRight(w, h int) []string {
	muted := styles.QueueItemMuted
	accent := styles.KeyName
	var rows []string
	if m.engineErr != "" {
		wrapped := lipgloss.NewStyle().Width(max(20, w-8)).Render(m.engineErr)
		for _, r := range strings.Split(wrapped, "\n") {
			rows = append(rows, styles.ErrorStyle.Render(strings.TrimRight(r, " ")))
		}
	} else {
		rows = []string{
			styles.Header.Render(i18n.T("Nothing playing yet")),
			"",
			accent.Render("/") + muted.Render(i18n.T("  search")),
			accent.Render("l") + muted.Render(i18n.T("  library")),
			accent.Render("F") + muted.Render(i18n.T("  feed")),
			accent.Render("q") + muted.Render(i18n.T("  history")),
		}
	}
	out := make([]string, h)
	top := max(0, (h-len(rows))/2)
	for i, r := range rows {
		if top+i < h {
			out[top+i] = "  " + r
		}
	}
	return out
}

// coverRightLines renders the right column: the lyrics bare, or the open
// panel (the queue when none is) in a frame.
func (m *Model) coverRightLines(w, h int) []string {
	if !m.coverPanelFramed() {
		if m.playerState.Track == nil {
			return m.coverIdleRight(w, h)
		}
		// Between the cover's top and bottom edges, in a column narrow
		// enough to read like a page rather than a log.
		top, bandH := m.coverLyricsBand(h)
		textW := min(w-6, 56)
		m.lyricsP.m.SetBare(true)
		m.lyricsP.SetSize(textW, bandH)
		lyr := m.lyricsP.m.CoverView(textW, bandH)
		out := make([]string, h)
		for i := range bandH {
			if top+i < h {
				out[top+i] = "    " + padRight(lyr[i], w-6)
			}
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
	return append(out, boxBottom(w, "", focused)) // the volume is on the left
}

// renderCoverLayout renders the whole cover view.
func (m *Model) renderCoverLayout() string {
	leftW, rightW, h := m.coverDims()
	full := max(6, m.height-len(m.statusLines(m.width-4)))
	top, _ := coverPads(full)
	rows := make([]string, top)
	if leftW > 0 {
		left := m.coverLeftLines(leftW, h)
		right := m.coverRightLines(rightW, h)
		for i := range h {
			rows = append(rows, padRight(left[i], leftW)+" "+padRight(safeIdx(right, i), rightW))
		}
	} else {
		for _, l := range m.coverSongText(m.width - 2) {
			rows = append(rows, " "+l)
		}
		rows = append(rows, m.coverRightLines(rightW, h)...)
	}
	for len(rows) < full {
		rows = append(rows, "")
	}
	var sb strings.Builder
	for _, r := range rows[:full] {
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

// coverHomeHints lists the keys of the cover view's home screen (the
// lyrics): the panels, the lyrics' own keys while a song plays, and the
// view switch.
func (m *Model) coverHomeHints() []string {
	muted := styles.QueueItemMuted
	accent := styles.KeyName
	hint := func(k, what string) string { return accent.Render(k) + muted.Render(i18n.T(what)) }
	parts := []string{
		styles.ModeNormal.Render(i18n.T("COVER")),
		hint("/", " search"),
		hint(i18n.T("l"), " library"),
		hint(i18n.T("q"), " history"),
		hint(i18n.T("F"), " feed"),
		hint(i18n.T("esc"), " queue"),
		hint(i18n.T("e"), " equalizer"),
		hint(i18n.T("i"), " song menu"),
	}
	if m.playerState.Track != nil {
		parts = append(parts,
			hint(i18n.T("j/k"), " scroll"),
			hint(i18n.T("Enter"), " seek"),
			hint(i18n.T("t"), " translate"),
			hint(i18n.T("T"), " language"),
			hint(i18n.T("c"), " source"),
		)
	}
	return append(parts,
		hint("v", " boxed view"),
		hint(":", " command"),
		hint("?", " about"),
		hint(i18n.T(":q"), " quit"),
	)
}
