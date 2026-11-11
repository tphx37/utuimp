package tui

import (
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// The interface's only animation is things coming up softly; nothing is
// ever moved. The screen is drawn as usual, read back into cells (see
// parseCells), and:
//
//   - a panel that opens: its text comes up out of the background, row
//     after row, and nothing outside it is touched;
//   - a new song: the song's text (the Now Playing box; in the cover view
//     the column with the cover) comes up left to right, from the very
//     frame it is shown.
//
// Fading is only ever towards the background: no colour is added. Where
// the terminal's own colours are used and not known (see
// styles.QueryPalette) text is hidden, then faint, then itself instead.
// config.UIMotion turns it off; on the Linux console there is none.

const (
	fxFrame    = 33 * time.Millisecond
	fxTrackFor = 0.6 // seconds
	fxWipeFor  = 0.45
)

type fxTickMsg struct{ gen int }

func fxTick(gen int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return fxTickMsg{gen} })
}

type uiFX struct {
	ticking bool
	gen     int
	last    time.Time

	trackAt, wipeAt time.Time
	sig             string

	cells [][]uiCell
}

// fxLevel is how strong the effects are (0 = none).
func (m *Model) fxLevel() float64 {
	if m.noAnim || styles.Console || m.background || m.cfg == nil {
		return 0
	}
	switch m.cfg.Motion() {
	case config.MotionCalm:
		return 0.55
	case config.MotionLively:
		return 1
	case config.MotionWild:
		return 1.6
	}
	return 0
}

func (m *Model) fxState() *uiFX {
	if m.fx == nil {
		m.fx = &uiFX{}
	}
	return m.fx
}

func fxAge(t, now time.Time) float64 {
	if t.IsZero() {
		return 1e9
	}
	return now.Sub(t).Seconds()
}

// fxKick starts a panel's reveal (a tab was switched to) and the frames
// that draw it.
func (m *Model) fxKick() tea.Cmd {
	if m.fxLevel() == 0 {
		return nil
	}
	m.fxState().wipeAt = time.Now()
	return m.fxEnsure()
}

// fxBusy reports whether anything is animating at now.
func (m *Model) fxBusy(now time.Time) bool {
	f := m.fx
	if f == nil {
		return false
	}
	return fxAge(f.trackAt, now) < fxTrackFor || fxAge(f.wipeAt, now) < fxWipeFor
}

// fxEnsure notices what changed since the last message (a new song, another
// panel) and starts the frame clock when something has
// to be drawn. Update calls it after every message.
func (m *Model) fxEnsure() tea.Cmd {
	if m.fxLevel() == 0 {
		return nil
	}
	f := m.fxState()
	now := time.Now()
	if m.introStep == introDone && !m.vizOpen() {
		// A small window over the screen (the song menu, the playlist
		// picker, a name prompt) doesn't count: what is behind it must not
		// come up again when it opens or closes.
		mode := m.mode
		if mode == modePlaylistPicker {
			mode = m.playlistPickerReturnMode
		}
		sig := strings.Join([]string{
			strconv.Itoa(m.activePanel), strconv.Itoa(int(mode)), boolS(m.coverView), boolS(m.debugView),
		}, ",")
		if f.sig != "" && sig != f.sig {
			f.wipeAt = now
		}
		f.sig = sig
	}
	if m.trackChangedAt.After(f.trackAt) {
		f.trackAt = m.trackChangedAt
	}
	if f.ticking || !m.fxBusy(now) || m.vizOpen() || m.introStep != introDone {
		return nil
	}
	f.ticking = true
	f.gen++
	f.last = now
	return fxTick(f.gen, fxFrame)
}

// fxFrameMsg handles a tick of the frame clock: the next one, or none
// when everything has settled.
func (m *Model) fxFrameMsg(msg fxTickMsg) tea.Cmd {
	f := m.fx
	if f == nil || msg.gen != f.gen || !f.ticking {
		return nil
	}
	now := time.Now()
	if m.fxLevel() == 0 || !m.fxBusy(now) || m.vizOpen() {
		f.ticking = false
		return nil
	}
	m.fxStep(now)
	return fxTick(f.gen, fxFrame)
}

// fxStep moves the clock on.
func (m *Model) fxStep(now time.Time) { m.fx.last = now }

// fxRect is a part of the screen: columns [x0, x1), rows [y0, y1).
type fxRect struct{ x0, y0, x1, y1 int }

func (r fxRect) clip(w, h int) fxRect {
	return fxRect{max(r.x0, 0), max(r.y0, 0), min(r.x1, w), min(r.y1, h)}
}

func (r fxRect) empty() bool { return r.x1 <= r.x0 || r.y1 <= r.y0 }

// fxPanel is the inside of the first frame (╭ … ╮ down to ╰) whose top is
// at row from or below and starts at column fromX or to its right.
func fxPanel(cells [][]uiCell, from, fromX int) (fxRect, bool) {
	for y := max(from, 0); y < len(cells); y++ {
		row := cells[y]
		for x := max(fromX, 0); x < len(row); x++ {
			if row[x].ch != "╭" {
				continue
			}
			x1 := -1
			for i := x + 1; i < len(row); i++ {
				if row[i].ch == "╮" {
					x1 = i
					break
				}
			}
			if x1 < 0 {
				continue
			}
			for r := y + 1; r < len(cells); r++ {
				if cells[r][x].ch == "╰" {
					return fxRect{x + 1, y + 1, x1, r}, true
				}
			}
		}
	}
	return fxRect{}, false
}

// fxRects is where the song is written and where the panel is (ok is false
// with no panel to be seen: the cover view's bare lyrics).
func (m *Model) fxRects(cells [][]uiCell, w, h int) (song, panel fxRect, ok bool) {
	if !m.coverView {
		nph := m.nowPlayingHeight()
		song = fxRect{1, 1, w - 1, nph + 1}
		panel, ok = fxPanel(cells, nph+2, 0)
		return song.clip(w, h), panel, ok
	}
	leftW, _, ch := m.coverDims()
	top, _ := coverPads(max(6, m.height-len(m.coverStatusLines())))
	switch {
	case leftW == 0:
		song = fxRect{0, top, w, top + coverTextRows}
	case !m.coverPlain:
		song = fxRect{0, top, leftW, top + ch}
	case !m.coverPanelFramed():
		song = fxRect{leftW + 1, top, w, top + ch}
	}
	panel, ok = fxPanel(cells, 0, leftW) // not the cover's placeholder frame
	if !ok && leftW > 0 && !m.coverPlain {
		panel = fxRect{leftW + 1, top, w, top + ch} // the lyrics' side
	}
	return song.clip(w, h), panel.clip(w, h), ok
}

// fxFade shows k of a cell: 0 is the bare screen, 1 the cell as it is.
// Pictures (the cover) are left alone.
func fxFade(c *uiCell, k float64, screen [3]uint8) {
	if k >= 1 || isBlockRune(c.ch) && !c.bgDefault {
		return
	}
	if c.guess {
		// Colours that can't be mixed: nothing, then faint, then itself.
		switch {
		case k < 0.4:
			c.ch, c.wide, c.sgr = " ", false, ""
		case c.ch != "" && c.ch != " " && c.sgr == "":
			c.sgr = "2"
		case c.ch != "" && c.ch != " ":
			c.sgr += ";2"
		}
		return
	}
	if c.ch == "" || c.ch == " " && c.bgDefault {
		return
	}
	k = clamp01(k)
	if !c.bgDefault {
		c.bg = blend3(c.bg, screen, 1-k)
	}
	c.fg = blend3(c.fg, c.bg, 1-k)
	c.mod = true
}

// fxApply lights the rendered screen where something was just done;
// content is returned as it is when nothing is.
func (m *Model) fxApply(content string) string {
	amp := m.fxLevel()
	f := m.fx
	if amp == 0 || f == nil || !f.ticking || m.width <= 2 || m.height <= 0 {
		return content
	}
	now := time.Now()
	if !m.fxBusy(now) {
		return content
	}
	w, h := m.width, m.height
	fgc, bgc, _ := styles.ScreenColors()
	fg := [3]uint8{fgc.R, fgc.G, fgc.B}
	bg := [3]uint8{bgc.R, bgc.G, bgc.B}
	cells := parseCellsInto(f.cells, content, w, h, fg, bg)
	f.cells = cells
	song, panel, _ := m.fxRects(cells, w, h)

	// ── A panel opens: its text comes up row after row ──
	if a := fxAge(f.wipeAt, now); a < fxWipeFor && !panel.empty() {
		for y := panel.y0; y < panel.y1; y++ {
			for x := panel.x0; x < panel.x1; x++ {
				k := smooth(0, 1, a/fxWipeFor*1.7-0.7*float64(y-panel.y0)/float64(panel.y1-panel.y0)-0.06*math.Sin(float64(x)*0.12))
				fxFade(&cells[y][x], k, bg)
			}
		}
	}

	// ── A new song: its text comes up left to right ──
	if a := fxAge(f.trackAt, now); a < fxTrackFor && !song.empty() {
		for y := song.y0; y < song.y1; y++ {
			for x := song.x0; x < song.x1; x++ {
				k := smooth(0, 1, a/fxTrackFor*1.6-0.6*float64(x-song.x0)/float64(song.x1-song.x0)-0.05*math.Sin(float64(y)*0.9))
				fxFade(&cells[y][x], k, bg)
			}
		}
	}

	return encodeCells(cells, w, h)
}

// isBlockRune tells the block elements pictures are drawn with.
func isBlockRune(s string) bool {
	for _, r := range s {
		return r >= 0x2580 && r <= 0x259f || r >= 0x1fb00 && r <= 0x1fbaf // and the sextants
	}
	return false
}

func isBoxRune(s string) bool {
	for _, r := range s {
		return r >= 0x2500 && r <= 0x257f
	}
	return false
}

// encodeCells writes cells back as a frame: a cell as it was, or, where an
// effect touched it (mod), in its new colours.
func encodeCells(cells [][]uiCell, w, h int) string {
	var sb strings.Builder
	sb.Grow(w * h * 8)
	// What the last SGR sequence set: an unchanged cell's own parameters,
	// or a touched cell's colours.
	var (
		started  bool
		lastMod  bool
		lastSgr  string
		lastAttr string
		lastFg   [3]uint8
		lastBg   [3]uint8
		lastDef  bool
	)
	var num [3]byte
	writeRGB := func(kind string, c [3]uint8) {
		sb.WriteString(kind)
		for i, v := range c {
			if i > 0 {
				sb.WriteByte(';')
			}
			sb.Write(strconv.AppendUint(num[:0], uint64(v), 10))
		}
	}
	for y := range h {
		row := cells[y]
		for x := 0; x < w; x++ {
			c := &row[x]
			if c.mod {
				if !started || !lastMod || c.attr != lastAttr || c.fg != lastFg || c.bg != lastBg || c.bgDefault != lastDef {
					sb.WriteString("\x1b[0")
					if c.attr != "" {
						sb.WriteByte(';')
						sb.WriteString(c.attr)
					}
					writeRGB(";38;2;", c.fg)
					if !c.bgDefault {
						writeRGB(";48;2;", c.bg)
					}
					sb.WriteByte('m')
					started, lastMod, lastAttr, lastFg, lastBg, lastDef = true, true, c.attr, c.fg, c.bg, c.bgDefault
				}
			} else if !started || lastMod || c.sgr != lastSgr {
				sb.WriteString("\x1b[0")
				if c.sgr != "" {
					sb.WriteByte(';')
					sb.WriteString(c.sgr)
				}
				sb.WriteByte('m')
				started, lastMod, lastSgr = true, false, c.sgr
			}
			if c.ch == "" || c.wide && (x+1 >= w || row[x+1].ch != "") {
				sb.WriteByte(' ') // half of a wide character whose other half was cut off
			} else {
				sb.WriteString(c.ch)
				if x+1 < w && row[x+1].ch == "" {
					x++ // the wide character took the next cell too
				}
			}
		}
		sb.WriteString("\x1b[0m")
		started = false
		if y < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func boolS(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
