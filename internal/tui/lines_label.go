package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tphx37/utuimp/internal/gfx"
)

// The player's own words (the song's title, the artist, the album, the
// times, the volume) drawn as a picture, like the lyrics: in the system's
// sans, the title larger and bold, instead of the terminal's letters; the
// volume's note is a small loudspeaker drawn in the buttons' strokes. In
// the simple style, and where no font is found, they are the characters
// they always were.
//
// A label is put into the screen as a mark in its first cell and blanks for
// the rest of its width (see Model.label); the mark is found again in the
// finished screen, which tells where the label is, and the picture is drawn
// over all of them.

// The kinds of label: each has its own size and colour.
const (
	labelTitle = iota
	labelArtist
	labelAlbum
	labelTime
	labelSmall
	labelSpeaker // no words: the loudspeaker, as loud as the volume
)

// Where a label's words sit in its cells.
const (
	alignCentre = iota
	alignLeft
	alignRight
)

// labelMark0 is the mark of the first label; the next ones follow.
const (
	labelMark0   = ''
	labelMarkEnd = ''
)

// lineLabel is one label: what it says, how, and where it was found. It
// takes rows rows from the one its mark is on down, and its words are
// scale times their usual size, in the middle of them.
type lineLabel struct {
	kind, align int
	text        string
	cells       int
	x, y        int
	found       bool
	rows        int
	scale       float64
	scroll      bool
	scrollAt    time.Time
	animate     bool
	glide       bool
}

// label makes a label of text, cells wide, and returns what to put into the
// screen in its place; ok is false when labels are not drawn (then the
// caller writes the words itself).
func (m *Model) label(kind int, text string, cells, align int) (string, bool) {
	return m.labelSized(kind, text, cells, align, 1, m.labelScale(kind))
}

// labelScale is how much larger than usual a kind of label is drawn: the
// song beside the cover alone (the cover view with the lyrics hidden) is
// set large, in proportion to the cover (see coverSongRight); everywhere
// else the words are their usual size.
func (m *Model) labelScale(kind int) float64 {
	if !m.songBig {
		return 1
	}
	switch kind {
	case labelTitle:
		return 1.4
	case labelArtist:
		return 1.2
	case labelAlbum:
		return 1.1
	case labelTime, labelSmall, labelSpeaker:
		return 1.25
	}
	return 1
}

// songBigRows is how many rows the title and the artist take, each, beside
// the cover alone (the album and the rest keep one).
const songBigRows = 2

// labelSized is label for words scale times their usual size, over rows
// rows (the one returned and as many blank ones under it as the caller
// puts there).
func (m *Model) labelSized(kind int, text string, cells, align, rows int, scale float64) (string, bool) {
	l := m.lines
	if l == nil || cells < 1 || !m.lyricsPicOn() || len(l.labels) >= labelMarkEnd-labelMark0 {
		return "", false
	}
	l.labels = append(l.labels, lineLabel{
		kind: kind, align: align, text: text, cells: cells,
		rows: max(1, rows), scale: scale,
		scroll:   kind == labelTitle,
		scrollAt: m.trackChangedAt,
		animate:  m.coverView && (kind == labelTitle || kind == labelArtist || kind == labelAlbum),
		glide:    m.coverView || (kind != labelTitle && kind != labelArtist && kind != labelAlbum),
	})
	return string(rune(labelMark0+len(l.labels)-1)) + strings.Repeat(" ", cells-1), true //nolint:gosec // under labelMarkEnd, checked above
}

// scrollOffset moves to the end and back, pausing at each end.
func scrollOffset(overflow int, speed float64, at, now time.Time) int {
	if overflow <= 0 {
		return 0
	}
	const pause = 1200 * time.Millisecond
	move := time.Duration(float64(overflow) / speed * float64(time.Second))
	phase := max(now.Sub(at), 0) % (2 * (pause + move))
	switch {
	case phase < pause:
		return 0
	case phase < pause+move:
		return int(float64(overflow) * float64(phase-pause) / float64(move))
	case phase < 2*pause+move:
		return overflow
	default:
		return overflow - int(float64(overflow)*float64(phase-2*pause-move)/float64(move))
	}
}

// scrollLabel returns the title and its pixel offset within the available span.
func scrollLabel(f *gfx.Font, text string, span int, at, now time.Time) (string, int) {
	if f == nil || span <= 0 {
		return text, 0
	}
	textW := f.Width(text)
	if textW <= span {
		return text, 0
	}
	return text, scrollOffset(textW-span, 42, at, now)
}

// labelFont is the font of a kind of label.
// markKind is the kind of the label whose mark starts s, or -1 when s
// does not start with one.
func (l *uiLines) markKind(s string) int {
	r, _ := utf8.DecodeRuneInString(s)
	if n := int(r - labelMark0); r >= labelMark0 && r < labelMarkEnd && n < len(l.labels) {
		return l.labels[n].kind
	}
	return -1
}

func (l *uiLines) labelFont(kind int, scale float64) *gfx.Font {
	size, pattern := 0.88, "sans-serif"
	switch kind {
	case labelTitle:
		size, pattern = 1.2, "sans-serif:bold"
	case labelArtist:
		size = 1.0
	}
	if scale <= 0 {
		scale = 1
	}
	px := math.Round(size * scale * float64(l.cellH))
	key := pattern + "@" + strconv.Itoa(int(px))
	if f, ok := l.fonts[key]; ok {
		return f
	}
	if l.fonts == nil {
		l.fonts = map[string]*gfx.Font{}
	}
	f, err := gfx.LoadFont(pattern, px)
	if err != nil {
		f = nil
	}
	l.fonts[key] = f
	return f
}

// labelLook is the colours the labels are drawn in.
type labelLook struct {
	fg, accent vcol
	volume     float64 // 0..1; below 0: muted
	fade       float64 // 0..1: how far the song's new words have come up
	gone       float64 // 1..0: how much is left of the last song's
}

// drawLabels draws the labels into buf (w×h pixels), whose corner is the
// pixel (ox, oy) of the whole terminal; hostRows are the rows over the
// player's screen. The labels on the screen are drawn where they are on
// their way (see glideLabels); the last song's words where they were.
func (l *uiLines) drawLabels(buf []byte, w, h int, ox, oy float64, hostRows int, look labelLook) {
	clear(buf)
	if look.gone > 0 {
		geo := make([]labelGeom, len(l.goneWords))
		for i, lb := range l.goneWords {
			geo[i] = l.labelTarget(lb, hostRows)
		}
		l.drawLabelSet(buf, w, h, ox, oy, l.goneWords, geo, look, true)
	}
	geo := l.labelCur
	if len(geo) != len(l.labels) {
		geo = make([]labelGeom, len(l.labels))
		for i, lb := range l.labels {
			if lb.found {
				geo[i] = l.labelTarget(lb, hostRows)
			}
		}
	}
	l.drawLabelSet(buf, w, h, ox, oy, l.labels, geo, look, false)
}

// fitLabel is a label's words cut to fit span pixels in f: only long words
// are cut (a title); a time or the volume is short and is always whole, a
// little over its cells if need be.
func fitLabel(f interface{ Width(string) int }, kind int, text string, span int) string {
	for (kind == labelTitle || kind == labelArtist || kind == labelAlbum) && f.Width(text) > span && len([]rune(text)) > 2 {
		r := []rune(strings.TrimSuffix(text, "…"))
		text = strings.TrimRight(string(r[:len(r)-1]), " ") + "…"
	}
	return text
}

// labelBase is the baseline of a label's words in f, its rows starting at
// y0 (pixels), chh pixels a row.
func labelBase(lb lineLabel, f *gfx.Font, y0, chh float64) float64 {
	// The words' middle on the middle of their rows, whatever their size.
	// Larger words are set by their capitals (about two thirds of the
	// font's ascent): set by the font's whole line, which has room to
	// spare over and under, they sank to the bottom of their rows and into
	// the words under them.
	if lb.rows > 1 || lb.scale > 1 {
		return y0 + float64(max(1, lb.rows))*chh/2 + float64(f.Ascent)/3
	}
	base := y0 + math.Trunc((chh-float64(f.Height))/2) + float64(f.Ascent)
	if lb.kind == labelTitle {
		base -= math.Trunc(chh * 22 / 100) // the larger title a little clear of the row under it
	}
	return base
}

// drawLabelSet draws one set of labels over what buf has, each where geo
// says: the words on screen, or (gone) the last song's on their way out.
func (l *uiLines) drawLabelSet(buf []byte, w, h int, ox, oy float64, labels []lineLabel, geo []labelGeom, look labelLook, gone bool) {
	white := vcol{1, 1, 1}
	chh := float64(l.cellH)
	for n, lb := range labels {
		if !lb.found || n >= len(geo) {
			continue
		}
		g := geo[n]
		// The song's words come and go with it; the times and the
		// volume stay as they are.
		k := 1.0
		switch {
		case gone:
			k = look.gone
		case isSongWord(lb):
			k = look.fade
		}
		if k <= 0 {
			continue
		}
		c, alpha := look.fg, 0.7
		switch lb.kind {
		case labelTitle:
			c, alpha = look.fg.mix(white, 0.45), 1
		case labelArtist:
			c, alpha = look.accent, 0.95
		case labelAlbum, labelSmall:
			alpha = 0.6
		case labelTime:
			alpha = 0.8
		case labelSpeaker:
			if !gone {
				drawSpeaker(buf, w, h, g.cx-ox, g.base-oy, chh*g.scale, look.volume, look.fg, 0.75)
			}
			continue
		}
		f := l.labelFont(lb.kind, g.scale)
		if f == nil {
			continue
		}
		text := fitLabel(f, lb.kind, lb.text, int(g.span))
		tw := f.Width(text)
		x := int(math.Round(g.cx-ox)) - tw/2
		if lb.scroll && !gone && f.Width(lb.text) > int(g.span) {
			var offset int
			text, offset = scrollLabel(f, lb.text, int(g.span), lb.scrollAt, time.Now())
			tw = f.Width(text)
			x = int(math.Round(g.cx-ox-g.span/2)) - offset
		}
		base := int(math.Round(g.base - oy))
		mask := l.labelMask(w * h)
		f.Draw(mask, w, h, x, base, text)
		if lb.scroll && !gone {
			left := int(math.Round(g.cx - ox - g.span/2))
			right := int(math.Round(g.cx - ox + g.span/2))
			for py := max(0, base-f.Ascent-2); py < min(h, base+f.Height); py++ {
				for px := 0; px < min(w, max(0, left)); px++ {
					mask[py*w+px] = 0
				}
				for px := max(0, min(w, right)); px < w; px++ {
					mask[py*w+px] = 0
				}
			}
		}
		r, gg, b := byte(255*clamp01(c.r)), byte(255*clamp01(c.g)), byte(255*clamp01(c.b))
		for py := max(0, base-f.Ascent-2); py < min(h, base+f.Height); py++ {
			for px := max(0, x-2); px < min(w, x+tw+4); px++ {
				m := mask[py*w+px]
				if m == 0 {
					continue
				}
				mask[py*w+px] = 0
				over(buf[(py*w+px)*4:], r, gg, b, float64(m)/255*alpha*k)
			}
		}
	}
}

// labelMask is a cleared scratch mask of n pixels.
func (l *uiLines) labelMask(n int) []byte {
	if len(l.mask) != n {
		l.mask = make([]byte, n)
	}
	return l.mask
}

// drawSpeaker draws a small loudspeaker with its middle at (cx, cy): the
// box and the cone, and one or two waves as the volume goes up, or a cross
// when there is none (it is at nothing, or muted). cell is a cell's height.
func drawSpeaker(buf []byte, w, h int, cx, cy, cell, volume float64, c vcol, alpha float64) {
	r := 0.36 * cell
	stroke := math.Max(1.3, cell/13) / 2
	cr, cg, cb := byte(255*clamp01(c.r)), byte(255*clamp01(c.g)), byte(255*clamp01(c.b))
	for y := int(cy-r*1.4) - 2; y <= int(cy+r*1.4)+2; y++ {
		for x := int(cx-r*1.7) - 2; x <= int(cx+r*1.9)+2; x++ {
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			px, py := (float64(x)-cx)/r, (float64(y)-cy)/r
			// The box and the cone, as one outline.
			d := math.Min(sdSeg(px, py, -1.2, -0.38, -0.7, -0.38), sdSeg(px, py, -1.2, 0.38, -0.7, 0.38))
			d = math.Min(d, sdSeg(px, py, -1.2, -0.38, -1.2, 0.38))
			d = math.Min(d, sdSeg(px, py, -0.7, -0.38, -0.1, -0.95))
			d = math.Min(d, sdSeg(px, py, -0.7, 0.38, -0.1, 0.95))
			d = math.Min(d, sdSeg(px, py, -0.1, -0.95, -0.1, 0.95))
			if volume <= 0.004 {
				d = math.Min(d, sdSeg(px, py, 0.45, -0.45, 1.25, 0.45))
				d = math.Min(d, sdSeg(px, py, 0.45, 0.45, 1.25, -0.45))
			} else {
				// Waves, each a part of a circle round the cone's mouth.
				ang := math.Abs(math.Atan2(py, px+0.1))
				for n, at := range []float64{0.02, 0.5} {
					if volume > at && ang < 0.85 {
						k := clamp01((volume - at) / 0.25) // it comes in as the volume passes it
						dw := math.Abs(math.Hypot(px+0.1, py)-(0.62+0.52*float64(n))) + (1-k)*0.5
						d = math.Min(d, dw)
					}
				}
			}
			a := clamp01(stroke-d*r+0.5) * alpha
			if a < 0.004 {
				continue
			}
			i := (y*w + x) * 4
			if v := byte(255 * a); v > buf[i+3] {
				buf[i], buf[i+1], buf[i+2], buf[i+3] = cr, cg, cb, v
			}
		}
	}
}

// labelsSig tells one set of labels from another: what they say and where.
func (l *uiLines) labelsSig() string {
	var sb strings.Builder
	for _, lb := range l.labels {
		if lb.found {
			// The place as numbers: a byte each would have taken a column
			// past 255 for another one.
			fmt.Fprintf(&sb, "%s\x00%d,%d,%d,%d,%g\x00", lb.text, lb.kind, lb.x, lb.y, lb.rows, lb.scale)
		}
	}
	return sb.String()
}

// Another song's words melt in as the last one's melt away: those go over
// labelsOutFor, the new ones come up from labelsInAfter to labelsInFor.
const (
	labelsOutFor  = 500 * time.Millisecond
	labelsInAfter = 0 * time.Millisecond
	labelsInFor   = 500 * time.Millisecond
)

// isSongWord reports whether a label gets the crossfade animation.
func isSongWord(lb lineLabel) bool {
	return lb.animate
}

// over lays a pixel of colour (r, g, b) and cover a (0..1) over the one at
// px (four bytes, its colour not multiplied by its cover).
func over(px []byte, r, g, b byte, a float64) {
	under := float64(px[3]) / 255
	out := a + under*(1-a)
	if out <= 0 {
		return
	}
	mix := func(top, bot byte) byte {
		return byte((float64(top)*a + float64(bot)*under*(1-a)) / out)
	}
	px[0], px[1], px[2], px[3] = mix(r, px[0]), mix(g, px[1]), mix(b, px[2]), byte(255*out+0.5)
}
