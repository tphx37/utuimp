package tui

import (
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/provider"
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
// nine rows with the artist on a row of its own under the title and the
// volume added under the controls.
const coverTextRows = 11

// coverPads returns the empty rows kept above and below the content, so the
// view breathes like a fullscreen player instead of touching the edges.
func coverPads(h int) (top, bottom int) {
	return min(3, max(1, h/12)), min(2, max(1, h/16))
}

// coverDims returns the layout: the left column's width (0 when stacked),
// the right column's width and the content height between the paddings.
func (m *Model) coverDims() (leftW, rightW, h int) {
	full := max(6, m.height-len(m.coverStatusLines()))
	top, bottom := coverPads(full)
	h = max(4, full-top-bottom)
	if m.width < coverSideBySideMinW {
		return 0, m.width, max(4, h-coverTextRows)
	}
	leftW = min(70, max(36, m.width*2/5))
	if m.coverPlain {
		leftW = min(80, max(36, m.width*2/5)) // leave more room for the song title
	}
	return leftW, m.width - leftW - 1, h
}

// coverPanelFramed reports whether the right column shows a framed panel
// (anything but the home screen: the lyrics, or the song with them hidden).
func (m *Model) coverPanelFramed() bool {
	if m.debugView || m.mode == modeSearch || m.mode == modeCommand {
		return true
	}
	if m.activePanel < 0 && m.coverPlain {
		leftW, _, _ := m.coverDims()
		// Stacked, the song is already on top: show the queue under it.
		return m.coverQueueOpen || leftW == 0
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
		centerStr(muted.Render("—"), w),
		"",
		"",
		centerStr(muted.Render("0:00  "+strings.Repeat("─", max(6, barW-12))+"  0:00"), w),
		"",
		m.coverIdleButtons(w),
		"",
		m.coverVolumeLine(w),
		m.statusLine(w),
	}
}

// coverVolumeLine is the volume as a short slider under the controls:
// "♪ ━━━━━━━──── 70%", or "muted".
func (m *Model) coverVolumeLine(w int) string {
	muted := styles.QueueItemMuted
	if m.preMuteVol >= 0 && !m.lyricsPicOn() {
		return centerStr(styles.Playing.Render(i18n.T("🔇 muted")), w)
	}
	vol := min(max(m.playerState.Volume, 0), 1)
	if m.preMuteVol >= 0 {
		vol = 0 // muted: the drawn loudspeaker is crossed out, the line empty
	}
	barW := min(24, max(8, w/3))
	filled := int(vol*float64(barW) + 0.5)
	bar := styles.ProgressBar.Render(strings.Repeat("━", filled)) + muted.Render(strings.Repeat("─", barW-filled))
	// Both sides as wide as "100%", so the bar sits in the middle like the
	// progress bar above it does between its two times.
	pct := fmt.Sprintf("%d%%", int(vol*100+0.5))
	left := fmt.Sprintf("%4s", "♪")
	right := fmt.Sprintf("%-4s", pct)
	leftS, rightS := muted.Render(left), muted.Render(right)
	if s, ok := m.label(labelSpeaker, "", 4, alignRight); ok {
		leftS = s
	}
	if s, ok := m.label(labelSmall, pct, 4, alignLeft); ok {
		rightS = s
	}
	return centerStr(leftS+"  "+bar+"  "+rightS, w)
}

// coverSongText is the song block: the playing song or the idle one.
func (m *Model) coverSongText(w int) []string {
	if m.playerState.Track == nil {
		return m.coverIdleText(w)
	}
	// Rows: "", title, album, "", progress, "", controls, "", status; the
	// title row is split into the title and the artist under it.
	lines := m.nowPlayingTextLines(w, 9)
	title, artist := m.coverTitleLines(m.playerState.Track, w)
	out := []string{lines[0], title}
	if m.songBig {
		out = append(out, make([]string, songBigRows-1)...) // the large title's rows
	}
	out = append(out, artist)
	if m.songBig {
		out = append(out, make([]string, songBigRows-1)...)
	}
	if m.songBig && m.pixButtons() {
		// album, "", progress, "", controls, "": the larger buttons get a
		// clear row more over and under them (see songBigCtl).
		out = append(out, lines[2:6]...)
		out = append(out, "", lines[6], "", lines[7])
	} else {
		out = append(out, lines[2:8]...)
	}
	return append(out, m.coverVolumeLine(w), lines[8])
}

// coverTitleLines are the song's title (with the explicit mark) and, under
// it, the artist, each centred in w.
func (m *Model) coverTitleLines(t *provider.Track, w int) (title, artist string) {
	titleStyle := styles.NowPlayingTitle
	if m.playerState.Playing || m.playerState.Loading {
		titleStyle = styles.NowPlayingTitlePlaying
	}
	title = titleStyle.Render(t.Title)
	if strings.EqualFold(t.ContentRating, "explicit") {
		title += " " + styles.QueueItemMuted.Render(explicitMark)
	}
	title = m.scrollTitle(title, w)
	artist = ansi.Truncate(styles.NowPlayingTrackArtist.Render(t.Artist), w, "…")
	title, artist = centerStr(title, w), centerStr(artist, w)
	rows := 1
	if m.songBig {
		rows = songBigRows
	}
	if s, ok := m.labelSized(labelTitle, t.Title, w, alignCentre, rows, m.labelScale(labelTitle)); ok {
		title = s
	}
	if s, ok := m.labelSized(labelArtist, t.Artist, w, alignCentre, rows, m.labelScale(labelArtist)); ok {
		artist = s
	}
	return title, artist
}

// coverLeftLines renders the left column: the cover (or its placeholder),
// then the song block, centred vertically in h rows.
func (m *Model) coverLeftLines(w, h int) []string {
	rows, cols, top := m.coverGeom(w, h)
	var block []string
	if rows > 0 {
		var cover []string
		switch {
		case m.coverWanted() && m.linesCoverAt(1+max(0, (w-2-cols)/2), top, cols, rows):
			cover = make([]string, rows) // the picture goes under these cells
		case m.coverWanted():
			cover = m.coverArtLines(art.Size{Width: cols, Height: rows})
			if cover == nil {
				cover = make([]string, rows) // loading: keep the space
			}
		default:
			cover = coverPlaceholder(cols, rows)
		}
		for i := range rows {
			block = append(block, centerStr(safeIdx(cover, i), w-2))
		}
		if !m.coverPlain {
			block = append(block, "")
		}
	}
	if !m.coverPlain || rows == 0 {
		block = append(block, m.coverSongText(w-2)...)
	}
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
	text := coverTextRows + 1 // the song block and a gap under the cover
	if m.coverPlain {
		text = 0 // the song is in the right column
	}
	rows = min(h-text, 24)
	if m.coverPlain {
		rows = min(h-2, 30)
	}
	cols = int(math.Round(float64(rows) * aspect))
	for rows > 2 && cols > w-6 {
		rows--
		cols = int(math.Round(float64(rows) * aspect))
	}
	block := text
	if rows < 4 {
		rows, cols = 0, 0
		block = coverTextRows
	} else {
		block += rows
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
	if rows < 14 {
		// No cover, or a small one (a low window): the lyrics take the
		// whole column, not a strip as low as the cover.
		return 0, h
	}
	return top, rows
}

// coverSongRight is the right column with the lyrics hidden: the song,
// its controls and volume, centred.
func (m *Model) coverSongRight(w, h int) []string {
	textW := min(w-4, 90)
	// Beside the cover alone the words, the lines and the buttons are
	// drawn large, where they are drawn and there is the room: at their
	// usual size they were small beside it.
	m.songBig = m.lyricsPicOn() && m.playerState.Track != nil && h >= 18 && w >= 40
	block := m.coverSongText(textW)
	if m.songBig && m.lines != nil {
		m.lines.songBig = true // the lines and buttons of this screen (see linesPlace)
	}
	m.songBig = false
	pad := strings.Repeat(" ", max(0, (w-textW)/2))
	out := make([]string, h)
	top := max(0, (h-len(block))/2)
	for i, l := range block {
		if top+i < h {
			out[top+i] = pad + l
		}
	}
	return out
}

// coverIdleRight fills the lyrics side while nothing plays: the start-up
// error if the service failed, else where to find music.
func (m *Model) coverIdleRight(w, h int) []string {
	muted := styles.QueueItemMuted
	accent := styles.KeyName
	var rows []string
	if m.engineErr != "" {
		wrapped := lipgloss.NewStyle().Width(max(20, w-8)).Render(m.engineErr)
		for r := range strings.SplitSeq(wrapped, "\n") {
			rows = append(rows, styles.ErrorStyle.Render(strings.TrimRight(r, " ")))
		}
	} else {
		searchHint := "  search"
		if m.serviceKey == "radio" {
			searchHint = "  link"
		}
		rows = []string{
			styles.Header.Render(i18n.T("Nothing playing yet")),
			"",
			accent.Render("/") + muted.Render(i18n.T(searchHint)),
			accent.Render("l") + muted.Render(i18n.T("  library")),
			accent.Render("F") + muted.Render(i18n.T("  feed")),
			accent.Render("h") + muted.Render(i18n.T("  history")),
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
	if !m.coverPanelFramed() && m.coverPlain {
		return m.coverSongRight(w, h)
	}
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
		if m.lyricsPicOn() {
			return make([]string, h) // the lyrics are a picture: see linesLyricsBack
		}
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

// coverStatusLines is the cover view's status rows. Every panel has hints
// of its own, which wrap to a different number of rows; were the rows not
// held to one number, the cover and the song would jump (and the cover come
// and go) on every esc, q or step into the queue. So the status always
// takes as many rows as its tallest set of hints, the shorter ones padded
// with blank rows on top.
func (m *Model) coverStatusLines() []string {
	w := m.width - 4
	lines := m.statusLinesFull(w)
	for rows := m.coverStatusRows(w, len(lines)); len(lines) < rows; {
		lines = append([]string{""}, lines...)
	}
	return m.hintsSlide(lines, w)
}

// coverStatusRows is the most rows the status takes at width w: every
// panel's hints are measured once for a width, and after that the number
// only grows (now is what it takes at the moment).
func (m *Model) coverStatusRows(w, now int) int {
	if m.statusW != w {
		m.statusW, m.statusRows = w, 0
		panel, queueOpen, cur, mode := m.activePanel, m.coverQueueOpen, m.queueMiniCursor, m.mode
		m.mode = modeNormal
		for p := -1; p < len(m.panels); p++ {
			m.activePanel = p
			for _, c := range []int{-1, 0} {
				if p >= 0 && c >= 0 {
					continue // the queue's cursor is only there with no panel open
				}
				m.queueMiniCursor = c
				for _, q := range []bool{false, true} {
					m.coverQueueOpen = q
					m.statusRows = max(m.statusRows, len(m.statusLinesFull(w)))
				}
			}
		}
		m.activePanel, m.coverQueueOpen, m.queueMiniCursor, m.mode = panel, queueOpen, cur, mode
	}
	m.statusRows = max(m.statusRows, now)
	return m.statusRows
}

// renderCoverLayout renders the whole cover view.
func (m *Model) renderCoverLayout() string {
	leftW, rightW, h := m.coverDims()
	status := m.coverStatusLines()
	full := max(6, m.height-len(status))
	top, _ := coverPads(full)
	rows := make([]string, top)
	if leftW > 0 {
		left := m.coverLeftLines(leftW, h)
		right := m.coverRightLines(rightW, h)
		m.linesLyricsBack(top, leftW+1, rightW, h)
		if m.lines != nil {
			m.lines.coverRow += top // it was noted from the column's own top
		}
		for i := range h {
			rows = append(rows, padRight(left[i], leftW)+" "+padRight(safeIdx(right, i), rightW))
		}
	} else {
		// In narrow windows put the cover beside the song details, keeping
		// the progress and controls across the full width below them.
		artRows, cols := 0, 0
		if m.coverWanted() && h >= 6 && m.width >= 28 {
			artRows = min(6, h-3)
			aspect := m.artCellAsp
			if aspect <= 0 {
				aspect = 2
			}
			cols = min(m.width-20, int(math.Round(float64(artRows)*aspect)))
		}
		labelStart := 0
		if m.lines != nil {
			labelStart = len(m.lines.labels)
		}
		song := m.coverSongText(m.width - 2)
		if artRows > 0 && len(song) >= 4 {
			textW := max(1, m.width-cols-5)
			if m.lines != nil {
				for i := labelStart; i < len(m.lines.labels); i++ {
					lb := &m.lines.labels[i]
					if lb.kind == labelTitle || lb.kind == labelArtist || lb.kind == labelAlbum {
						lb.cells, lb.align = textW, alignLeft
					}
				}
			}
			headerRows := max(artRows, 3)
			header := make([]string, headerRows)
			textTop := (headerRows - 3) / 2
			for i := range 3 {
				header[textTop+i] = ansi.Truncate(strings.TrimSpace(song[i+1]), textW, "…")
			}
			song = append(header, song[4:]...)
		}
		home := m.mode == modeNormal && !m.debugView && !m.coverQueueOpen &&
			(m.activePanel < 0 || m.panels[m.activePanel] == m.lyricsP && !m.lyricsP.m.PickerOpen())
		if home {
			rows = append(rows, make([]string, max(0, full-len(rows)-len(song)-2))...)
		}
		if artRows > 0 {
			const col = 3
			var cover []string
			if m.linesCoverAt(col, len(rows), cols, artRows) {
				cover = make([]string, artRows)
			} else {
				cover = m.coverArtLines(art.Size{Width: cols, Height: artRows})
			}
			for i := range artRows {
				song[i] = "  " + padRight(safeIdx(cover, i), cols) + "  " + song[i]
			}
		}
		for _, l := range song {
			rows = append(rows, " "+l)
		}
		rows = append(rows, m.coverRightLines(rightW, h-artRows)...)
	}
	for len(rows) < full {
		rows = append(rows, "")
	}
	var sb strings.Builder
	for _, r := range rows[:full] {
		sb.WriteString(padRight(r, m.width) + "\n")
	}
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

// coverHome goes to the cover view's home screen: the lyrics, or with them
// hidden the song beside the cover.
func (m *Model) coverHome() tea.Cmd {
	m.coverQueueOpen = false
	if m.coverPlain {
		m.activePanel = -1
		return nil
	}
	return m.openLyricsPanel()
}

// toggleCoverView switches between the boxed layout and the cover view and
// remembers the choice (for every tab).
func (m *Model) toggleCoverView() tea.Cmd {
	m.cfg.Layout = ""
	if !m.coverView {
		m.cfg.Layout = LayoutCover
	}
	m.saveLayout()
	return m.followLayout()
}

// toggleCoverLyrics shows or hides the lyrics in the cover view and
// remembers the choice.
func (m *Model) toggleCoverLyrics() tea.Cmd {
	m.cfg.CoverHideLyrics = !m.coverPlain
	m.saveLayout()
	return m.followLayout()
}

func (m *Model) saveLayout() {
	if err := m.cfg.Save(""); err != nil {
		m.appendLog(fmt.Sprintf("[layout] config save error: %v", err))
	}
}

// followLayout brings this tab to the layout in the config, which every
// tab shares: v or y pressed here or in another tab.
func (m *Model) followLayout() tea.Cmd {
	cover, plain := m.cfg.Layout == LayoutCover, m.cfg.CoverHideLyrics
	if cover == m.coverView && plain == m.coverPlain {
		return nil
	}
	wasCover := m.coverView
	m.coverView, m.coverPlain = cover, plain
	if !cover {
		m.lyricsP.m.SetBare(false)
		m.coverQueueOpen = false
		if m.activePanel == m.lyricsPanelIdx() {
			m.activePanel = -1
		}
		return nil
	}
	var cmds []tea.Cmd
	if !m.settingsOpen() {
		// (Changed in the settings, the settings stay open: the next key
		// is still theirs.)
		cmds = append(cmds, m.coverHome())
	}
	if t := m.playerState.Track; !wasCover && t != nil && t.ArtworkURL != "" {
		cmds = append(cmds, m.fetchArtworkCmd(t.ArtworkURL, m.artworkGen))
	}
	return tea.Batch(cmds...)
}

// settingsOpen reports whether the settings panel is the one open.
func (m *Model) settingsOpen() bool {
	return m.activePanel >= 0 && m.activePanel < len(m.panels) && m.panels[m.activePanel] == m.settingsP
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
	searchHint := " search"
	if m.serviceKey == "radio" {
		searchHint = " link"
	}
	parts := []string{
		styles.ModeNormal.Render("COVER"),
		hint("/", searchHint),
		hint(i18n.T("l"), " library"),
		hint(i18n.T("h"), " history"),
		hint(i18n.T("F"), " feed"),
		hint(i18n.T("q"), " queue"),
		hint(i18n.T("e"), " equalizer"),
		hint(i18n.T("i"), " song menu"),
		hint("P", " add to playlist"),
	}
	if m.coverPlain {
		parts = append(parts, hint(i18n.T("y"), " lyrics"))
	} else {
		parts = append(parts, hint(i18n.T("y"), " hide lyrics"))
	}
	if m.playerState.Track != nil && !m.coverPlain {
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
		hint("V", " visualizer"),
		hint(":", " command"),
		hint(",", " settings"),
		hint("H", " hide hints"),
		hint("?", " about"),
		hint(i18n.T(":q"), " quit"),
	)
}

// linesLyricsBack tells the interface's lines where the bare lyrics are on
// the screen being laid out (the right column starts at the cell (col,
// row) and is w×h), for the ribbons that run behind them.
func (m *Model) linesLyricsBack(row, col, w, h int) {
	if m.coverPanelFramed() || m.coverPlain || m.playerState.Track == nil {
		return
	}
	top, bandH := m.coverLyricsBand(h)
	// As wide as the words (see coverRightLines), and a little more.
	textW := min(w-6, 56)
	if m.lyricsPicOn() {
		// Drawn large, they get a wider column and all of its height,
		// like a fullscreen player's.
		textW = min(w-6, 96)
		top, bandH = 0, h
		m.linesLyricsAt(col+4, row, textW, h)
	}
	m.linesBackAt(col+2, row+top, min(w-2, textW+4), min(bandH, h-top))
}

// coverIdleButtons is the row of the player's buttons while nothing plays:
// the characters, greyed, or the mark of the picture they are drawn as.
func (m *Model) coverIdleButtons(w int) string {
	if m.pixButtons() && w >= linesCtlCols {
		return centerStr(string(linesCtlMark)+strings.Repeat(" ", linesCtlCols-1), w)
	}
	return centerStr(styles.QueueItemMuted.Render("↺   ⇄   ▶   ♡"), w)
}
