package tui

import (
	"image"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// The frames of the panels and the cover, drawn as pictures (see lines.go).
// A frame is four lines: each is held at the frame's round corners and
// sways between them like a string, the song's colours run slowly round it,
// and a new one draws itself in from its top left corner. The cover is the
// picture itself, not cells, with a soft light behind it that breathes
// with the lows.

const (
	linesMaxBoxes = 4
	// linesFrameSway is how far a frame's line may leave its place, in
	// cells (its own row or column is all the room it has).
	linesFrameSway = 0.2
)

// The sides of a frame, in the order the colours run round it.
const (
	sideTop = iota
	sideRight
	sideBottom
	sideLeft
)

// lineBox is a frame found on the screen, in cells.
type lineBox struct {
	x0, y0, x1, y1 int
	focused        bool
	underOverlay   bool
	top, bottom    []bool // which cells of its top and bottom rows are line
}

// cellGrid lays a row of plain text out by cells: one rune a cell, 0 for
// the second half of a wide one.
func cellGrid(plain string) []rune {
	out := make([]rune, 0, len(plain))
	for _, r := range plain {
		out = append(out, r)
		if r >= 0x1100 && (r < 0x2190 || r > 0x2bff) && ansi.StringWidth(string(r)) == 2 {
			out = append(out, 0)
		}
	}
	return out
}

func gridAt(g [][]rune, x, y int) rune {
	if y < 0 || y >= len(g) || x < 0 || x >= len(g[y]) {
		return ' '
	}
	return g[y][x]
}

// linesBoxes finds the whole frames on a screen: ╭ … ╮ over sides of │
// down to ╰ … ╯. One that something lies over is left to the cells.
func linesBoxes(g [][]rune) []lineBox {
	var out []lineBox
	for y := range g {
		for x, r := range g[y] {
			if r != '╭' || len(out) >= linesMaxBoxes {
				continue
			}
			x1 := -1
			for k := x + 1; k < len(g[y]); k++ {
				if g[y][k] == '╮' {
					x1 = k
					break
				}
				if g[y][k] == '╭' {
					break
				}
			}
			if x1 < x+3 {
				continue
			}
			y1 := y + 1
			for gridAt(g, x, y1) == '│' && gridAt(g, x1, y1) == '│' {
				y1++
			}
			if y1 < y+2 || gridAt(g, x, y1) != '╰' || gridAt(g, x1, y1) != '╯' {
				continue
			}
			b := lineBox{x0: x, y0: y, x1: x1, y1: y1}
			for _, row := range []int{y, y1} {
				mask := make([]bool, x1-x+1)
				for k := range mask {
					switch gridAt(g, x+k, row) {
					case '─', '╭', '╮', '╰', '╯':
						mask[k] = true
					}
				}
				if row == y {
					b.top = mask
				} else {
					b.bottom = mask
				}
			}
			out = append(out, b)
		}
	}
	return out
}

// blankCells empties w cells of a styled row from the cell x on.
func blankCells(line string, x, w int) string {
	total := ansi.StringWidth(line)
	if x >= total {
		return line
	}
	w = min(w, total-x)
	return ansi.Cut(line, 0, x) + "\x1b[0m" + strings.Repeat(" ", w) + ansi.Cut(line, x+w, total)
}

// linesPlaceBoxes turns the frames of the screen into lines: their cells
// are emptied and their sides noted. rows is the screen, changed in place.
func (m *Model) linesPlaceBoxes(rows []string, grid [][]rune, now time.Time) bool {
	l := m.lines
	boxes := linesBoxes(grid)
	if l.baseFrames != "" {
		baseRows := strings.Split(l.baseFrames, "\n")
		baseGrid := make([][]rune, len(baseRows))
		for y, row := range baseRows {
			baseGrid[y] = cellGrid(ansi.Strip(row))
		}
		baseBoxes := linesBoxes(baseGrid)
		for i := range baseBoxes {
			baseBoxes[i].underOverlay = !l.overlayRect.empty()
		}
		for _, b := range boxes {
			duplicate := false
			for _, base := range baseBoxes {
				if b.x0 == base.x0 && b.y0 == base.y0 && b.x1 == base.x1 && b.y1 == base.y1 {
					duplicate = true
					break
				}
			}
			if !duplicate && len(baseBoxes) < linesMaxBoxes {
				baseBoxes = append(baseBoxes, b)
			}
		}
		boxes = baseBoxes
	}
	// Frames outside the visible screen cannot be represented by image strips.
	valid := boxes[:0]
	for _, b := range boxes {
		if b.x0 >= 0 && b.x1 < m.width && b.y0 >= 0 && b.y1 < len(rows) && b.y1 < len(grid) {
			valid = append(valid, b)
		}
	}
	boxes = valid
	seen := map[[4]int]time.Time{}
	// A frame in focus is told by the colour its corner is written in.
	// It is read back out of the row, not looked for as bytes: the same
	// colour is written in different bytes once the row has been through
	// the text effects (which happens whenever words are changing), and a
	// frame taken for unfocused every other moment flickers between its
	// two colours.
	var none [3]uint8
	corner := func(s string, x, w int) string { return parseCells(s, w, 1, none, none)[0][x].sgr }
	lit, unlit := corner(borderStyle(true).Render("╭"), 0, 1), corner(borderStyle(false).Render("╭"), 0, 1)
	// A corner in neither colour is one still written in the theme that
	// was there a moment ago (it is swept away with the words): the frame
	// stays as it was, or with a key held on the theme it flashes between
	// lit and plain with every step.
	lits := map[[4]int]bool{}
	// A frame that changed its size or place (a panel opening beside it, a
	// list growing) goes over from where it was to where it is, instead of
	// going and drawing itself in anew: each new frame is matched with the
	// one gone that covered most of the same cells (or, one gone and one
	// come, with that one).
	geos := map[[4]int]boxMorph{}
	var gone, come [][4]int
	for _, b := range boxes {
		if _, ok := l.boxGeo[[4]int{b.x0, b.y0, b.x1, b.y1}]; !ok {
			come = append(come, [4]int{b.x0, b.y0, b.x1, b.y1})
		}
	}
	for key := range l.boxGeo {
		still := false
		for _, b := range boxes {
			still = still || key == [4]int{b.x0, b.y0, b.x1, b.y1}
		}
		if !still {
			gone = append(gone, key)
		}
	}
	morphs := m.fxLevel() > 0
	// Match all changed frames together. Greedily walking destination
	// order lets a small cover frame steal an overlay from the much larger
	// panel it actually belongs to.
	type framePair struct {
		from, to [4]int
		overlap  int
	}
	var pairs []framePair
	for _, src := range gone {
		for _, dst := range come {
			area := overlapCells(src, dst)
			if area > 0 || len(gone) == 1 && len(come) == 1 {
				pairs = append(pairs, framePair{src, dst, area})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].overlap > pairs[j].overlap })
	sources := make(map[[4]int][4]int)
	usedSource := make(map[[4]int]bool)
	for _, pair := range pairs {
		if usedSource[pair.from] {
			continue
		}
		if _, taken := sources[pair.to]; taken {
			continue
		}
		usedSource[pair.from] = true
		sources[pair.to] = pair.from
	}
	// Each frame keeps its own four strips while it is there, and one going
	// over to another place takes those of the one it comes from: drawn by
	// the order of the frames on the screen, a frame that came or went
	// before it would hand its sides, with their colour and their picture
	// on the screen, to another.
	slots := map[[4]int]int{}
	used := [linesMaxBoxes]bool{}
	for _, b := range boxes {
		key := [4]int{b.x0, b.y0, b.x1, b.y1}
		if s, ok := l.boxSlot[key]; ok {
			if _, had := l.boxGeo[key]; had {
				slots[key], used[s] = s, true
			}
		}
	}
	for _, b := range boxes {
		key := [4]int{b.x0, b.y0, b.x1, b.y1}
		to := [4]float64{float64(b.x0), float64(b.y0 + m.hostRows), float64(b.x1), float64(b.y1 + m.hostRows)}
		geo, had := l.boxGeo[key]
		if !had {
			geo = boxMorph{from: to, to: to}
			if src, ok := sources[key]; ok && morphs {
				geo = boxMorph{from: l.boxGeo[src].at(now), to: to, start: now}
				if born, ok := l.boxes[src]; ok {
					l.boxes[key] = born // drawn in already: it only moves
				}
				// The new geometry inherits the source's focus while its
				// corner colour is still being composited or faded.
				if l.boxLit == nil {
					l.boxLit = make(map[[4]int]bool)
				}
				l.boxLit[key] = l.boxLit[src]
				if s, ok := l.boxSlot[src]; ok && !used[s] {
					slots[key], used[s] = s, true
				}

			}
		}
		geos[key] = geo
	}
	// In the cover view there may be no panel for a closing menu to return
	// to. Animate its own frame towards the centre instead of dropping it.
	if l.closingActive && (!l.closingFrame.moving(now) || m.songMenu != nil || m.linkOffer != nil || m.prompt != nil || m.mode == modePlaylistPicker) {
		l.closingActive = false
	}
	if morphs && l.menuShown && m.songMenu == nil && m.linkOffer == nil && m.prompt == nil && m.mode != modePlaylistPicker {
		menu := [4]int{l.menuKey[0], l.menuKey[1] - m.hostRows, l.menuKey[0] + l.menuKey[2] - 1, l.menuKey[1] - m.hostRows + l.menuKey[3] - 1}
		if geo, ok := l.boxGeo[menu]; ok && !usedSource[menu] {
			if slot, ok := l.boxSlot[menu]; ok && !used[slot] {
				from := geo.at(now)
				cx, cy := (from[0]+from[2])/2, (from[1]+from[3])/2
				l.closingFrame = boxMorph{from: from, to: [4]float64{cx, cy, cx, cy}, start: now}
				l.closingSlot, l.closingActive = slot, true
			}
		}
	}
	if l.closingActive {
		if used[l.closingSlot] {
			l.closingActive = false
		} else {
			used[l.closingSlot] = true
			for side := range 4 {
				st := &l.strips[stripFrame+l.closingSlot*4+side]
				st.seen, st.morph = true, l.closingFrame
			}
		}
	}
	for _, b := range boxes {
		key := [4]int{b.x0, b.y0, b.x1, b.y1}
		if _, ok := slots[key]; ok {
			continue
		}
		// A new one: a free place, one whose last frame has faded out
		// before one that is still fading.
		best := -1
		for s := range linesMaxBoxes {
			if used[s] {
				continue
			}
			if best < 0 || l.strips[stripFrame+best*4].shown && !l.strips[stripFrame+s*4].shown {
				best = s
			}
		}
		if best < 0 {
			continue // (no more frames are found than there are places for)
		}
		slots[key], used[best] = best, true //nolint:gosec // best is a place found free just above
	}
	for _, b := range boxes {
		key := [4]int{b.x0, b.y0, b.x1, b.y1}
		n, ok := slots[key]
		if !ok {
			continue
		}
		colourRow := rows[b.y0]
		if b.underOverlay {
			baseRows := strings.Split(l.baseFrames, "\n")
			if b.y0 < len(baseRows) {
				colourRow = baseRows[b.y0]
			}
		}
		switch c := corner(colourRow, b.x0, b.x1+1); {
		case lit != unlit && c == lit:
			b.focused = true
		case lit != unlit && c == unlit:
			b.focused = false
		default:
			b.focused = l.boxLit[key]
		}
		lits[key] = b.focused
		// Keep the destination's cell border while a smaller overlaid window
		// returns to it. The animated line meets it before the cells disappear.
		geo := geos[key]
		r := l.overlayRect
		isOverlay := !r.empty() && b.x0 == r.x0 && b.y0 == r.y0 && b.x1 == r.x1-1 && b.y1 == r.y1-1
		keepBorder := !isOverlay && geo.moving(now) && (geo.from[2]-geo.from[0])*(geo.from[3]-geo.from[1]) < (geo.to[2]-geo.to[0])*(geo.to[3]-geo.to[1])
		if keepBorder {
			// Fade the stationary cell border throughout the return instead
			// of removing it abruptly when the moving frame arrives.
			fg, bg, _ := styles.ScreenColors()
			foreground := [3]uint8{fg.R, fg.G, fg.B}
			background := [3]uint8{bg.R, bg.G, bg.B}
			alpha := 1 - smooth(0, linesMorphFor, now.Sub(geo.start).Seconds())
			for y := b.y0; y <= b.y1; y++ {
				cells := parseCells(rows[y], m.width, 1, foreground, background)
				for x := b.x0; x <= b.x1; x++ {
					border := (y == b.y0 && b.top[x-b.x0]) || (y == b.y1 && b.bottom[x-b.x0]) || (y > b.y0 && y < b.y1 && (x == b.x0 || x == b.x1))
					if border {
						c := &cells[0][x]
						c.fg = blend3(background, c.fg, alpha)
						c.mod = true
					}
				}
				rows[y] = encodeCells(cells, m.width, 1)
			}
		}
		if !keepBorder {
			// Empty the cells: the runs of line on the top and bottom rows,
			// and both sides.
			for _, e := range []struct {
				y    int
				mask []bool
			}{{b.y0, b.top}, {b.y1, b.bottom}} {
				for k := 0; k < len(e.mask); k++ {
					covered := func(x int) bool {
						r := l.overlayRect
						return b.underOverlay && x >= r.x0 && x < r.x1 && e.y >= r.y0 && e.y < r.y1
					}
					if !e.mask[k] || covered(b.x0+k) {
						continue
					}
					from := k
					for k < len(e.mask) && e.mask[k] && !covered(b.x0+k) {
						k++
					}
					rows[e.y] = blankCells(rows[e.y], b.x0+from, k-from)
				}
			}
			for y := b.y0 + 1; y < b.y1; y++ {
				for _, x := range []int{b.x0, b.x1} {
					r := l.overlayRect
					if b.underOverlay && x >= r.x0 && x < r.x1 && y >= r.y0 && y < r.y1 {
						continue
					}
					rows[y] = blankCells(rows[y], x, 1)
					if x < len(grid[y]) {
						grid[y][x] = ' '
					}
				}
			}
		}

		born, old := l.boxes[key]
		if !old {
			born = now
		}
		seen[key] = born
		W, H := b.x1-b.x0+1, b.y1-b.y0+1
		for side := range 4 {
			st := &l.strips[stripFrame+n*4+side]
			z := -1
			if b.underOverlay {
				z = -3
			}
			if st.img.Z != z {
				st.img.Z, st.sentKey = z, [5]int{-1}
			}
			st.morph = geos[key]
			st.seen, st.born, st.focused, st.side = true, born, b.focused, side
			st.boxW, st.boxH = W, H
			// A cell of room each side of the line, where the screen has
			// it: a touch bends the line further than its own cell.
			room := func(at, last int) (before, n int) {
				before = min(1, at)
				return before, before + 1 + min(1, last-at)
			}
			lastRow, lastCol := m.height+m.hostRows-1, m.width-1
			switch side {
			case sideTop, sideBottom:
				y, mask := b.y0+m.hostRows, b.top
				if side == sideBottom {
					y, mask = b.y1+m.hostRows, b.bottom
				}
				pad, n := room(y, lastRow)
				st.col, st.row, st.cols, st.rows, st.mask, st.pad = b.x0, y-pad, W, n, mask, pad
			default:
				x := b.x0
				if side == sideRight {
					x = b.x1
				}
				pad, n := room(x, lastCol)
				st.col, st.row, st.cols, st.rows, st.mask, st.pad = x-pad, b.y0+1+m.hostRows, n, H-2, nil, pad
			}
		}
	}

	l.boxes, l.boxLit, l.boxGeo, l.boxSlot = seen, lits, geos, slots
	return len(boxes) > 0
}

// boxMorph is a frame going over from one place and size to another: its
// corner cells (x0, y0, x1, y1, on the whole terminal) from and to, since
// start. A frame that stays has from equal to to.
type boxMorph struct {
	from, to [4]float64
	start    time.Time
}

// linesMorphFor is how long a frame takes to go over to its new place.
const linesMorphFor = 0.42

// at is where the frame's corners are at now.
func (b boxMorph) at(now time.Time) [4]float64 {
	p := clamp01(now.Sub(b.start).Seconds() / linesMorphFor)
	if p >= 1 || b.from == b.to {
		return b.to
	}
	// Out quickly and settling, never past where it goes: a frame that
	// went a touch past and back crossed the lines beside it on the way,
	// and one sent on again while it came back turned round sharply.
	q := 1 - p
	k := 1 - q*q*q
	var r [4]float64
	for i := range r {
		r[i] = b.from[i] + (b.to[i]-b.from[i])*k
	}
	return r
}

// moving reports whether the frame is still on its way.
func (b boxMorph) moving(now time.Time) bool {
	return b.from != b.to && now.Sub(b.start).Seconds() < linesMorphFor
}

// overlapCells is how many cells two frames (x0, y0, x1, y1) share.
func overlapCells(a, b [4]int) int {
	w := min(a[2], b[2]) - max(a[0], b[0]) + 1
	h := min(a[3], b[3]) - max(a[1], b[1]) + 1
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// morphSide places a side of a frame on its way (see boxMorph) by the
// pixel: where its picture goes and how big it is, and the mask of the top
// or bottom row there (a title is still left out where it will be).
func (l *uiLines) morphSide(st *lineStrip, r [4]float64) (x, y, w, h int, mask []bool) {
	cw, ch := float64(l.cellW), float64(l.cellH)
	pad := float64(st.pad)
	var fx, fy, fw, fh float64
	switch st.side {
	case sideTop, sideBottom:
		row := r[1]
		if st.side == sideBottom {
			row = r[3]
		}
		fx, fy = r[0]*cw, (row-pad)*ch
		fw, fh = (r[2]-r[0]+1)*cw, float64(st.rows)*ch
	default:
		col := r[0]
		if st.side == sideRight {
			col = r[2]
		}
		fx, fy = (col-pad)*cw, (r[1]+1)*ch
		fw, fh = float64(st.cols)*cw, (r[3]-r[1]-1)*ch
	}
	x, y = max(0, int(math.Round(fx))), max(0, int(math.Round(fy)))
	w, h = max(1, int(math.Round(fw))), max(1, int(math.Round(fh)))
	if st.side == sideTop || st.side == sideBottom {
		// The title's gap where the title is (it is written there
		// already), on the row where the line ends up.
		final := st.morph.to
		row, endRow := r[1], final[1]
		if st.side == sideBottom {
			row, endRow = r[3], final[3]
		}
		cells := int(math.Ceil(fw / cw))
		mask = make([]bool, cells)
		x0 := int(math.Round(r[0]))
		for c := range mask {
			mask[c] = true
			if i := x0 + c - int(final[0]); math.Abs(row-endRow) < 0.5 && i >= 0 && i < len(st.mask) {
				mask[c] = st.mask[i]
			}
		}
	}
	return x, y, w, h, mask
}

// maskKey is a number that changes when the mask does.
func maskKey(mask []bool) int {
	k := len(mask)
	for i, on := range mask {
		if on {
			k = k*31 + i + 1
		}
	}
	return k & 0xffffff
}

// frameLook is how a frame's lines are drawn right now.
type frameLook struct {
	amp  float64 // how far they sway, 0..1
	kick float64 // the drum: the line thickens and brightens for a moment, in its place
	pad  int     // cells of room in the picture before the line, across it
	// bend is how far a touch pushes the line out of its place (see
	// uiLines.frameBend); nil when nothing does.
	bend    func(side int, u, s float64) float64
	t       float64
	reveal  float64 // below 1: still drawing itself in
	focused bool
	lit     float64 // how much of the focus colour it wears, 0..1
	pal     [3]vcol
	fg      vcol
}

// frameColour is the colour of a frame's line at s, 0..1 round it.
func (f frameLook) colour(s float64) vcol {
	plain := f.fg.mul(0.6)
	if f.lit <= 0 {
		return plain
	}
	x := math.Mod(s*3+f.t*0.07, 3)
	if x < 0 {
		x += 3
	}
	i := int(x)
	k := x - float64(i)
	k = k * k * (3 - 2*k)
	return plain.mix(f.pal[i%3].mix(f.pal[(i+1)%3], k).mix(vcol{1, 1, 1}, 0.2), f.lit)
}

// drawFrameSide draws one side of a frame boxW×boxH cells into buf (w×h
// pixels: the side's row of cells with the corners, or its column without
// them). cw and chh are a cell's size in pixels. mask says which cells of a
// row are line (a title set into it is not).
func drawFrameSide(buf []byte, w, h, side, boxW, boxH int, mask []bool, cw, chh float64, f frameLook) {
	clear(buf)
	if w <= 0 || h <= 0 {
		return
	}
	core := math.Max(0.8, chh/22) * (1 + 0.2*f.amp + 0.45*f.kick)
	strong := 0.6 + 0.25*f.lit + (0.25-0.1*f.lit)*f.kick
	kc := math.Exp(-2 / (core * core))
	var win [64]float64 // the line across, a few pixels each side (see gaussRow)
	// Where a point of this side is on the way round the frame, in cells,
	// from the top left corner, clockwise.
	per := float64(2 * (boxW + boxH))
	// at is the colour at s round the frame and how much of the frame is
	// there yet (a new one draws itself in both ways round from its
	// corner): the same for a whole row or column of pixels across the
	// line, so worked out once for it.
	type spot struct {
		r, g, b byte
		k       float64
	}
	at := func(s float64) spot {
		way := math.Min(s, per-s) / (per / 2)
		k := smooth(0, 0.08, f.reveal*1.1-way)
		if k < 0.004 {
			return spot{}
		}
		c := f.colour(s / per)
		return spot{byte(255 * clamp01(c.r)), byte(255 * clamp01(c.g)), byte(255 * clamp01(c.b)), k}
	}
	put := func(x, y int, a float64, p spot) {
		a *= p.k
		if a < 0.004 {
			return
		}
		i := (y*w + x) * 4
		if a255 := byte(255 * math.Min(1, a)); a255 > buf[i+3] {
			buf[i], buf[i+1], buf[i+2], buf[i+3] = p.r, p.g, p.b, a255
		}
	}
	phase := float64(side) * 1.9
	if side == sideLeft || side == sideRight {
		// A column: x sways, held at both ends.
		mid := float64(f.pad)*cw + cw/2 - 0.5
		sway := f.amp * linesFrameSway * cw
		out := 1.0
		if side == sideLeft {
			out = -1
		}
		for y := range h {
			fy := float64(y)
			u := fy / chh
			env := smooth(0, 2.5*chh, fy) * smooth(0, 2.5*chh, float64(h-1)-fy)
			x := mid + env*sway*(0.7*math.Sin(u*0.33-f.t*1.0+phase)+0.3*math.Sin(u*0.71+f.t*0.6+phase))
			s := float64(boxW) + 1 + u
			if side == sideLeft {
				s = per - 1 - u
			}
			if f.bend != nil {
				x += env * out * cw * f.bend(side, fy/float64(max(1, h-1)), s/per)
			}
			sp := at(s)
			if sp.k == 0 {
				continue
			}
			lo, hi := max(0, int(x-3*core)), min(w-1, int(x+3*core)+1)
			if hi-lo+1 > len(win) {
				hi = lo + len(win) - 1
			}
			if hi >= lo {
				gaussRow(win[:hi-lo+1], x-float64(lo), core, kc)
			}
			for px := lo; px <= hi; px++ {
				put(px, y, strong*win[px-lo], sp)
			}
		}
		return
	}
	// A row: y sways between the corners; in the corner cells the line
	// turns down (or up) into the side through a quarter of a circle.
	mid := float64(f.pad)*chh + chh/2 - 0.5
	sway := f.amp * linesFrameSway * chh
	r := cw / 2 // the corner's radius
	dir := 1.0  // the sides are below the top row
	if side == sideBottom {
		dir = -1
	}
	gap := make([]float64, w) // how much of the line there is at x: none in a title
	for x := range w {
		c := int(float64(x) / cw)
		if c < len(mask) && mask[c] {
			gap[x] = 1
		}
	}
	// Fade towards a title instead of stopping dead.
	fade := int(0.7 * cw)
	for x := range w {
		if gap[x] == 0 {
			continue
		}
		for d := 1; d <= fade; d++ {
			if (x-d >= 0 && gap[x-d] == 0) || (x+d < w && gap[x+d] == 0) {
				gap[x] = float64(d) / float64(fade+1)
				break
			}
		}
	}
	for x := range w {
		fx := float64(x)
		if gap[x] == 0 {
			continue
		}
		s := fx / cw
		if side == sideBottom {
			s = float64(boxW+boxH) + (float64(w)-fx)/cw
		}
		sp := at(s)
		if sp.k == 0 {
			continue
		}
		fromEnd := math.Min(fx, float64(w-1)-fx)
		if fromEnd < cw {
			// A corner cell: the arc's centre is r in from the column's
			// middle and r towards the sides from the row's middle.
			ax := (cw - 1) / 2
			cx := ax + r
			if fx > float64(w)/2 {
				ax = float64(w) - cw + (cw-1)/2
				cx = ax - r
			}
			cy := mid + dir*r
			// Only the row's own cells: in the room beyond them the side's
			// own line runs, and a second, still one beside it would show
			// whenever the side moves.
			ownTop, ownBot := float64(f.pad)*chh, float64(f.pad+1)*chh
			for y := range h {
				fy := float64(y)
				if fy < ownTop-0.5 || fy > ownBot-0.5 {
					continue
				}
				var d float64
				switch {
				case (fy-cy)*dir > 0:
					d = math.Abs(fx - ax) // the stub that runs on into the side
				case (fx-cx)*(ax-cx) < 0:
					d = math.Abs(fy - mid)
				default:
					d = math.Abs(math.Hypot(fx-cx, fy-cy) - r)
				}
				put(x, y, strong*gap[x]*math.Exp(-sq(d/core)), sp)
			}
			continue
		}
		u := fx / chh
		env := smooth(cw, cw+2.5*chh, fx) * smooth(cw, cw+2.5*chh, float64(w-1)-fx)
		y := mid + env*sway*(0.7*math.Sin(u*0.17-f.t*1.1+phase)+0.3*math.Sin(u*0.39+f.t*0.7+phase))
		if f.bend != nil {
			y -= env * dir * chh * f.bend(side, fx/float64(max(1, w-1)), s/per)
		}
		lo, hi := max(0, int(y-3*core)), min(h-1, int(y+3*core)+1)
		if hi-lo+1 > len(win) {
			hi = lo + len(win) - 1
		}
		if hi >= lo {
			gaussRow(win[:hi-lo+1], y-float64(lo), core, kc)
		}
		for py := lo; py <= hi; py++ {
			put(x, py, strong*gap[x]*win[py-lo], sp)
		}
	}
}

// drawCover scales img into buf (w×h pixels), with round corners of radius
// rad.
// coverPixels is the picture's own pixels as numbers, kept for the last
// picture asked for: reading them through image.At is most of the work of
// drawing the cover at a new size, and the cover is drawn at a new size on
// the frame the view changes.
func coverPixels(img image.Image) []float32 {
	coverSrcMu.Lock()
	defer coverSrcMu.Unlock()
	if coverSrcImg == img && coverSrc != nil {
		return coverSrc
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	src := make([]float32, sw*sh*3)
	for y := range sh {
		for x := range sw {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := (y*sw + x) * 3
			src[i], src[i+1], src[i+2] = float32(r>>8), float32(g>>8), float32(bl>>8)
		}
	}
	coverSrcImg, coverSrc = img, src
	return src
}

var (
	coverSrcMu  sync.Mutex
	coverSrcImg image.Image
	coverSrc    []float32
)

func drawCover(buf []byte, w, h int, img image.Image, rad float64) {
	b := img.Bounds()
	if w <= 0 || h <= 0 || b.Dx() <= 0 || b.Dy() <= 0 {
		return
	}
	sw, sh := b.Dx(), b.Dy()
	src := coverPixels(img)
	par(h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			// Each pixel is the mean of the source pixels it covers (a
			// smaller picture), or the blend of the four around it.
			fy0, fy1 := float64(y)*float64(sh)/float64(h), float64(y+1)*float64(sh)/float64(h)
			for x := range w {
				fx0, fx1 := float64(x)*float64(sw)/float64(w), float64(x+1)*float64(sw)/float64(w)
				var cr, cg, cb float32
				if fx1-fx0 >= 1 && fy1-fy0 >= 1 {
					n := float32(0)
					for sy := int(fy0); sy < min(sh, int(math.Ceil(fy1))); sy++ {
						for sx := int(fx0); sx < min(sw, int(math.Ceil(fx1))); sx++ {
							i := (sy*sw + sx) * 3
							cr, cg, cb, n = cr+src[i], cg+src[i+1], cb+src[i+2], n+1
						}
					}
					cr, cg, cb = cr/n, cg/n, cb/n
				} else {
					px, py := (fx0+fx1)/2-0.5, (fy0+fy1)/2-0.5
					xa, ya := max(0, min(sw-1, int(math.Floor(px)))), max(0, min(sh-1, int(math.Floor(py))))
					xb, yb := min(sw-1, xa+1), min(sh-1, ya+1)
					kx, ky := float32(clamp01(px-float64(xa))), float32(clamp01(py-float64(ya)))
					at := func(sx, sy, c int) float32 { return src[(sy*sw+sx)*3+c] }
					mixc := func(c int) float32 {
						top := at(xa, ya, c)*(1-kx) + at(xb, ya, c)*kx
						bot := at(xa, yb, c)*(1-kx) + at(xb, yb, c)*kx
						return top*(1-ky) + bot*ky
					}
					cr, cg, cb = mixc(0), mixc(1), mixc(2)
				}
				// Round corners.
				a := 1.0
				dx := math.Max(0, math.Max(rad-float64(x)-0.5, float64(x)+0.5-(float64(w)-rad)))
				dy := math.Max(0, math.Max(rad-float64(y)-0.5, float64(y)+0.5-(float64(h)-rad)))
				if dx > 0 && dy > 0 {
					a = clamp01(rad - math.Hypot(dx, dy) + 0.5)
				}
				i := (y*w + x) * 4
				buf[i], buf[i+1], buf[i+2], buf[i+3] = byte(cr), byte(cg), byte(cb), byte(255*a)
			}
		}
	})
}

// linesCoverMark stands in the first cell of a cover that is to be drawn as
// a picture, where the layout doesn't know its own place on the screen:
// the place is found by it.
const linesCoverMark = '\ue000'

// linesPlaceMarks finds what is left on the screen after the frames: the
// mark of a cover's place, and the mark of the chosen row (the │ a list
// puts before it, or ▸), which becomes a drop of light that flows from row
// to row. rows is the screen, changed in place.
func (m *Model) linesPlaceMarks(rows []string, grid [][]rune, cover *bool) bool {
	l := m.lines
	changed := false
	selX, selY, selN := -1, 0, 0
	arrow := false
	// Runs of rows whose only │ is in one column: a list's mark is a short
	// one; a long one is the side of a frame that something lies over.
	runX, runY, runN := -1, 0, 0
	inOverlay := func(x, y int) bool {
		r := l.overlayRect
		return !r.empty() && x >= r.x0 && x < r.x1 && y >= r.y0 && y < r.y1
	}
	endRun := func() {
		if runN >= 1 && runN <= 3 && (selX < 0 || inOverlay(runX, runY) && !inOverlay(selX, selY)) {
			selX, selY, selN = runX, runY, runN
		}
		runX, runN = -1, 0
	}
	for y, g := range grid {
		bars, barX := 0, -1
		for x, r := range g {
			switch r {
			case linesCoverMark:
				rows[y] = blankCells(rows[y], x, 1)
				changed = true
				if *cover {
					l.coverCol, l.coverRow = x, y
				}
			case linesCtlMark:
				rows[y] = blankCells(rows[y], x, 1)
				changed = true
				if y >= 1 && y+1 < len(rows) {
					st := &l.strips[stripCtl]
					st.seen, st.col, st.row, st.cols, st.rows, st.scale = true, x, y-1+m.hostRows, linesCtlCols, linesCtlRows, 1
					if l.songBig {
						// Larger, round the same middle (the rows over and
						// under them are left clear for it).
						cols := int(math.Round(linesCtlCols * songBigCtl))
						rows := int(math.Ceil(linesCtlRows*songBigCtl)) | 1
						st.col, st.row = x-(cols-linesCtlCols)/2, y-rows/2+m.hostRows
						st.cols, st.rows, st.scale = cols, rows, songBigCtl
					}
				}
			case '│':
				bars++
				barX = x
			case '▸':
				// A menu's own mark comes before a list's under it.
				if !arrow && gridAt(grid, x+1, y) == ' ' {
					selX, selY, selN, arrow = x, y, 1, true
				}
			default:
				// A label's mark stays for now: the progress and volume
				// lines are told by the labels beside them (linesPlace
				// empties the marks when it has found those).
				if r >= labelMark0 && r <= labelMarkEnd {
					changed = true
					if n := int(r - labelMark0); n < len(l.labels) {
						l.labels[n].x, l.labels[n].y, l.labels[n].found = x, y, true
					}
				}
			}
		}
		if bars == 1 && gridAt(grid, barX+1, y) == ' ' {
			if barX == runX && y == runY+runN {
				runN++
				continue
			}
			endRun()
			runX, runY, runN = barX, y, 1
		} else {
			endRun()
		}
	}
	endRun()
	st := &l.strips[stripSel]
	if selX < 0 || !m.pixLines() {
		l.selOn = false
		return changed
	}
	// Popup backgrounds sit above ordinary graphics. Lift the popup's
	// selection mark above that background, then restore its normal layer
	// when focus returns to the underlying list.
	z := -3
	if inOverlay(selX, selY) {
		z = -1
	}
	if st.img.Z != z {
		st.img.Z, st.sentKey = z, [5]int{-1}
	}
	for y := selY; y < selY+selN; y++ {
		rows[y] = blankCells(rows[y], selX, 1)
	}
	if l.cellH <= 0 {
		l.cellW, l.cellH = 9, 18
		if w, h := cellPixels(); w > 0 && h > 0 {
			l.cellW, l.cellH = w, h
		}
	}
	if selX != l.selCol {
		l.selOn = false // another list: the mark does not fly across the screen
	}
	l.selCol = selX
	chh := float64(l.cellH)
	l.selTop = (float64(selY+m.hostRows) + 0.14) * chh
	l.selBot = (float64(selY+selN+m.hostRows) - 0.14) * chh
	st.seen, st.col, st.row, st.cols, st.rows = true, selX, 0, 1, m.height+m.hostRows
	return true
}

// drawSel draws the mark of the chosen row into buf (w×h pixels, a column
// of cells the height of the screen): a short upright stroke with round
// ends from a to b pixels down, with a little light round it that breathes
// with the lows. chh is a cell's height in pixels.
func drawSel(buf []byte, w, h int, a, b, chh, level float64, c vcol) {
	clear(buf)
	if w <= 0 || h <= 0 || b <= a {
		return
	}
	mid := float64(w)/2 - 0.5
	half := math.Max(1.1, chh/15)
	glow := math.Min(float64(w)/2-0.5, 2.6*half)
	c = c.mix(vcol{1, 1, 1}, 0.25)
	r, g, bl := byte(255*clamp01(c.r)), byte(255*clamp01(c.g)), byte(255*clamp01(c.b))
	for y := max(0, int(a-glow)-1); y <= min(h-1, int(b+glow)+1); y++ {
		fy := float64(y)
		dy := math.Max(0, math.Max(a+half-fy, fy-(b-half)))
		for x := range w {
			d := math.Hypot(float64(x)-mid, dy)
			al := clamp01(half-d+0.5) + (0.12+0.3*level)*math.Exp(-sq(d/glow))*clamp01((glow-d)/glow*2)
			if al < 0.004 {
				continue
			}
			i := (y*w + x) * 4
			buf[i], buf[i+1], buf[i+2], buf[i+3] = r, g, bl, byte(255*math.Min(1, al))
		}
	}
}

// drawWave draws one of the two waves that lie over and under the lyrics
// into buf (w×h pixels, three rows of cells): a single fine line in one of
// the song's colours with a breath of light round it, on the visualizer's
// ribbon curve. swing (0..1) is how far it leaves its place, level how
// loud its voice is, reveal below 1 a wave still drawing itself in. It
// fades out towards both ends and never reaches the picture's edges.
func drawWave(buf []byte, w, h int, t, swing, level, reveal float64, n int, c vcol) {
	clear(buf)
	if w <= 0 || h <= 0 {
		return
	}
	fw, fh := float64(w), float64(h)
	fn := float64(n)
	mid := fh/2 - 0.5
	reach := fh * 0.3 // how far it swings
	core := math.Max(0.8, fh/60)
	glow := fh * 0.13
	c = c.mix(vcol{1, 1, 1}, 0.2)
	r, g, b := byte(255*clamp01(c.r)), byte(255*clamp01(c.g)), byte(255*clamp01(c.b))
	upTo := reveal * fw * 1.2
	for x := range w {
		fx := float64(x)
		k := smooth(0, 0.25*fw, fx) * smooth(0, 0.25*fw, fw-1-fx) * smooth(0, 0.2*fw, upTo-fx)
		if k < 0.004 {
			continue
		}
		u := fx / fh * 0.55
		// It swings as far as the music is loud, and a drum beat sends a
		// fine shiver along it; its light stays as it is.
		y := mid + reach*(swing*ribbonWave(u, t, 0.9+0.23*fn, 0.5+0.17*fn, fn*1.7)+
			0.16*clamp01(level)*math.Sin(u*7+t*5+fn*2))
		const lit = 0.09
		for py := max(1, int(y-2.2*glow)); py <= min(h-2, int(y+2.2*glow)+1); py++ {
			d := math.Abs(float64(py) - y)
			edge := smooth(0, fh*0.12, math.Min(float64(py), fh-1-float64(py)))
			a := k * edge * (0.55*math.Exp(-sq(d/core)) + lit*math.Exp(-sq(d/glow)))
			if a < 0.004 {
				continue
			}
			i := (py*w + x) * 4
			buf[i], buf[i+1], buf[i+2], buf[i+3] = r, g, b, byte(255*math.Min(1, a))
		}
	}
}

// haloShape is the light behind the cover, at the screen's own pixels. Its
// outline changes only with the cover's size, so how far each pixel is
// from the cover and which way it lies are worked out once and kept.
type haloShape struct {
	w, h, mx, my int
	rad          float64
	far          []float32 // 0 at the cover's outline, 1 where the light ends; -1 well under the cover
	turn         []uint8   // the direction from the cover's middle, as 0..255 of a turn
}

// draw draws the light into buf (w×h pixels): the cover is the rectangle
// mx, my in from each edge, with corners of radius rad, and the light
// spreads out of its outline in the song's colours, which turn slowly
// round it. level (0..1) is how strong it is: it breathes with the lows.
// It lies under the cover's round corners too, and nothing reaches the
// picture's edges.
func (s *haloShape) draw(buf []byte, w, h, mx, my int, rad, t, level, gain float64, pal [3]vcol) {
	clear(buf)
	if w <= 0 || h <= 0 || mx <= 0 || my <= 0 {
		return
	}
	if s.w != w || s.h != h || s.mx != mx || s.my != my || s.rad != rad || len(s.far) != w*h {
		s.w, s.h, s.mx, s.my, s.rad = w, h, mx, my, rad
		s.far, s.turn = make([]float32, w*h), make([]uint8, w*h)
		cx, cy := float64(w)/2, float64(h)/2
		reach := float64(min(mx, my))
		par(h, func(y0, y1 int) {
			for y := y0; y < y1; y++ {
				fy := float64(y) + 0.5
				for x := range w {
					fx := float64(x) + 0.5
					i := y*w + x
					dx := math.Max(float64(mx)+rad-fx, fx-(float64(w-mx)-rad))
					dy := math.Max(float64(my)+rad-fy, fy-(float64(h-my)-rad))
					s.turn[i] = uint8(int((math.Atan2(fy-cy, fx-cx)/(2*math.Pi)+0.5)*256) & 255)
					if dx < -2 && dy < -2 || (dx < -rad-2 || dy < -rad-2) && dx <= 0 && dy <= 0 {
						s.far[i] = -1 // the cover hides it
						continue
					}
					d := math.Hypot(math.Max(0, dx), math.Max(0, dy)) - rad
					s.far[i] = float32(math.Max(0, d) / reach)
				}
			}
		})
	}
	// The colours round the cover, turned to where they are now.
	var ring [256][3]byte
	white := vcol{1, 1, 1}
	for i := range ring {
		p := math.Mod((float64(i)/256+t*0.03)*3, 3)
		i0 := int(p)
		k := p - float64(i0)
		k = k * k * (3 - 2*k)
		c := pal[i0%3].mix(pal[(i0+1)%3], k).mix(white, 0.1)
		ring[i] = [3]byte{byte(255 * clamp01(c.r)), byte(255 * clamp01(c.g)), byte(255 * clamp01(c.b))}
	}
	top := 255 * (0.05 + 0.2*level) * clamp01(gain)
	if top < 0.5 {
		return
	}
	bayer := [16]float64{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}
	par(h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := range w {
				i := y*w + x
				d := float64(s.far[i])
				c := ring[s.turn[i]]
				j := i * 4
				// Its colour even where none of it shows: the terminal
				// blends neighbouring pixels as it stretches the picture,
				// and a clear black pixel next to the light darkened its
				// edge.
				buf[j], buf[j+1], buf[j+2] = c[0], c[1], c[2]
				if d < 0 || d >= 1 {
					continue
				}
				k := (1 - d) * (1 - d)
				buf[j+3] = byte(math.Min(255, top*k*k+bayer[(y&3)<<2|x&3]/16))
			}
		}
	})
}
