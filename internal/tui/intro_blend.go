package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tphx37/utuimp/internal/tui/styles"

	"github.com/charmbracelet/x/ansi"
)

// uiCell is one cell of a rendered frame of the interface, read back so the
// start screen can blend into it: the character and its SGR state, kept as
// written (to show the cell exactly as the interface would) and as RGB (to
// mix it with the ribbons).
type uiCell struct {
	ch        string // "" for the second half of a wide character
	sgr       string // the cell's SGR parameters, from a reset
	fg, bg    [3]uint8
	bgDefault bool   // the terminal's own background
	attr      string // the SGR parameters other than colour, faint and reverse (folded into fg and bg)
	guess     bool   // its colours are the terminal's own and not known: they can't be blended
	mod       bool   // coloured over by the interface's effects (see fxApply)
	wide      bool   // takes two cells (the next one has ch "")
}

// sgrState follows the SGR sequences of a frame.
type sgrState struct {
	fg, bg                                      string // "" is the default; else the parameters that set it
	bold, faint, italic, under, reverse, strike bool
}

// attrs is params without the colours, faintness and reverse video.
func (s *sgrState) attrs() string {
	var p []string
	for _, a := range []struct {
		on  bool
		par string
	}{{s.bold, "1"}, {s.italic, "3"}, {s.under, "4"}, {s.strike, "9"}} {
		if a.on {
			p = append(p, a.par)
		}
	}
	return strings.Join(p, ";")
}

func (s *sgrState) params() string {
	var p []string
	for _, a := range []struct {
		on  bool
		par string
	}{{s.bold, "1"}, {s.faint, "2"}, {s.italic, "3"}, {s.under, "4"}, {s.reverse, "7"}, {s.strike, "9"}} {
		if a.on {
			p = append(p, a.par)
		}
	}
	if s.fg != "" {
		p = append(p, s.fg)
	}
	if s.bg != "" {
		p = append(p, s.bg)
	}
	return strings.Join(p, ";")
}

// apply takes the parameters of one SGR sequence.
func (s *sgrState) apply(params string) {
	if params == "" {
		*s = sgrState{}
		return
	}
	toks := strings.Split(params, ";")
	for k := 0; k < len(toks); k++ {
		t := toks[k]
		if base, _, ok := strings.Cut(t, ":"); ok {
			switch base {
			case "38":
				s.fg = t
			case "48":
				s.bg = t
			case "4":
				s.under = !strings.HasSuffix(t, ":0")
			}
			continue
		}
		n, err := strconv.Atoi(t)
		if t == "" {
			n, err = 0, nil
		}
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			*s = sgrState{}
		case n == 1:
			s.bold = true
		case n == 2:
			s.faint = true
		case n == 3:
			s.italic = true
		case n == 4:
			s.under = true
		case n == 7:
			s.reverse = true
		case n == 9:
			s.strike = true
		case n == 22:
			s.bold, s.faint = false, false
		case n == 23:
			s.italic = false
		case n == 24:
			s.under = false
		case n == 27:
			s.reverse = false
		case n == 29:
			s.strike = false
		case n >= 30 && n <= 37, n >= 90 && n <= 97:
			s.fg = t
		case n == 39:
			s.fg = ""
		case n >= 40 && n <= 47, n >= 100 && n <= 107:
			s.bg = t
		case n == 49:
			s.bg = ""
		case n == 38 || n == 48:
			used := 0
			if k+1 < len(toks) {
				switch toks[k+1] {
				case "5":
					used = 2
				case "2":
					used = 4
				}
			}
			used = min(used, len(toks)-1-k)
			spec := strings.Join(toks[k:k+1+used], ";")
			if n == 38 {
				s.fg = spec
			} else {
				s.bg = spec
			}
			k += used
		}
	}
}

// specGuess reports whether an SGR colour spec names a colour of the
// terminal's own whose value is not known (defUnknown: the default one's
// isn't).
func specGuess(spec string, defUnknown bool) bool {
	if spec == "" {
		return styles.ANSIMode && defUnknown
	}
	if styles.PaletteKnown {
		return false
	}
	toks := strings.FieldsFunc(spec, func(r rune) bool { return r == ';' || r == ':' })
	n, _ := strconv.Atoi(toks[0])
	if n != 38 && n != 48 {
		return true // 30–37, 90–97 and the backgrounds
	}
	if len(toks) >= 3 && toks[1] == "5" {
		i, _ := strconv.Atoi(toks[2])
		return i < 16
	}
	return false
}

// specRGB is the colour an SGR colour spec stands for; ok is false for the
// default one.
func specRGB(spec string) (c [3]uint8, ok bool) {
	if spec == "" {
		return c, false
	}
	toks := strings.FieldsFunc(spec, func(r rune) bool { return r == ';' || r == ':' })
	n, _ := strconv.Atoi(toks[0])
	switch {
	case n >= 30 && n <= 37:
		return xtermRGB(n - 30), true
	case n >= 90 && n <= 97:
		return xtermRGB(n - 90 + 8), true
	case n >= 40 && n <= 47:
		return xtermRGB(n - 40), true
	case n >= 100 && n <= 107:
		return xtermRGB(n - 100 + 8), true
	}
	if len(toks) >= 3 && toks[1] == "5" {
		i, _ := strconv.Atoi(toks[2])
		return xtermRGB(i), true
	}
	if len(toks) >= 5 && toks[1] == "2" {
		rgb := toks[len(toks)-3:]
		for i := range 3 {
			x, _ := strconv.Atoi(rgb[i])
			c[i] = uint8(min(255, max(0, x))) //nolint:gosec // clamped
		}
		return c, true
	}
	return c, false
}

// xtermRGB is xterm's colour for a 256-colour index (the terminal's own 16
// may differ a little: these only tint the moment of the blend).
func xtermRGB(n int) [3]uint8 {
	base := [16][3]uint8{
		{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
		{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
	}
	switch {
	case n < 0:
		return base[0]
	case n < 16:
		if styles.PaletteKnown {
			return styles.Palette[n]
		}
		return base[n]
	case n < 232:
		n -= 16
		lv := func(v int) uint8 {
			if v == 0 {
				return 0
			}
			return uint8(55 + v*40) //nolint:gosec // at most 255
		}
		return [3]uint8{lv(n / 36), lv(n / 6 % 6), lv(n % 6)}
	case n < 256:
		g := uint8(8 + (n-232)*10) //nolint:gosec // at most 238
		return [3]uint8{g, g, g}
	}
	return base[7]
}

// parseCells reads a rendered frame back into w×h cells; fg and bg are
// the screen's default colours.
func parseCells(frame string, w, h int, fg, bg [3]uint8) [][]uiCell {
	return parseCellsInto(nil, frame, w, h, fg, bg)
}

// parseCellsInto is parseCells reusing buf's memory when it is the right
// size (an animation reads a frame back many times a second).
func parseCellsInto(buf [][]uiCell, frame string, w, h int, fg, bg [3]uint8) [][]uiCell {
	cells := buf
	if len(cells) != h || h > 0 && len(cells[0]) != w {
		cells = make([][]uiCell, h)
		for y := range cells {
			cells[y] = make([]uiCell, w)
		}
	}
	blank := uiCell{ch: " ", fg: fg, bg: bg, bgDefault: true}
	for y := range cells {
		for x := range cells[y] {
			cells[y][x] = blank
		}
	}
	var st sgrState
	// The cell the current SGR state makes, worked out again only when
	// the state changes (a frame has far fewer changes than cells).
	var proto uiCell
	dirty := true
	put := func(row, col int, ch string, width int) {
		if row >= h || col >= w {
			return
		}
		if dirty {
			dirty = false
			proto = uiCell{sgr: st.params(), attr: st.attrs(), fg: fg, bg: bg, bgDefault: true}
			proto.guess = specGuess(st.fg, styles.TermFg == nil) || specGuess(st.bg, styles.TermBg == nil)
			if rgb, ok := specRGB(st.fg); ok {
				proto.fg = rgb
			}
			if rgb, ok := specRGB(st.bg); ok {
				proto.bg, proto.bgDefault = rgb, false
			}
			if st.reverse {
				proto.fg, proto.bg, proto.bgDefault = proto.bg, proto.fg, false
			}
			if st.faint {
				proto.fg = blend3(proto.bg, proto.fg, 0.6)
			}
		}
		c := proto
		c.ch = ch
		c.wide = width > 1
		cells[row][col] = c
		for k := 1; k < width && col+k < w; k++ {
			cells[row][col+k] = uiCell{sgr: c.sgr, attr: c.attr, fg: c.fg, bg: c.bg, bgDefault: c.bgDefault}
		}
	}
	row, col := 0, 0
	for i := 0; i < len(frame); {
		b := frame[i]
		switch {
		case b == '\n':
			row, col = row+1, 0
			i++
		case b == '\r':
			col = 0
			i++
		case b == 0x1b && i+1 < len(frame) && frame[i+1] == '[':
			j := i + 2
			for j < len(frame) && (frame[j] < 0x40 || frame[j] > 0x7e) {
				j++
			}
			if j < len(frame) && frame[j] == 'm' {
				st.apply(frame[i+2 : j])
				dirty = true
			}
			i = j + 1
		case b == 0x1b && i+1 < len(frame) && frame[i+1] == ']':
			// OSC (a hyperlink): up to BEL or ST.
			j := i + 2
			for j < len(frame) && frame[j] != 0x07 && (frame[j] != 0x1b || j+1 >= len(frame) || frame[j+1] != '\\') {
				j++
			}
			if j < len(frame) && frame[j] == 0x07 {
				i = j + 1
			} else {
				i = j + 2
			}
		case b == 0x1b:
			i += 2
		case b >= 0x20 && b < 0x7f:
			// Plain ASCII: one cell, nothing to join.
			put(row, col, frame[i:i+1], 1)
			col++
			i++
		default:
			r, size := utf8.DecodeRuneInString(frame[i:])
			start := i
			i += size
			// Combining marks and variation selectors join the cell before.
			for i < len(frame) {
				r2, s2 := utf8.DecodeRuneInString(frame[i:])
				if r2 < 0x300 || ansi.StringWidth(string(r2)) != 0 || r2 == 0x1b {
					break
				}
				i += s2
			}
			// The cell's letter and what joined it: one piece of the frame
			// (no copy: this runs for every cell of every frame).
			ch := frame[start:i]
			width := 1
			if len(ch) != size || r >= 0x1100 && (r < 0x2500 || r > 0x25ff) {
				width = ansi.StringWidth(ch) // box drawing and blocks are narrow
			}
			if r < 0x20 {
				continue
			}
			put(row, col, ch, max(1, width))
			col += max(1, width)
		}
	}
	return cells
}
