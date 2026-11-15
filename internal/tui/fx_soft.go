package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// Nothing on the screen changes at a stroke. Where the interface's lines
// are pictures (see lines.go) the text is made as soft as they are: words
// that appear come up out of the background, and words that go fade into
// it, each in a fraction of a second. That is the key hints at the bottom
// when the mode changes, a panel's contents, a title: any text.
//
// Words that stay but change colour (the chosen row of a list, a panel's
// title as it takes or loses focus) go from the one colour to the other,
// never through the background.
//
// Text that only moved is left alone (a list scrolled by a row or three is
// still the same rows, and must stay readable while it scrolls), and so
// are changes of a letter or two (the clock, a letter typed).

const (
	softIn  = 0.16 // seconds for new words to come up
	softOut = 0.10 // and for old ones to go
	// softMin is how many cells of a row must change for it to count as
	// new words.
	softMin = 4
	// softShift is how many rows text may have moved and still be the
	// same text.
	softShift = 3
	// softTint is how long a letter takes to go from one colour to another.
	softTint = 0.18
)

// softText is what the soft changes remember of the last frame.
type softText struct {
	w, h  int
	start time.Time
	was   [][]uiCell // the last frame, as it was meant to look
	rows  []string   // its rows' letters
	born  [][]float32
	gone  [][]float32
	ord   [][]float32 // each changed cell's place in its row's sweep, 0..1
	ghost [][]uiCell  // what a cell showed before it went blank
	tint  [][]float32 // when a cell's colour began to change (0: it isn't)
	from  [][]softTo  // the colours it is changing from
	cells [][]uiCell
	busy  bool // words are coming up or going right now
}

// softTo is a cell's colours as they were when its colour began to change
// (or as far as they had got, when it changed again on the way).
type softTo struct {
	fg, bg [3]uint8
}

// softSkip tells the characters that are not words: the lines' own (they
// become pictures) and the marks that tell where pictures go.
func softSkip(ch string) bool {
	for _, r := range ch {
		return r >= 0x2500 && r <= 0x257f || r == '▸' || r >= 0xe000 && r <= 0xf8ff
	}
	return true
}

// fxSoft makes the changes of the rendered screen soft; content is
// returned as it is when nothing is changing.
func (m *Model) fxSoft(content string) string {
	if m.fxLevel() == 0 || !m.linesOn() || m.width <= 2 || m.height <= 0 {
		m.soft = nil
		return content
	}
	w, h := m.width, m.height
	fgc, bgc, _ := styles.ScreenColors()
	fg := [3]uint8{fgc.R, fgc.G, fgc.B}
	bg := [3]uint8{bgc.R, bgc.G, bgc.B}
	s := m.soft
	fresh := s == nil || s.w != w || s.h != h
	if fresh {
		s = &softText{w: w, h: h, start: time.Now(), rows: make([]string, h)}
		grid := func() [][]uiCell {
			g := make([][]uiCell, h)
			for y := range g {
				g[y] = make([]uiCell, w)
			}
			return g
		}
		s.was, s.ghost = grid(), grid()
		s.born, s.gone, s.ord, s.tint = make([][]float32, h), make([][]float32, h), make([][]float32, h), make([][]float32, h)
		s.from = make([][]softTo, h)
		for y := range h {
			s.born[y], s.gone[y], s.ord[y], s.tint[y] = make([]float32, w), make([]float32, w), make([]float32, w), make([]float32, w)
			s.from[y] = make([]softTo, w)
		}
		m.soft = s
	}
	cells := parseCellsInto(s.cells, content, w, h, fg, bg)
	s.cells = cells
	now := float32(time.Since(s.start).Seconds()) + 1 // never 0: that means "not changing"
	if m.keyHeld(time.Now()) {
		// A key held down (a list run through): the text is as it is, at
		// once, readable all the way, with nothing of it left to come up
		// or change colour once the key is let go. (Fading the words in
		// as they came showed bits of letters of rows long gone, and the
		// colours caught up all at once at the end.)
		for y := range h {
			clear(s.born[y])
			clear(s.gone[y])
			clear(s.tint[y])
			copy(s.was[y], cells[y])
		}
		var sb strings.Builder
		for y := range h {
			sb.Reset()
			for x := range cells[y] {
				sb.WriteString(cells[y][x].ch)
			}
			s.rows[y] = sb.String()
		}
		s.busy = false
		return content
	}

	var sb strings.Builder
	rows := make([]string, h)
	for y := range h {
		sb.Reset()
		for x := range cells[y] {
			sb.WriteString(cells[y][x].ch)
		}
		rows[y] = sb.String()
	}
	for y := 0; y < h && !fresh; y++ {
		if rows[y] == s.rows[y] {
			continue
		}
		if !m.coverView && m.activePanel < 0 && m.playerState.Track != nil &&
			m.isNowPlayingMetadataRow(rows[y]) {
			// The boxed player's metadata should change as one stable
			// block. The per-cell sweep makes a new title briefly dark,
			// then bright again, which reads as a flicker.
			clear(s.born[y])
			clear(s.gone[y])
			clear(s.tint[y])
			continue
		}
		if y >= h-m.hintsRowsNow {
			// The key hints sliding down or up: whole rows that move, as
			// they are, not words coming and going.
			for x := range cells[y] {
				s.born[y][x], s.gone[y][x] = 0, 0
			}

			continue
		}
		moved := false
		for d := 1; d <= softShift && !moved; d++ {
			moved = y-d >= 0 && rows[y] == s.rows[y-d] || y+d < h && rows[y] == s.rows[y+d]
		}
		if strings.TrimSpace(rows[y]) == "" {
			// A row that emptied: its words go softly, unless they went
			// to a row near it (the key hints sliding down, a list
			// scrolled): then they are there, and a ghost of them here
			// would show them twice.
			moved = false
			for d := 1; d <= softShift && !moved; d++ {
				moved = y+d < h && rows[y+d] == s.rows[y] || y-d >= 0 && rows[y-d] == s.rows[y]
			}
		}
		changed := 0
		for x := range cells[y] {
			if cells[y][x].ch != s.was[y][x].ch {
				changed++
			}
		}
		if moved || changed < softMin {
			// The same words in another place, or a letter or two: as
			// they are, at once.
			for x := range cells[y] {
				s.born[y][x], s.gone[y][x] = 0, 0
			}
			continue
		}
		// The changed cells' place along the row: the sweep runs over them
		// left to right.
		xa, xb := -1, -1
		for x := range cells[y] {
			if cells[y][x].ch != s.was[y][x].ch {
				if xa < 0 {
					xa = x
				}
				xb = x
			}
		}
		for x := range cells[y] {
			c, old := &cells[y][x], &s.was[y][x]
			if c.ch == old.ch {
				continue
			}
			s.ord[y][x] = float32(x-xa) / float32(xb-xa+1)
			blank := c.ch == " " || c.ch == ""
			wasBlank := old.ch == " " || old.ch == ""
			switch {
			case !blank && !softSkip(c.ch):
				// Words already coming up go on from where they are: a
				// held key that changes them again and again must not
				// keep them at the start of their fade.
				if s.born[y][x] == 0 {
					s.born[y][x] = now
				}
				s.gone[y][x] = 0
			case blank && !wasBlank && !softSkip(old.ch) && c.bgDefault:
				s.ghost[y][x] = *old
				s.born[y][x], s.gone[y][x] = 0, now
			}
		}
	}
	// The same letter in another colour: it changes over, from the colour
	// it showed last (part way, if it was already changing).
	for y := 0; y < h && !fresh; y++ {
		for x := range cells[y] {
			c, old := &cells[y][x], &s.was[y][x]
			if c.ch != old.ch || c.ch == " " || c.ch == "" || softSkip(c.ch) || c.guess || old.guess {
				s.tint[y][x] = 0
				continue
			}
			bgSame := c.bgDefault == old.bgDefault && (c.bgDefault || c.bg == old.bg)
			bgBlend := !c.bgDefault && !old.bgDefault // a background comes or goes at once: no colour of the screen's to go through
			if c.fg == old.fg && bgSame {
				continue
			}
			if !bgSame && !bgBlend {
				s.tint[y][x] = 0
				continue
			}
			from := softTo{old.fg, old.bg}
			if t := s.tint[y][x]; t > 0 {
				from = s.shown(y, x, old, float64(now-t)) // already on its way
			}
			s.from[y][x], s.tint[y][x] = from, now
		}
	}
	for y := range h {
		copy(s.was[y], cells[y])
	}
	s.rows = rows

	touched := false
	for y := range h {
		for x := range cells[y] {
			if t := s.tint[y][x]; t > 0 {
				c := &cells[y][x]
				if age := float64(now - t); age < softTint && c.ch == s.was[y][x].ch {
					to := s.shown(y, x, c, age)
					c.fg, c.bg, c.mod = to.fg, to.bg, true
					touched = true
				} else {
					s.tint[y][x] = 0
				}
			}
			if b := s.born[y][x]; b > 0 {
				if age := float64(now - b); age < softIn {
					// Whole letters, in a sweep: not there yet, or there.
					if float64(s.ord[y][x]) > age/softIn {
						c := &cells[y][x]
						if c.bgDefault {
							c.ch, c.wide, c.sgr, c.mod = " ", false, "", false
						}
					}
					touched = true
				} else {
					s.born[y][x] = 0
				}
			}
			if g := s.gone[y][x]; g > 0 {
				c := &cells[y][x]
				if age := float64(now - g); age < softOut && (c.ch == " " || c.ch == "") {
					// The old words are swept away the same way.
					if float64(s.ord[y][x]) > age/softOut {
						*c = s.ghost[y][x]
						c.mod = false
					}
					touched = true
				} else {
					s.gone[y][x] = 0
				}
			}
		}
	}
	s.busy = touched
	if !touched {
		return content
	}
	return encodeCells(cells, w, h)
}

func (m *Model) isNowPlayingMetadataRow(row string) bool {
	plain := strings.TrimSpace(ansi.Strip(row))
	t := m.playerState.Track
	if plain == "" || t == nil {
		return false
	}
	for _, text := range []string{t.Title, t.Artist, t.Album} {
		if text != "" && strings.Contains(plain, text) {
			return true
		}
	}
	return false
}

// shown is the colours of the cell at (x, y) age seconds into its change
// towards c's.
func (s *softText) shown(y, x int, c *uiCell, age float64) softTo {
	k := smooth(0, softTint, age)
	f := s.from[y][x]
	out := softTo{blend3(f.fg, c.fg, k), c.bg}
	if !c.bgDefault {
		out.bg = blend3(f.bg, c.bg, k)
	}
	return out
}
