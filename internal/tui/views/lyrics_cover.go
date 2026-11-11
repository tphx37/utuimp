package views

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// Cover-view lyrics, laid out like a fullscreen player's: the current line
// sits about a third of the way down, the lines already sung fade into the
// background as they rise, the coming ones wait in grey and dim towards the
// bottom edge. Lines are word-wrapped and spaced apart, and a line change
// slides the text up instead of jumping.

// coverAnchor is where the current line starts, as a share of the height.
const coverAnchor = 0.3

// coverSlide is how long a line change's colours take: the line just sung
// lets go of its highlight while the new one lights up. The text itself
// moves up once, at the change: a terminal can only move it by whole rows,
// and stepping row by row (a line plus the gap) reads as jitter.
const coverSlide = 450 * time.Millisecond

// easeInOut is a smoothstep: slow start, slow finish.
func easeInOut(t float64) float64 { return t * t * (3 - 2*t) }

// wrapRanges word-wraps text to width w, returning each row as a rune range
// (spaces at the breaks are dropped).
func wrapRanges(text string, w int) [][2]int {
	runes := []rune(text)
	if w <= 0 || len(runes) == 0 {
		return [][2]int{{0, len(runes)}}
	}
	var out [][2]int
	start := 0
	for start < len(runes) {
		for start < len(runes) && runes[start] == ' ' {
			start++
		}
		if start >= len(runes) {
			break
		}
		end, width, lastSpace := start, 0, -1
		for end < len(runes) {
			cw := ansi.StringWidth(string(runes[end]))
			if width+cw > w {
				break
			}
			if runes[end] == ' ' {
				lastSpace = end
			}
			width += cw
			end++
		}
		if end < len(runes) && lastSpace > start {
			end = lastSpace
		}
		if end == start { // a single character wider than w
			end++
		}
		out = append(out, [2]int{start, end})
		start = end
	}
	if len(out) == 0 {
		out = [][2]int{{0, 0}}
	}
	return out
}

// coverRow is one row of a lyric line: its text, the line it belongs to
// (-1 for the gap between lines), where it starts within the line, and
// whether it is the line's translation.
type coverRow struct {
	text        string
	line        int
	from        int
	translation bool
}

// coverBlock renders lyric line i as its rows (wrapped text, then its
// translation), unstyled except for the active line.
func (l *LyricsModel) coverBlock(i, w int) []coverRow {
	line := l.lines[i]
	text := lineText(line)
	var rows []coverRow
	for _, r := range wrapRanges(text, w) {
		rows = append(rows, coverRow{text: string([]rune(text)[r[0]:r[1]]), line: i, from: r[0]})
	}
	if tr := l.translationFor(i); tr != "" && strings.TrimSpace(text) != "" {
		for _, r := range wrapRanges(tr, w) {
			rows = append(rows, coverRow{text: string([]rune(tr)[r[0]:r[1]]), line: i, translation: true})
		}
	}
	return rows
}

// CoverView renders the lyrics for the cover view in exactly h rows of
// width w.
func (l *LyricsModel) CoverView(w, h int) []string {
	out := make([]string, h)
	if h <= 0 || w <= 0 {
		return out
	}
	anchor := max(0, int(float64(h)*coverAnchor))
	muted := styles.QueueItemMuted
	message := func(lines ...string) []string {
		for i, s := range lines {
			if anchor+i < h {
				out[anchor+i] = s
			}
		}
		return out
	}
	switch {
	case l.loading:
		return message(muted.Render(i18n.T("fetching lyrics…")))
	case l.errMsg != "", len(l.lines) == 0:
		msg := i18n.T("no lyrics found")
		if l.errMsg != "" {
			msg = i18n.T("You cannot sing this song :(")
		}
		return message(muted.Render(msg), "",
			styles.KeyName.Render(i18n.T("c"))+muted.Render(i18n.T(" choose another source")))
	}

	// The line the view is built around: the sung one while following, the
	// picked one while browsing, the top one for unsynced lyrics.
	focus := l.scroll
	switch {
	case !l.follow && l.cursor >= 0:
		focus = l.cursor
	case l.synced:
		focus = max(l.currentIdx, 0)
	}
	focus = min(max(focus, 0), len(l.lines)-1)
	if !l.synced && l.follow {
		anchor = 0 // nothing to place: read from the top
	}

	// The last row reports what the lyrics are doing (translating…).
	if st := l.coverStatus(); st != "" && h > anchor+2 {
		out[h-1] = st
		h--
	}
	anchorF := float64(anchor)

	// Rows above the anchor, walking back from the focus line.
	type placed struct {
		coverRow
		dist int // lines away from the focus: <0 above, >0 below
	}
	var above []placed
	for i := focus - 1; i >= 0 && len(above) < anchor; i-- {
		block := l.coverBlock(i, w)
		rows := append(block, coverRow{line: -1})
		for j := len(rows) - 1; j >= 0; j-- {
			above = append(above, placed{rows[j], i - focus})
		}
	}
	var below []placed
	for i := focus; i < len(l.lines) && len(below) < h-anchor; i++ {
		for _, r := range l.coverBlock(i, w) {
			below = append(below, placed{r, i - focus})
		}
		below = append(below, placed{coverRow{line: -1}, i - focus})
	}

	for k, p := range above {
		row := anchor - 1 - k
		if row < 0 {
			break
		}
		// 0 at the anchor … 1 at the top edge, measured from the unrounded
		// anchor so the fade glides while the rows step.
		fade := min(max((anchorF-float64(row))/(anchorF+1), 0), 1)
		out[row] = l.coverStyle(p.coverRow, p.dist, fade, false)
	}
	for k, p := range below {
		row := anchor + k
		if row >= h {
			break
		}
		// The last quarter dims towards the bottom edge.
		depth := 0.0
		if q := h - h/4; row >= q {
			depth = float64(row-q+1) / float64(h-q+1) * 0.6
		}
		out[row] = l.coverStyle(p.coverRow, p.dist, depth, p.dist > 0 || !l.synced)
	}
	return out
}

// coverStyle colours one row. fade is 0 (full) … 1 (gone into the
// background); waiting marks lines not sung yet.
func (l *LyricsModel) coverStyle(r coverRow, dist int, fade float64, waiting bool) string {
	if r.line < 0 || r.text == "" {
		return ""
	}
	translation, text := r.translation, r.text
	dimC, bgC, rgb := styles.LyricFadeColors()

	// The line just sung lets go of its highlight gradually.
	if rgb && !translation && l.follow && l.synced && r.line == l.prevIdx && r.line != l.currentIdx {
		if p := easeInOut(l.fade(coverSlide)); p < 1 {
			sungC, _, _ := styles.LyricColors()
			rest := styles.LerpColor(dimC, bgC, 0.15+0.85*fade)
			return lipgloss.NewStyle().Bold(p < 0.5).Foreground(styles.LerpColor(sungC, rest, p)).Render(text)
		}
	}

	// The focus line.
	if dist == 0 && !translation {
		switch {
		case !l.follow && l.cursor == r.line && r.line != l.currentIdx:
			return styles.Selected.Render(text)
		case l.synced && r.line == l.currentIdx:
			line := l.lines[r.line]
			lineEnd := time.Duration(0)
			if r.line+1 < len(l.lines) {
				lineEnd = l.lines[r.line+1].Start
			}
			return l.renderActiveRange(line, lineEnd, r.from, r.from+len([]rune(text)))
		}
	}
	if !l.synced && !waiting {
		waiting = true
	}
	if !rgb {
		switch {
		case fade >= 0.7:
			return ""
		case fade >= 0.35 || translation:
			return lipgloss.NewStyle().Faint(true).Render(text)
		default:
			return styles.QueueItemMuted.Render(text)
		}
	}
	c := dimC
	switch {
	case !waiting:
		// Sung lines: a touch dimmer than the coming ones, then into the
		// background with the distance.
		c = styles.LerpColor(dimC, bgC, 0.15+0.85*fade)
	case fade > 0:
		c = styles.LerpColor(dimC, bgC, fade)
	}
	if dist == 0 && translation {
		c = styles.LerpColor(dimC, lipgloss.Color("#ffffff"), 0.15)
	}
	st := lipgloss.NewStyle().Foreground(c)
	if translation {
		st = st.Italic(true)
	}
	return st.Render(text)
}

// Timing of the cover view's "no translation" note: shown, then faded out.
const (
	noteShow = 3 * time.Second
	noteFade = time.Second
)

// coverStatus is a short note on the lyrics' state for the cover view, which
// has no header, worded as the boxed view's header: "translating…" while it
// lasts, "no translation" for a few seconds before it fades away.
func (l *LyricsModel) coverStatus() string {
	muted := styles.QueueItemMuted
	switch {
	case l.showTranslation && l.translating:
		return muted.Render(i18n.T("translating…"))
	case l.showTranslation && l.translateErr != "":
		age := l.now().Sub(l.translateErrAt)
		if age >= noteShow+noteFade {
			return ""
		}
		text := i18n.T("no translation")
		if age <= noteShow {
			return muted.Render(text)
		}
		dimC, bgC, ok := styles.LyricFadeColors()
		if !ok {
			return lipgloss.NewStyle().Faint(true).Render(text)
		}
		t := float64(age-noteShow) / float64(noteFade)
		return lipgloss.NewStyle().Foreground(styles.LerpColor(dimC, bgC, easeInOut(t))).Render(text)
	}
	return ""
}
