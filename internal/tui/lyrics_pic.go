package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/tphx37/utuimp/internal/gfx"
	"github.com/tphx37/utuimp/internal/tui/views"
)

// The cover view's lyrics as a picture (where the interface's lines are
// pictures, see lines.go): the words are drawn by utuimp itself, in the
// system's bold sans at well over a cell's height, the way a fullscreen
// player shows them. Being a picture, they move by pixels: a line change
// glides the text up, the line being sung brightens and the one just sung
// lets go, and with word times the light runs along the line with the
// voice. Lines sung fade towards the top, the coming ones wait dimmer, and
// the text melts away at the top and bottom edges.

const (
	// lyricSize is the letters' size, in cell heights; lyricSmall the
	// translations'.
	lyricSize  = 1.6
	lyricSmall = 0.95
	// lyricAnchor is where the current line sits, as a share of the height.
	lyricAnchor = 0.34
	// lyricClear is the share of the height at the top, and at the bottom,
	// that is kept clear of words: the waves over and under the lyrics run
	// there.
	lyricClear = 0.15
)

// lyricRow is one row of drawn words: how solid each pixel is, and where
// each letter ends.
type lyricRow struct {
	mask []byte
	w, h int
	ends []int
	from int // the row's first letter, counted in the line
}

// lyricBlock is a line of the lyrics drawn: its rows, its translation's.
type lyricBlock struct {
	rows, trans []lyricRow
	height      int
}

// lyricsPic draws the lyrics and keeps what moves between frames.
type lyricsPic struct {
	font, small *gfx.Font
	failed      bool
	cell        int
	scale       float64 // the size asked for, as against the usual (see ready)
	width       int
	blocks      map[string]*lyricBlock
	tops        []int
	scroll      float64
	vel         float64 // how fast the text glides, pixels a second
	settled     bool
	lit         []float64 // each line's light, 0..1, eased
	// Where the words that can be read begin and end, in pixels down the
	// picture: the waves over and under the lyrics keep to them.
	extTop, extBot float64
	of             string // the lyrics these are (their first lines)
	// What is shown (another song's lyrics, a word in their place, the
	// translation turned on or off) melts into what comes next: id says
	// what it is, last is the picture last shown, from the one melting
	// away and fade how far it has gone, 0..1.
	id, shownID string
	last, from  []byte
	fade        float64
	// lastItems is what was shown last; lastOK says whether last is it as a
	// whole picture too (shown a line a picture, it isn't drawn as one).
	lastItems []lyricItem
	lastOK    bool
}

// lyricGlide is the spring the text glides to a new line on, in radians a
// second: some two thirds of a second to get there.
const lyricGlide = 7.5

// lyricFadeFor is how long, in seconds, one lyrics picture takes to melt
// into the next.
const lyricFadeFor = 0.32

// ready makes the fonts for cells cell pixels high, scale times the usual
// size (Settings, Lyrics, Size), and reports whether the lyrics can be
// drawn at all (no fontconfig, no font: then they stay cells).
func (p *lyricsPic) ready(cell int, scale float64) bool {
	if p.failed || cell <= 0 {
		return false
	}
	if scale <= 0 {
		scale = 1
	}
	if p.font != nil && p.cell == cell && p.scale == scale {
		return true
	}
	big, err := gfx.LoadFont("sans-serif:bold", math.Round(lyricSize*scale*float64(cell)))
	if err != nil {
		p.failed = true
		return false
	}
	small, err := gfx.LoadFont("sans-serif", math.Round(lyricSmall*scale*float64(cell)))
	if err != nil {
		small = big
	}
	p.font, p.small, p.cell, p.scale = big, small, cell, scale
	p.blocks = map[string]*lyricBlock{}
	p.settled = false // the lines are other heights now: no glide to them
	return true
}

// lyricWrap breaks text into rows no wider than w pixels, at spaces where
// it has them.
func lyricWrap(f *gfx.Font, text string, w int) []string {
	var rows []string
	var row []rune
	rowW, lastSpace := 0, -1
	for _, r := range strings.TrimRight(text, " ") {
		adv := f.Glyph(r).Advance
		if rowW+adv > w && len(row) > 0 {
			cut := len(row)
			if lastSpace > 0 {
				cut = lastSpace
			}
			rows = append(rows, string(row[:cut]))
			rest := row[cut:]
			for len(rest) > 0 && unicode.IsSpace(rest[0]) {
				rest = rest[1:]
			}
			row = append([]rune{}, rest...)
			rowW, lastSpace = f.Width(string(row)), -1
		}
		if unicode.IsSpace(r) {
			lastSpace = len(row)
		}
		row = append(row, r)
		rowW += adv
	}
	if len(row) > 0 {
		rows = append(rows, string(row))
	}
	return rows
}

// drawRows draws text as rows of font f, no wider than w.
func drawRows(f *gfx.Font, text string, w int) []lyricRow {
	var out []lyricRow
	from := 0
	all := []rune(text)
	for _, part := range lyricWrap(f, text, w) {
		// Where this row starts in the line (the wrap dropped blanks).
		runes := []rune(part)
		for from < len(all) && len(runes) > 0 && all[from] != runes[0] {
			from++
		}
		rw := min(w, f.Width(part)+2)
		row := lyricRow{w: rw, h: f.Height, from: from, mask: make([]byte, rw*f.Height)}
		row.ends = f.Draw(row.mask, rw, f.Height, 0, f.Ascent, part)
		out = append(out, row)
		from += len(runes)
	}
	return out
}

// block is line drawn for a column w pixels wide.
func (p *lyricsPic) block(line views.CoverSceneLine, w int) *lyricBlock {
	key := line.Text + "\x00" + line.Translation
	if b, ok := p.blocks[key]; ok {
		return b
	}
	b := &lyricBlock{}
	if strings.TrimSpace(line.Text) == "" {
		b.height = p.font.Height * 6 / 10 // a pause in the song: a gap
	} else {
		b.rows = drawRows(p.font, line.Text, w)
		b.height = len(b.rows) * p.font.Height
		if line.Translation != "" {
			b.trans = drawRows(p.small, line.Translation, w)
			b.height += len(b.trans)*p.small.Height + p.small.Height/4
		}
	}
	p.blocks[key] = b
	return b
}

// lyricLook is the colours the lyrics are drawn in.
type lyricLook struct {
	fg, sung, pick vcol
}

// lyricPut is a row of words to put down: where, in what colours, and how
// far the voice's light has run along it (litX, in pixels; past the row
// for all of it, before it for none).
type lyricPut struct {
	row     *lyricRow
	y       int // down from the item's top
	c, c2   vcol
	a, a2   float64
	litX    float64
	noFade  bool // the edges' melting leaves it alone (the status line)
	litEdge float64
}

// lyricItem is one line of the song as it shows now (or the status line):
// its rows, where its top is down the picture, how tall it is, and sig,
// what it looks like apart from where it is (the same sig, the same
// pixels, wherever it is clear of the edges' melting).
type lyricItem struct {
	key  int // the line's number in the song; -1 the status
	y, h int
	puts []lyricPut
	sig  string
	text string // what it says (another text is another picture)
}

// crop is the part of the item inside a picture w×h: the rows y0 to y1,
// down the picture, and the first iw columns; none (y1 <= y0) when nothing
// of it shows, a line of the song wholly in the rows kept for the waves.
// (A line partly there is not cut: its picture keeps one size as it
// glides, and is only see-through there.)
func (it *lyricItem) crop(w, h int) (y0, y1, iw int) {
	y0, y1 = max(it.y, 0), min(it.y+it.h, h)
	if it.key >= 0 {
		fh := float64(h)
		if y1 <= int(math.Ceil(lyricClear*fh)) || y0 >= int(math.Floor(fh-lyricClear*fh)) {
			return 0, 0, 0
		}
	}
	for _, pt := range it.puts {
		iw = max(iw, pt.row.w)
	}
	return y0, y1, min(iw, w)
}

// lyricFade is how much of the words show at y pixels down a picture h
// high: they melt away towards the top and bottom edges, nothing at all in
// the first and last lyricClear of the height (where the waves run), then
// up to full strength.
func lyricFade(y, h int) float64 {
	fh := float64(h)
	return smooth(lyricClear*fh, 0.3*fh, float64(y)) * smooth(lyricClear*fh, 0.3*fh, float64(h-1-y))
}

// paint puts item down into dst (dw×dh pixels, see-through where there are
// no words), its top at dy in dst; y0 is where dst's first row is down the
// lyrics' picture, h that picture's height (for the edges' melting).
func (it *lyricItem) paint(dst []byte, dw, dh, dy, y0, h int) {
	for _, pt := range it.puts {
		row := pt.row
		for py := range row.h {
			Y := dy + pt.y + py // in dst
			if Y < 0 || Y >= dh {
				continue
			}
			ef := 1.0
			if !pt.noFade {
				if ef = lyricFade(y0+Y, h); ef < 0.004 {
					continue
				}
			}
			for px := range row.w {
				m := row.mask[py*row.w+px]
				if m == 0 || px >= dw {
					continue
				}
				k := 0.0
				if pt.litX > -1e8 {
					k = clamp01((pt.litX-float64(px))/pt.litEdge + 0.5)
				}
				col := pt.c.mix(pt.c2, k)
				a := (pt.a + (pt.a2-pt.a)*k) * ef * float64(m) / 255
				i := (Y*dw + px) * 4
				dst[i], dst[i+1], dst[i+2] = byte(255*clamp01(col.r)), byte(255*clamp01(col.g)), byte(255*clamp01(col.b))
				dst[i+3] = byte(255 * clamp01(a))
			}
		}
	}
}

// draw draws the scene into buf (w×h pixels, see-through where there are
// no words) and reports whether anything is still moving. dt is the time
// since the last frame.
func (p *lyricsPic) draw(buf []byte, w, h int, sc views.CoverScene, dt float64, look lyricLook) bool {
	_, _, moving := p.frame(buf, w, h, sc, dt, look, false)
	return moving
}

// frame works the lyrics out as they are now. With lines, they come back
// as items, a picture each (see sendLyricLines), and buf is left alone:
// a line that only glides is then only placed elsewhere, not drawn and
// sent again. When the whole picture has to be drawn instead (a word in
// place of the lyrics, one picture melting into the next, or lines false)
// it is drawn into buf and whole is true.
func (p *lyricsPic) frame(buf []byte, w, h int, sc views.CoverScene, dt float64, look lyricLook, lines bool) (items []lyricItem, whole, moving bool) {
	if w <= 0 || h <= 0 || p.font == nil {
		clear(buf)
		return nil, true, false
	}
	if sc.Message != "" {
		clear(buf)
		p.drawMessage(buf, w, h, sc, look)
		return nil, true, p.melt(buf, w, h, dt)
	}
	items, moving = p.scene(w, h, sc, dt, look)
	if p.id != p.shownID && p.shownID != "" && !p.lastOK && len(p.lastItems) > 0 {
		// It was shown a line a picture: the picture it melts from is
		// drawn once, as it was.
		p.last = p.paintAll(p.last[:0], w, h, p.lastItems)
		p.lastOK = true
	}
	if lines && p.id == p.shownID && !(len(p.from) == len(buf) && p.fade < 1) {
		p.lastItems, p.lastOK = items, false
		return items, false, moving
	}
	clear(buf)
	for i := range items {
		items[i].paint(buf, w, h, items[i].y, 0, h)
	}
	p.lastItems = items
	return items, true, p.melt(buf, w, h, dt) || moving
}

// paintAll draws items into buf, grown to w×h, as the whole picture.
func (p *lyricsPic) paintAll(buf []byte, w, h int, items []lyricItem) []byte {
	if cap(buf) < w*h*4 {
		buf = make([]byte, w*h*4)
	}
	buf = buf[:w*h*4]
	clear(buf)
	for i := range items {
		items[i].paint(buf, w, h, items[i].y, 0, h)
	}
	return buf
}

// melt blends what was shown before into buf while one picture melts into
// the next (another song's lyrics, a word in their place, the translation
// turned on or off), keeps buf as the picture last shown, and reports
// whether it is still melting.
func (p *lyricsPic) melt(buf []byte, w, h int, dt float64) bool {
	moving := false
	if p.id != p.shownID {
		if p.shownID != "" && p.lastOK && len(p.last) == len(buf) {
			p.from, p.fade = append(p.from[:0], p.last...), 0
		}
		p.shownID = p.id
	}
	if len(p.from) == len(buf) && p.fade < 1 {
		p.fade = math.Min(1, p.fade+dt/lyricFadeFor)
		k := p.fade * p.fade * (3 - 2*p.fade)
		par(len(buf)/4, func(i0, i1 int) {
			for i := i0 * 4; i < i1*4; i += 4 {
				na := float64(buf[i+3]) / 255 * k
				oa := float64(p.from[i+3]) / 255 * (1 - k)
				a := na + oa*(1-na)
				if a <= 0 {
					buf[i+3] = 0
					continue
				}
				for c := range 3 {
					buf[i+c] = byte((float64(buf[i+c])*na + float64(p.from[i+c])*oa*(1-na)) / a)
				}
				buf[i+3] = byte(255*a + 0.5)
			}
		})
		moving = true
	} else {
		p.from = p.from[:0]
	}
	p.last, p.lastOK = append(p.last[:0], buf...), true
	return moving
}

// drawMessage draws a word where the lyrics would be: loading, or nothing
// to sing.
func (p *lyricsPic) drawMessage(buf []byte, w, h int, sc views.CoverScene, look lyricLook) {
	p.id = "\x01" + sc.Message
	p.settled = false
	anchor := lyricAnchor * float64(h)
	y := int(anchor)
	var it lyricItem
	for _, row := range drawRows(p.small, sc.Message, w) {
		r := row
		it.puts = append(it.puts, lyricPut{row: &r, y: y, c: look.fg, a: 0.5, c2: look.fg, a2: 0.5, litX: -1e9, litEdge: 1})
		y += row.h
	}
	p.extTop, p.extBot = anchor, math.Max(float64(y), anchor+float64(p.font.Height))
	y += p.small.Height / 2
	for _, row := range drawRows(p.small, sc.Hint, w) {
		r := row
		it.puts = append(it.puts, lyricPut{row: &r, y: y, c: look.fg, a: 0.35, c2: look.fg, a2: 0.35, litX: -1e9, litEdge: 1})
		y += row.h
	}
	it.paint(buf, w, h, 0, 0, h)
}

// scene works out the lyrics as they are now, a line an item, and moves
// what moves (the text gliding to the line sung, the lines' light) on by
// dt. It reports whether anything is still moving.
func (p *lyricsPic) scene(w, h int, sc views.CoverScene, dt float64, look lyricLook) (items []lyricItem, moving bool) {
	if p.width != w {
		p.width = w
		p.blocks = map[string]*lyricBlock{}
		p.settled = false
	}
	anchor := lyricAnchor * float64(h)
	// The words count as readable between the edges' fades.
	// Everything that can be seen at all counts, however faint: the waves
	// must be clear of every word.
	fadeTop, fadeBot := (lyricClear+0.02)*float64(h), (1-lyricClear-0.02)*float64(h)
	p.extTop, p.extBot = fadeBot, fadeTop
	note := func(y0, y1 int) {
		if float64(y1) > fadeTop && float64(y0) < fadeBot {
			p.extTop = math.Min(p.extTop, math.Max(float64(y0), fadeTop))
			p.extBot = math.Max(p.extBot, math.Min(float64(y1), fadeBot))
		}
	}
	defer func() {
		if p.extBot < p.extTop { // no words at all
			p.extTop, p.extBot = anchor, anchor+float64(p.font.Height)
		}
	}()
	edge := 0.45 * p.font.Size

	// Another song's lyrics: start anew, without gliding there.
	var ofb strings.Builder
	for i := 0; i < len(sc.Lines) && i < 3; i++ {
		ofb.WriteString(sc.Lines[i].Text + "\n")
	}
	of := ofb.String()
	if of != p.of || len(p.lit) != len(sc.Lines) {
		p.of, p.settled = of, false
		p.lit = make([]float64, len(sc.Lines))
	}

	// Where each line starts, down the whole song.
	gap := p.font.Height * 55 / 100
	if len(p.tops) != len(sc.Lines) {
		p.tops = make([]int, len(sc.Lines))
	}
	y := 0
	for i, line := range sc.Lines {
		p.tops[i] = y
		y += p.block(line, w).height + gap
	}
	p.id = of + "\x00" + strconv.Itoa(y) // (the translation on or off changes the height)
	if len(sc.Lines) == 0 {
		return nil, false
	}
	focus := min(max(sc.Focus, 0), len(sc.Lines)-1)
	top := anchor
	if !sc.Synced && sc.Cursor < 0 {
		top = 0.1 * float64(h) // nothing to follow: read from the top
	}
	target := float64(p.tops[focus]) - top
	if !p.settled {
		p.scroll, p.vel, p.settled = target, 0, true
	}
	// The text glides to the new line on a spring that just doesn't
	// overshoot: it sets off gently, comes on and settles. (It eased out
	// at once, most of the way in the first frames: the text seemed to
	// jump.) A new line on the way carries on from where it is going.
	if d := target - p.scroll; math.Abs(d) > 0.3 || math.Abs(p.vel) > 2 {
		w := lyricGlide
		n := int(math.Ceil(dt / 0.004))
		h := dt / float64(n)
		for range n {
			p.vel += (w*w*(target-p.scroll) - 2*w*p.vel) * h
			p.scroll += p.vel * h
		}
		moving = true
	} else {
		p.scroll, p.vel = target, 0
	}

	q := func(v float64) int { return int(math.Round(v * 255)) }
	qc := func(c vcol) int { return q(c.r)<<16 | q(c.g)<<8 | q(c.b) }
	for i, line := range sc.Lines {
		b := p.block(line, w)
		y := int(math.Round(float64(p.tops[i]) - p.scroll))
		// Its light: full for the line being sung (or picked), eased.
		want := 0.0
		if i == sc.Current || i == sc.Cursor {
			want = 1
		}
		if d := want - p.lit[i]; math.Abs(d) > 0.01 {
			p.lit[i] += d * (1 - math.Exp(-dt*8))
			moving = true
		} else {
			p.lit[i] = want
		}
		if y+b.height < 0 || y > h {
			continue
		}
		if len(b.rows) > 0 {
			note(y, y+b.height)
		}
		// Lines sung are dimmer than the coming ones.
		base := 0.4
		if sc.Synced && sc.Current >= 0 && i < sc.Current {
			base = 0.26
		}
		lit := p.lit[i]
		rest, sung := base+(0.55-base)*lit, base+(1-base)*lit
		colRest, colSung := look.fg, look.fg.mix(look.sung, lit)
		if i == sc.Cursor && i != sc.Current {
			colRest, colSung = look.fg.mix(look.pick, lit), look.fg.mix(look.pick, lit)
			rest = sung
		}
		it := lyricItem{key: i, y: y, h: b.height, text: line.Text + "\x00" + line.Translation}
		var sig strings.Builder
		fmt.Fprintf(&sig, "%d %d %d %d|", qc(colRest), qc(colSung), q(rest), q(sung))
		ry := 0
		for r := range b.rows {
			row := &b.rows[r]
			litX := 1e9 // the whole row
			if i == sc.Current && sc.Lit >= 0 {
				n := sc.Lit - float64(row.from)
				switch {
				case n <= 0:
					litX = -float64(p.font.Size)
				case n < float64(len(row.ends)):
					k := int(n)
					start := 0
					if k > 0 {
						start = row.ends[k-1]
					}
					litX = float64(start) + (n-float64(k))*float64(row.ends[k]-start)
				}
			}
			it.puts = append(it.puts, lyricPut{row: row, y: ry, c: colRest, a: rest, litX: litX, c2: colSung, a2: sung, litEdge: edge})
			fmt.Fprintf(&sig, "%d ", int(math.Round(litX*4)))
			ry += row.h
		}
		if len(b.trans) > 0 {
			ry += p.small.Height / 4
			a := 0.62 * (base + (0.8-base)*lit)
			for r := range b.trans {
				row := &b.trans[r]
				it.puts = append(it.puts, lyricPut{row: row, y: ry, c: look.fg, a: a, litX: -1e9, c2: look.fg, a2: a, litEdge: 1})
				ry += row.h
			}
			fmt.Fprintf(&sig, "t%d", q(a))
		}
		it.sig = sig.String()
		items = append(items, it)
	}
	if sc.Status != "" {
		if rows := drawRows(p.small, sc.Status, w); len(rows) > 0 {
			row := rows[0] // one row of it, at the bottom, clear of the edge's fade
			a := 0.45
			items = append(items, lyricItem{key: -1, y: h - row.h - 2, h: row.h, text: sc.Status, sig: fmt.Sprint(qc(look.fg)),
				puts: []lyricPut{{row: &row, c: look.fg, a: a, c2: look.fg, a2: a, litX: -1e9, litEdge: 1, noFade: true}}})
		}
	}
	return items, moving
}
