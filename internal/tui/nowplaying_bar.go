package tui

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/tui/views"
)

// The boxed view's Now Playing panel where the interface is drawn in
// pictures (see lines.go): laid out as a player's bar, not as centred rows
// of text. The cover stands small on the left with the song beside it, the
// buttons are in the middle, the volume is on the right, and the progress
// line runs under them from the cover to the panel's edge:
//
//	┌──────┐  Title                    ↺  ⇄  (▶)  ♡          🔈 ───●── 80%
//	│cover │  Artist
//	│      │  Album
//	└──────┘  0:39 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━────────────────────── 4:41
//
// In the simple style, and in a panel too narrow or too low for it, the
// panel is what it always was.

const (
	barCoverRows = 7
	barMinW      = 96
	barVolumeW   = 30
)

// nowPlayingBarOn reports whether the panel is laid out as the bar, in a
// panel contentW wide and h high.
func (m *Model) nowPlayingBarOn(contentW, h int) bool {
	return m.playerState.Track != nil && contentW >= barMinW && h >= barCoverRows && !m.artModeActive() && m.lyricsPicOn()
}

// nowPlayingBar renders the panel as the bar.
func (m *Model) nowPlayingBar(contentW, h int) []string {
	t := m.playerState.Track
	lines := make([]string, h)
	top := (h - barCoverRows) / 2

	// The cover, when the song has one: a picture, its place told by a mark.
	coverW := 0
	if m.pixCover() && m.artwork.img != nil && m.artwork.url == t.ArtworkURL {
		asp := m.artCellAsp
		if asp <= 0 {
			asp = 2
		}
		cols := int(math.Round(barCoverRows * asp))
		if m.lines.coverSize(cols, barCoverRows) {
			coverW = cols + 3
			lines[top] = string(linesCoverMark)
		}
	}
	rest := contentW - coverW
	leftW := rest - linesCtlCols - barVolumeW - 4
	if leftW < 20 {
		leftW = rest
	}
	row := func(n int, left, mid, right string) {
		s := padRight(lines[top+n], coverW) + padRight(left, leftW)
		if leftW < rest {
			s += "  " + padRight(mid, linesCtlCols) + "  " + padRight(right, barVolumeW)
		}
		lines[top+n] = s
	}
	words := func(kind int, text string) string {
		s, _ := m.label(kind, text, leftW, alignLeft)
		return s
	}
	album := strings.TrimSpace(ansi.Strip(m.renderAlbumMetaLine(t, leftW, true, false)))
	buttons := string(linesCtlMark) + strings.Repeat(" ", linesCtlCols-1)
	if !m.pixButtons() {
		buttons = centerStr(m.controlsText(t), linesCtlCols)
	}
	row(1, words(labelTitle, t.Title), "", "")
	row(2, words(labelArtist, t.Artist), buttons, m.coverVolumeLine(barVolumeW))
	row(3, words(labelAlbum, album), "", "")

	// The progress line, from the cover to the panel's edge.
	pos := m.smoothPosition()
	elapsed, total := views.FormatDuration(pos), views.FormatDuration(t.Duration)
	elapsedS, _ := m.label(labelTime, elapsed, lipgloss.Width(elapsed), alignLeft)
	totalS, _ := m.label(labelTime, total, lipgloss.Width(total), alignRight)
	barW := max(6, rest-lipgloss.Width(elapsed)-lipgloss.Width(total)-4-2) // a little clear of the panel's edge
	lines[top+5] = padRight(lines[top+5], coverW) + elapsedS + "  " + views.RenderProgressBar(pos, t.Duration, barW) + "  " + totalS

	// What the player has to say (an error, a note) on the last row.
	if st := m.statusLine(contentW); strings.TrimSpace(ansi.Strip(st)) != "" {
		lines[h-1] = st
	}
	return lines
}
