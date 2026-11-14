package tui

import (
	"math"
	"time"
)

// The song's block (its words, the progress line, the buttons and the
// volume) goes over to another place and size instead of jumping there:
// with the lyrics hidden or shown again it moves between beside the cover
// and under it, growing or shrinking on the way, along with the cover; so
// it does on another view, the key hints going, and the like. A label
// goes by itself, its words growing as it goes; a line or the buttons go
// as a whole, drawn at their size on the way and placed by the pixel.

// Beside the cover alone (see Model.songBig) the buttons, and the
// progress and volume lines, are drawn this much larger.
const (
	songBigCtl  = 1.35
	songBigLine = 1.25
)

// stripGlide is a strip's way from one place and size to another: x, y, w,
// h in pixels of the whole terminal, and how large it is drawn (1 as
// usual), from and to, since at.
type stripGlide struct {
	from, to [5]float64
	at       time.Time
	ok       bool
}

// cur is where the strip is on its way at now.
func (g stripGlide) cur(now time.Time) [5]float64 {
	p := clamp01(now.Sub(g.at).Seconds() / linesMoveFor)
	if p >= 1 {
		return g.to
	}
	p = p * p * (3 - 2*p) // as the cover goes (see linesDraw)
	var r [5]float64
	for i := range r {
		r[i] = g.from[i] + (g.to[i]-g.from[i])*p
	}
	return r
}

// moving reports whether the strip is still on its way.
func (g stripGlide) moving(now time.Time) bool {
	return g.from != g.to && now.Sub(g.at).Seconds() < linesMoveFor
}

// aim sends the strip to, from where it is on its way, when it goes (carry);
// else it is there at once.
func (g *stripGlide) aim(to [5]float64, now time.Time, carry bool) {
	switch {
	case !g.ok || !carry:
		*g = stripGlide{from: to, to: to, ok: true}
	case to != g.to:
		g.from, g.to, g.at = g.cur(now), to, now
	}
}

// glideCarry reports whether what moves on the screen now goes over to
// its new place: not with the effects off, nor when the terminal or its
// cells changed their size (all of it is laid out anew then).
func (l *uiLines) glideCarry(m *Model) bool {
	size := [4]int{m.width, m.height, l.cellW, l.cellH}
	same := size == l.glideSize
	l.glideSize = size
	return same && m.fxLevel() > 0
}

// glideStrip places a line or the buttons (i) on their way: w and h are
// their size where they are going; it returns the size to draw them at
// now, how large, and whether they are on their way. Placed on their way,
// they are put by the pixel (st.col and st.row the cell, the offsets in
// st.img); where they are going is kept (st.cell) for the frames drawn
// before the screen is laid out again.
func (l *uiLines) glideStrip(st *lineStrip, w, h int, now time.Time, carry bool) (int, int, float64, bool) {
	if st.glided {
		st.col, st.row = st.cell[0], st.cell[1]
	} else {
		st.cell = [2]int{st.col, st.row}
	}
	scale := st.scale
	if scale <= 0 {
		scale = 1
	}
	to := [5]float64{float64(st.col * l.cellW), float64(st.row * l.cellH), float64(w), float64(h), scale}
	st.glide.aim(to, now, carry && st.shown)
	if !st.glide.moving(now) {
		return w, h, scale, false
	}
	r := st.glide.cur(now)
	x, y := int(math.Round(r[0])), int(math.Round(r[1]))
	st.col, st.row, st.glided = x/l.cellW, y/l.cellH, true
	st.img.Native, st.img.OffX, st.img.OffY = true, x%l.cellW, y%l.cellH
	return max(1, int(math.Round(r[2]))), max(1, int(math.Round(r[3]))), r[4], true
}

// labelGeom is where a label's words are drawn, in pixels of the whole
// terminal: the middle of the words across, their baseline, how large
// they are, and how wide they may be.
type labelGeom struct {
	cx, base, scale, span float64
}

// labelGlide is a label's way from one place and size to another.
type labelGlide struct {
	from, to labelGeom
	at       time.Time
	seen     int // the frame it was last drawn in (see uiLines.labelGen)
}

func (g *labelGlide) cur(now time.Time) labelGeom {
	p := clamp01(now.Sub(g.at).Seconds() / linesMoveFor)
	if p >= 1 {
		return g.to
	}
	p = p * p * (3 - 2*p)
	mix := func(a, b float64) float64 { return a + (b-a)*p }
	return labelGeom{mix(g.from.cx, g.to.cx), mix(g.from.base, g.to.base), mix(g.from.scale, g.to.scale), mix(g.from.span, g.to.span)}
}

// labelKeys tell the labels from one another across screens: each one's
// kind, and which of that kind it is (the two times).
func (l *uiLines) labelKeys() []int {
	keys := make([]int, len(l.labels))
	var n [8]int
	for i, lb := range l.labels {
		k := min(max(lb.kind, 0), len(n)-1)
		keys[i] = k*8 + n[k]
		n[k]++
	}
	return keys
}

// labelTarget is where a label's words go, as laid out on the screen now
// (hostRows: the rows over the player's screen).
func (l *uiLines) labelTarget(lb lineLabel, hostRows int) labelGeom {
	cw, chh := float64(l.cellW), float64(l.cellH)
	scale := lb.scale
	if scale <= 0 {
		scale = 1
	}
	x0, y0 := float64(lb.x)*cw, float64(lb.y+hostRows)*chh
	span := float64(lb.cells) * cw
	rowsH := float64(max(1, lb.rows)) * chh
	if lb.kind == labelSpeaker {
		return labelGeom{cx: x0 + span - 0.55*chh*scale, base: y0 + rowsH/2 - 0.5, scale: scale, span: span}
	}
	g := labelGeom{scale: scale, span: span}
	f := l.labelFont(lb.kind, scale)
	if f == nil {
		g.cx, g.base = x0+span/2, y0+chh/2
		return g
	}
	tw := float64(f.Width(fitLabel(f, lb.kind, lb.text, int(span))))
	switch lb.align {
	case alignLeft:
		g.cx = x0 + tw/2
	case alignRight:
		g.cx = x0 + span - tw/2
	default:
		g.cx = x0 + span/2
	}
	g.base = labelBase(lb, f, y0, chh)
	return g
}

// glideLabels brings each label's place up to now (see labelGlide) and
// reports whether any is on its way.
func (l *uiLines) glideLabels(now time.Time, hostRows int, carry bool) bool {
	if l.labelGlides == nil {
		l.labelGlides = map[int]*labelGlide{}
	}
	keys := l.labelKeys()
	l.labelGen++
	l.labelCur = l.labelCur[:0]
	moving := false
	for i, lb := range l.labels {
		if !lb.found {
			l.labelCur = append(l.labelCur, labelGeom{})
			continue
		}
		to := l.labelTarget(lb, hostRows)
		g, ok := l.labelGlides[keys[i]]
		switch {
		case !ok || !carry || g.seen != l.labelGen-1:
			// New, or not there in the last frame: there at once.
			g = &labelGlide{from: to, to: to}
			l.labelGlides[keys[i]] = g
		case to != g.to:
			g.from, g.to, g.at = g.cur(now), to, now
		}
		g.seen = l.labelGen
		cur := g.cur(now)
		moving = moving || cur != g.to
		l.labelCur = append(l.labelCur, cur)
	}
	return moving
}

// labelsBounds is the rectangle (x0, y0, x1, y1, in pixels of the whole
// terminal) the labels on their way take, with room round them.
func (l *uiLines) labelsBounds(hostRows int) (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	cw, chh := float64(l.cellW), float64(l.cellH)
	for i, lb := range l.labels {
		if !lb.found || i >= len(l.labelCur) {
			continue
		}
		g := l.labelCur[i]
		x0, x1 = math.Min(x0, g.cx-g.span/2-cw), math.Max(x1, g.cx+g.span/2+2*cw)
		if lb.align != alignCentre {
			// Words set to one side are as wide as their span either way
			// of their middle, at most.
			x0, x1 = math.Min(x0, g.cx-g.span-cw), math.Max(x1, g.cx+g.span+2*cw)
		}
		y0, y1 = math.Min(y0, g.base-1.9*chh*g.scale), math.Max(y1, g.base+chh*g.scale)
	}
	for _, lb := range l.goneWords {
		x0, x1 = math.Min(x0, float64(lb.x)*cw-cw), math.Max(x1, float64(lb.x+lb.cells+2)*cw)
		y0, y1 = math.Min(y0, float64(lb.y+hostRows-1)*chh), math.Max(y1, float64(lb.y+hostRows+max(1, lb.rows)+1)*chh)
	}
	return x0, y0, x1, y1
}

// glideLabelsStrip places the labels' picture over where they are on
// their way, by the pixel: the size to draw it at, and whether they are on
// their way (else it stays where the screen was laid out, in cells).
func (l *uiLines) glideLabelsStrip(st *lineStrip, w, h int, now time.Time, m *Model, carry bool) (int, int, bool) {
	if st.glided {
		st.col, st.row = st.cell[0], st.cell[1]
	} else {
		st.cell = [2]int{st.col, st.row}
	}
	st.img.Native, st.img.OffX, st.img.OffY = false, 0, 0
	if !l.glideLabels(now, m.hostRows, carry && st.shown) {
		return w, h, false
	}
	x0, y0, x1, y1 := l.labelsBounds(m.hostRows)
	x, y := max(0, int(math.Floor(x0))), max(0, int(math.Floor(y0)))
	xe := min(m.width*l.cellW, int(math.Ceil(x1)))
	ye := min((m.height+m.hostRows)*l.cellH, int(math.Ceil(y1)))
	if xe <= x || ye <= y {
		return w, h, false
	}
	st.col, st.row, st.glided = x/l.cellW, y/l.cellH, true
	st.img.Native, st.img.OffX, st.img.OffY = true, x%l.cellW, y%l.cellH
	return xe - x, ye - y, true
}
