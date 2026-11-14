package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/i18n"
)

// Turning the visualizer's picture into terminal cells.

// vizOverlay is the text laid over the picture: label on the second row,
// the esc hint on the last but one, each with how visible it is (0..1).
type vizOverlay struct {
	label, hint   string
	labelA, hintA float64
	texts         []vizText
}

// vizText is a line of text over the picture, centred on its row, in its
// own colours: one per letter (the last one for the rest), or light grey.
type vizText struct {
	row  int
	text string
	fg   [][3]uint8
}

func (v *visualizer) overlay(label string, now time.Time) vizOverlay {
	o := vizOverlay{label: label, labelA: 1, texts: v.texts}
	if now.Before(v.hintUntil) {
		o.hint = "esc  " + i18n.T("exit") + "   ·   tab  " + i18n.T("scene") +
			"   ·   [ ]  " + i18n.T("strength") + "   ·   { }  " + i18n.T("brightness") + "   ·   b  " + i18n.T("background")
		// Up once the picture has opened (its words were under it until
		// then), and away again at the end.
		o.hintA = clamp01(v.hintUntil.Sub(now).Seconds()/vizFadeFor.Seconds()) *
			smooth(vizOpenFor, vizOpenFor+0.35, now.Sub(v.openedAt).Seconds())
	}
	if !v.labelUntil.IsZero() {
		o.labelA = clamp01(v.labelUntil.Sub(now).Seconds() / vizFadeFor.Seconds())
	}
	if label != "" && o.labelA == 0 {
		o.labelA = 1 // an error message, shown for as long as it lasts
	}
	return o
}

// row returns the text of a row, where it starts and how visible it is;
// start is -1 when the row has none.
func (o vizOverlay) row(row, w, h int) (runes []rune, start int, alpha float64) {
	text := ""
	for _, t := range o.texts {
		if t.row == row {
			text, alpha = t.text, 1
		}
	}
	switch {
	case row == h-2 && o.hint != "":
		text, alpha = o.hint, o.hintA
	case row == 1 && o.label != "":
		text, alpha = o.label, o.labelA
	}
	if text == "" || alpha <= 0 {
		return nil, -1, 0
	}
	runes = []rune(text)
	if len(runes) > w {
		runes = runes[:w]
	}
	return runes, (w - len(runes)) / 2, alpha
}

// encode draws the frame in true colour: each cell is ▀ with the upper
// pixel as its foreground and the lower one as its background. A colour is
// written only when it changes. The texts are laid over the picture, fading
// out.
func (v *visualizer) encode(label string, now time.Time) string {
	w, h := v.w, v.h
	o := v.overlay(label, now)
	var sb strings.Builder
	sb.Grow(w * h * 24)
	var lastFg, lastBg [3]uint8
	for row := range h {
		runes, tx, alpha := o.row(row, w, h)
		var fgs [][3]uint8
		for _, t := range o.texts {
			if t.row == row {
				fgs = t.fg
			}
		}
		have := false
		for col := range w {
			top := toByte3(v.out[(row*2)*w+col])
			bot := toByte3(v.out[(row*2+1)*w+col])
			ch := "▀"
			if tx >= 0 && col >= tx && col < tx+len(runes) {
				// Text over the picture: a darkened backdrop, light letters.
				avg := [3]uint8{mean8(top[0], bot[0]), mean8(top[1], bot[1]), mean8(top[2], bot[2])}
				bg := scale3(avg, 1-0.65*alpha)
				fg := [3]uint8{235, 235, 235}
				if k := col - tx; len(fgs) > 0 {
					fg = fgs[min(k, len(fgs)-1)]
				}
				top = blend3(bg, fg, alpha)
				bot = bg
				ch = string(runes[col-tx])
				if ch == " " {
					top = bot
				}
			}
			if !have || top != lastFg {
				sb.WriteString("\x1b[38;2;")
				writeRGB(&sb, top)
				lastFg = top
			}
			if !have || bot != lastBg {
				sb.WriteString("\x1b[48;2;")
				writeRGB(&sb, bot)
				lastBg = bot
			}
			have = true
			sb.WriteString(ch)
		}
		sb.WriteString("\x1b[0m")
		if row < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// pixels is the frame as bytes, three a pixel, for a terminal that shows
// pictures.
func (v *visualizer) pixels() []byte {
	n := v.w * v.h2
	bpp := 3
	if v.over && len(v.alpha) == n {
		bpp = 4 // and how much of each pixel it covers, while it opens
	}
	if len(v.rgb) != n*bpp {
		v.rgb = make([]byte, n*bpp)
	}
	// All 256 levels (cells round theirs, to change colour less often), and
	// a fine ordered dither so that slow gradients don't show as bands.
	bayer := [16]float64{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}
	w := v.w
	par(v.h2, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := range w {
				i := y*w + x
				d := bayer[(y&3)<<2|x&3] / 16
				f := func(c float64) byte {
					return byte(min(255, int(toneOf(c)+d))) //nolint:gosec // 0..255: the curve and the dither are never below 0
				}
				c := v.out[i]
				o := i * bpp
				v.rgb[o], v.rgb[o+1], v.rgb[o+2] = f(c.r), f(c.g), f(c.b)
				if bpp == 4 {
					v.rgb[o+3] = byte(min(255, int(v.alpha[i]*255+0.5))) //nolint:gosec // 0..255: alpha is 0..1
				}
			}
		}
	})
	return v.rgb
}

// toneN is how many steps toneLUT has over 0..1.
const toneN = 8192

// toneLUT is the picture's tone curve, (1 - e^(-1.4c)) * 255/0.75, at
// toneN+1 steps of c over 0..1 (past 1 every channel is full): read with a
// straight line between steps it is within a hundred-thousandth of a level
// of the curve, and a frame takes three e^x a pixel fewer.
var toneLUT = func() (t [toneN + 2]float64) { //nolint:gochecknoglobals // a fixed table
	for i := range t {
		t[i] = (1 - math.Exp(-float64(i)/toneN*1.4)) * 255 / 0.75
	}
	return t
}()

// toneOf is the tone curve at c, in levels (see toneLUT).
func toneOf(c float64) float64 {
	if c <= 0 {
		return 0
	}
	if c >= 1 {
		return 256 // full
	}
	x := c * toneN
	i := int(x)
	return toneLUT[i] + (toneLUT[i+1]-toneLUT[i])*(x-float64(i))
}

// encodeText is the cells that go with a frame shown as a picture: empty
// ones, which the picture shows through, and the texts over it.
func (v *visualizer) encodeText(label string, now time.Time) string {
	w, h := v.cw, v.ch
	o := v.overlay(label, now)
	var sb strings.Builder
	sb.Grow((w + 1) * h)
	for row := range h {
		runes, tx, alpha := o.row(row, w, h)
		if tx < 0 {
			sb.WriteString(strings.Repeat(" ", w))
		} else {
			sb.WriteString(strings.Repeat(" ", tx))
			sb.WriteString("\x1b[1;38;2;")
			writeRGB(&sb, blend3(v.edge, [3]uint8{240, 240, 240}, alpha))
			sb.WriteString(string(runes))
			sb.WriteString("\x1b[0m")
			sb.WriteString(strings.Repeat(" ", max(0, w-tx-len(runes))))
		}
		if row < h-1 {
			sb.WriteByte('\n')
		}
	}
	rows := strings.Split(sb.String(), "\n")
	for y := range rows {
		rows[y] = linesRowMark(rows[y], y) // a scroll would take the picture along
	}
	return strings.Join(rows, "\n")
}

func writeRGB(sb *strings.Builder, c [3]uint8) {
	var buf [12]byte
	b := strconv.AppendUint(buf[:0], uint64(c[0]), 10)
	b = append(b, ';')
	b = strconv.AppendUint(b, uint64(c[1]), 10)
	b = append(b, ';')
	b = strconv.AppendUint(b, uint64(c[2]), 10)
	sb.Write(b)
	sb.WriteByte('m')
}

// toByte3 tone-maps a colour (glows add up past 1) and rounds it to steps of
// 4, so dark areas repeat colours and the output stays small.
func toByte3(c vcol) [3]uint8 {
	f := func(x float64) uint8 {
		x = 1 - math.Exp(-max(0, x)*1.4)
		return uint8(min(255, int(x*255.0/0.75+0.5)) &^ 3) //nolint:gosec // 0..255: x is 0..1
	}
	return [3]uint8{f(c.r), f(c.g), f(c.b)}
}

func scale3(c [3]uint8, k float64) [3]uint8 {
	return [3]uint8{uint8(float64(c[0]) * k), uint8(float64(c[1]) * k), uint8(float64(c[2]) * k)}
}

func blend3(a, b [3]uint8, k float64) [3]uint8 {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*k) }
	return [3]uint8{m(a[0], b[0]), m(a[1], b[1]), m(a[2], b[2])}
}

// ── The Linux console ───────────────────────────────────────────────────────
//
// The console has 16 colours, and a background can use only the first 8.
// While the visualizer is open the whole palette is its own: black, then
// each of the picture's three hues dark, medium, full and pale, and white.
// A cell is drawn one of two ways, whichever is closer to the picture:
//
//   - two pixels, "▀" or "▄": one half any of the 16 colours, the other one
//     of the 8;
//   - one colour for the whole cell, mixed from two palette colours with
//     "░", "▒" or "▓". That gives a couple of hundred shades, so glows and
//     gradients come out smooth instead of in bands. Only colours that sit
//     next to each other are mixed (a hue with its own lighter tone, or two
//     hues of the same tone): far ones would show as dots, not as a shade.
//
// The palette goes back to the app's when the visualizer closes.

// vizCon holds the console palette of the visualizer and the tables that
// find the closest way to draw a colour with it.
type vizCon struct {
	pal    [16][3]float64 // 0..255
	seq    string         // programs pal into the console
	mixes  []vizMix
	lutMix []uint16 // colour key → mixes index + 1; 0: not looked up yet
	lut16  []uint8  // colour key → nearest of all 16, + 1
	lut8   []uint8  // colour key → nearest of the first 8, + 1
}

// vizMix is one way to fill a cell with a single colour. fg or bg is -1
// when the glyph doesn't show it. rough is how much its two colours differ:
// between two mixes that match as well, the smoother one wins.
type vizMix struct {
	fg, bg int
	ch     string
	c      [3]float64
	rough  float64
}

// vizConHue and vizConTone tell where each palette entry sits: its hue (0–2;
// -1 for black, white and the neutral light one) and how light it is.
var (
	vizConHue  = [16]int{-1, 0, 1, 2, 0, 1, 2, -1, 0, 0, 1, 2, 0, 1, 2, -1}
	vizConTone = [16]int{0, 1, 1, 1, 2, 2, 2, 4, 3, 3, 3, 3, 4, 4, 4, 5}
)

// vizConNear reports whether palette entries a and b are close enough to be
// mixed in one cell.
func vizConNear(a, b int) bool {
	dt := vizConTone[a] - vizConTone[b]
	if dt < 0 {
		dt = -dt
	}
	if vizConHue[a] == vizConHue[b] || vizConHue[a] < 0 || vizConHue[b] < 0 {
		return dt <= 2
	}
	return dt == 0
}

// vizConPalette builds the console palette from the picture's three hues.
// Entries 0–7 can be backgrounds: they get the dark and medium tones, which
// fill most of any frame.
func vizConPalette(p [3]vcol) [16][3]float64 {
	rgb := func(c vcol) [3]float64 {
		return [3]float64{clamp01(c.r) * 255, clamp01(c.g) * 255, clamp01(c.b) * 255}
	}
	all := p[0].add(p[1]).add(p[2]).mul(1.0 / 3)
	return [16][3]float64{
		{},
		rgb(p[0].mul(0.22)), rgb(p[1].mul(0.22)), rgb(p[2].mul(0.22)),
		rgb(p[0].mul(0.55)), rgb(p[1].mul(0.55)), rgb(p[2].mul(0.55)),
		rgb(all.mix(vizWhite, 0.5).mul(0.75)),
		rgb(p[0].mix(p[1], 0.5)),
		rgb(p[0]), rgb(p[1]), rgb(p[2]),
		rgb(p[0].mix(vizWhite, 0.55)), rgb(p[1].mix(vizWhite, 0.55)), rgb(p[2].mix(vizWhite, 0.55)),
		{255, 255, 255},
	}
}

func newVizCon(p [3]vcol) *vizCon {
	c := &vizCon{
		pal:    vizConPalette(p),
		lutMix: make([]uint16, 1<<15),
		lut16:  make([]uint8, 1<<15),
		lut8:   make([]uint8, 1<<15),
	}
	var sb strings.Builder
	for i, q := range c.pal {
		fmt.Fprintf(&sb, "\x1b]P%x%02x%02x%02x", i, int(q[0]+0.5), int(q[1]+0.5), int(q[2]+0.5))
	}
	c.seq = sb.String()
	for i, q := range c.pal {
		if i < 8 {
			c.mixes = append(c.mixes, vizMix{fg: -1, bg: i, ch: " ", c: q})
		} else {
			c.mixes = append(c.mixes, vizMix{fg: i, bg: -1, ch: "█", c: q})
		}
	}
	// A shade glyph lights a part of the cell's dots with the foreground:
	// the eye adds the two colours up as light, not as numbers.
	lin := func(x float64) float64 { return math.Pow(x/255, 2.2) }
	for bg := range 8 {
		for fg := range 16 {
			if fg == bg || !vizConNear(fg, bg) {
				continue
			}
			rough := 0.04 * vizDist3(c.pal[fg], c.pal[bg])
			for k, ch := range []string{"░", "▒", "▓"} {
				part := 0.25 * float64(k+1)
				var m [3]float64
				for i := range 3 {
					m[i] = 255 * math.Pow(lin(c.pal[bg][i])*(1-part)+lin(c.pal[fg][i])*part, 1/2.2)
				}
				c.mixes = append(c.mixes, vizMix{fg, bg, ch, m, rough})
			}
		}
	}
	return c
}

func vizKey(c [3]uint8) int { return int(c[0]>>3)<<10 | int(c[1]>>3)<<5 | int(c[2]>>3) }

// vizDist is how different two colours look, roughly. A wrong hue shows
// more than a wrong brightness (neighbouring cells of different hues look
// like noise), so the colour part of the difference counts three times.
func vizDist(a [3]uint8, b [3]float64) float64 {
	dr, dg, db := float64(a[0])-b[0], float64(a[1])-b[1], float64(a[2])-b[2]
	dy := 0.3*dr + 0.59*dg + 0.11*db
	return dy*dy + 3*(sq(dr-dy)+sq(db-dy))
}

func vizDist3(a, b [3]float64) float64 {
	dr, dg, db := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	dy := 0.3*dr + 0.59*dg + 0.11*db
	return dy*dy + 3*(sq(dr-dy)+sq(db-dy))
}

// nearest is the palette entry below limit closest to c.
func (c *vizCon) nearest(col [3]uint8, limit int) int {
	lut := c.lut16
	if limit == 8 {
		lut = c.lut8
	}
	k := vizKey(col)
	if lut[k] == 0 {
		best, bd := 0, math.Inf(1)
		for i := range limit {
			if d := vizDist(col, c.pal[i]); d < bd {
				best, bd = i, d
			}
		}
		lut[k] = uint8(best + 1) //nolint:gosec // below 16
	}
	return int(lut[k]) - 1
}

// mix is the closest single-colour cell to c.
func (c *vizCon) mix(col [3]uint8) *vizMix {
	k := vizKey(col)
	if c.lutMix[k] == 0 {
		best, bd := 0, math.Inf(1)
		for i := range c.mixes {
			if d := vizDist(col, c.mixes[i].c) + c.mixes[i].rough; d < bd {
				best, bd = i, d
			}
		}
		c.lutMix[k] = uint16(best + 1) //nolint:gosec // a few hundred
	}
	return &c.mixes[c.lutMix[k]-1]
}

// cell picks how to draw a cell whose halves are top and bot.
func (c *vizCon) cell(top, bot [3]uint8) (fg, bg int, ch string) {
	avg := [3]uint8{mean8(top[0], bot[0]), mean8(top[1], bot[1]), mean8(top[2], bot[2])}
	m := c.mix(avg)
	if top == bot {
		return m.fg, m.bg, m.ch
	}
	eMix := vizDist(top, m.c) + vizDist(bot, m.c)
	t16, b8 := c.nearest(top, 16), c.nearest(bot, 8)
	eUp := vizDist(top, c.pal[t16]) + vizDist(bot, c.pal[b8])
	t8, b16 := c.nearest(top, 8), c.nearest(bot, 16)
	eLow := vizDist(top, c.pal[t8]) + vizDist(bot, c.pal[b16])
	switch {
	case eMix <= eUp && eMix <= eLow:
		return m.fg, m.bg, m.ch
	case eUp <= eLow:
		return t16, b8, "▀"
	default:
		return b16, t8, "▄"
	}
}

// encodeConsole draws the frame for the Linux console. Colours go out the
// way the console takes them: 30–37 and 40–47, and bold for the bright
// eight (22 takes it off again).
func (v *visualizer) encodeConsole(label string, now time.Time) string {
	w, h := v.w, v.h
	o := v.overlay(label, now)
	var sb strings.Builder
	sb.Grow(w * h * 8)
	for row := range h {
		runes, tx, _ := o.row(row, w, h)
		curFg, curBg := -1, -1
		for col := range w {
			fg, bg, ch := v.con.cell(toByte3(v.out[(row*2)*w+col]), toByte3(v.out[(row*2+1)*w+col]))
			if tx >= 0 && col >= tx && col < tx+len(runes) {
				fg, bg, ch = 15, 0, string(runes[col-tx])
			}
			if fg >= 0 && fg != curFg {
				if fg >= 8 {
					sb.WriteString("\x1b[1;3")
				} else {
					sb.WriteString("\x1b[22;3")
				}
				sb.WriteByte(byte('0' + fg&7))
				sb.WriteByte('m')
				curFg = fg
			}
			if bg >= 0 && bg != curBg {
				sb.WriteString("\x1b[4")
				sb.WriteByte(byte('0' + bg))
				sb.WriteByte('m')
				curBg = bg
			}
			if bg < 0 && curBg < 0 {
				// "█" hides the background, but the cell still needs one.
				sb.WriteString("\x1b[40m")
				curBg = 0
			}
			sb.WriteString(ch)
		}
		sb.WriteString("\x1b[0m")
		if row < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// mean8 is the mean of two bytes, rounded down.
func mean8(a, b uint8) uint8 { return a/2 + b/2 + a&b&1 }
