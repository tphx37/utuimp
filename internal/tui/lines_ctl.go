package tui

import (
	"math"
	"time"

	"github.com/tphx37/utuimp/internal/player"
)

// The player's buttons (repeat, shuffle, play or pause, favourite) drawn as
// pictures, in the lines' own hand: fine strokes, three rows of cells high,
// in the song's colours when they are on. A button that changes springs for
// a moment; a ring round the play button breathes with the drum. In cells
// (the simple style) they are the characters they always were.

const (
	// linesCtlMark stands in the first cell of the buttons' row, to tell
	// where they go (like linesCoverMark).
	linesCtlMark = ''
	// linesCtlCols is how many cells wide the four buttons are together.
	linesCtlCols = 24
	linesCtlRows = 3
)

// ctlState is what the buttons show.
type ctlState struct {
	repeat           int // 0 off, 1 all, 2 one
	shuffle          bool
	playing, loading bool
	fav              bool
}

// ctlLook is how the buttons are drawn right now.
type ctlLook struct {
	ctlState
	pop  [4]float64 // each button's spring, about -1..1
	kick float64
	spin float64 // the loading ring's turn
	pal  [3]vcol
	fg   vcol
}

// linesCtlState reads the buttons' state off the player.
func (m *Model) linesCtlState() ctlState {
	s := ctlState{
		shuffle: m.playerState.ShuffleMode,
		playing: m.playerState.Playing,
		loading: m.playerState.Loading,
	}
	switch m.playerState.RepeatMode {
	case player.RepeatModeAll:
		s.repeat = 1
	case player.RepeatModeOne:
		s.repeat = 2
	}
	if t := m.playerState.Track; t != nil {
		s.fav = m.isTrackFavorite(t)
	}
	return s
}

// ctlPops notes which buttons have just changed and returns each one's
// spring.
func (l *uiLines) ctlPops(s ctlState, now time.Time) (pop [4]float64, live bool) {
	was := l.ctl
	if l.ctlKnown {
		if was.repeat != s.repeat {
			l.ctlTouch(0, now)
		}
		if was.shuffle != s.shuffle {
			l.ctlTouch(1, now)
		}
		if was.playing != s.playing {
			l.ctlTouch(2, now)
		}
		if was.fav != s.fav {
			l.ctlTouch(3, now)
		}
	}
	l.ctl, l.ctlKnown = s, true
	for i := range pop {
		if age := now.Sub(l.ctlAt[i]).Seconds(); age < linesTouchFor {
			pop[i] = spring(age)
			live = true
		}
	}
	return pop, live
}

// sdSeg is the distance from p to the stroke from a to b.
func sdSeg(px, py, ax, ay, bx, by float64) float64 {
	vx, vy := bx-ax, by-ay
	k := clamp01(((px-ax)*vx + (py-ay)*vy) / (vx*vx + vy*vy))
	return math.Hypot(px-ax-k*vx, py-ay-k*vy)
}

// sdHeart is the distance from p to the edge of a heart whose tip is at
// the origin and which is one unit tall (y up); negative inside.
func sdHeart(x, y float64) float64 {
	x = math.Abs(x)
	if y+x > 1 {
		return math.Hypot(x-0.25, y-0.75) - math.Sqrt2/4
	}
	k := 0.5 * math.Max(x+y, 0)
	d := math.Sqrt(math.Min(x*x+(y-1)*(y-1), (x-k)*(x-k)+(y-k)*(y-k)))
	if x-y < 0 {
		return -d
	}
	return d
}

// drawControls draws the four buttons into buf (w×h pixels). cell is a
// cell's height in pixels: the buttons are sized by it.
func drawControls(buf []byte, w, h int, cell float64, f ctlLook) {
	clear(buf)
	if w <= 0 || h <= 0 {
		return
	}
	white := vcol{1, 1, 1}
	off := f.fg.mul(0.62)
	on := f.pal[0].mix(white, 0.3)
	rad := 0.5 * cell // a button's half size, in pixels
	stroke := math.Max(1.5, cell/11) / 2
	cy := float64(h)/2 - 0.5
	put := func(x, y int, a float64, c vcol) {
		if a < 0.004 || x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		i := (y*w + x) * 4
		if a255 := byte(255 * math.Min(1, a)); a255 > buf[i+3] {
			buf[i], buf[i+1], buf[i+2], buf[i+3] = byte(255*clamp01(c.r)), byte(255*clamp01(c.g)), byte(255*clamp01(c.b)), a255
		}
	}
	// button draws one: dist gives, for a point in the button's own units
	// (1 = its half size, y down), the distance to its strokes and to its
	// filled parts (a large number for none).
	button := func(n int, size, solid float64, c vcol, dist func(x, y float64) (line, fill float64)) {
		cx := (float64(n) + 0.5) * float64(w) / 4
		r := rad * size * (1 + 0.28*f.pop[n])
		reach := int(r*1.9) + 3
		for y := int(cy) - reach; y <= int(cy)+reach; y++ {
			for x := int(cx) - reach; x <= int(cx)+reach; x++ {
				line, fill := dist((float64(x)-cx)/r, (float64(y)-cy)/r)
				a := clamp01(stroke - line*r + 0.5)
				a = math.Max(a, clamp01(0.5-fill*r))
				put(x, y, a*solid, c)
			}
		}
	}
	const none = 1e9

	// Repeat: a ring with a gap and an arrow head; a dot in it for "one".
	c := off
	if f.repeat > 0 {
		c = on
	}
	button(0, 1, 1, c, func(x, y float64) (float64, float64) {
		const ring, gapAt, gapHalf = 0.72, -math.Pi / 4, 0.42
		ang := math.Atan2(y, x)
		d := math.Abs(ang - gapAt)
		line := none
		if d > gapHalf {
			line = math.Abs(math.Hypot(x, y) - ring)
		}
		// The arrow head, where the ring ends going round clockwise.
		a0 := gapAt - gapHalf
		tx, ty := ring*math.Cos(a0), ring*math.Sin(a0)
		line = math.Min(line, sdSeg(x, y, tx, ty, tx-0.42, ty+0.02))
		line = math.Min(line, sdSeg(x, y, tx, ty, tx+0.04, ty-0.42))
		fill := none
		if f.repeat == 2 {
			fill = math.Hypot(x, y) - 0.2
		}
		return line, fill
	})

	// Shuffle: two lines that cross, each ending in an arrow head.
	c = off
	if f.shuffle {
		c = on
	}
	button(1, 1, 1, c, func(x, y float64) (float64, float64) {
		line := none
		for _, s := range []float64{1, -1} {
			px, py := -0.85, -0.42*s
			for k := 1; k <= 10; k++ {
				qx := -0.85 + 1.7*float64(k)/10
				qy := 0.42 * s * math.Sin(clamp01(float64(k)/10)*math.Pi-math.Pi/2)
				line = math.Min(line, sdSeg(x, y, px, py, qx, qy))
				px, py = qx, qy
			}
			line = math.Min(line, sdSeg(x, y, 0.85, 0.42*s, 0.55, 0.42*s-0.27))
			line = math.Min(line, sdSeg(x, y, 0.85, 0.42*s, 0.55, 0.42*s+0.27))
		}
		return line, none
	})

	// Play, pause, or a turning ring while the song loads; a ring round it
	// that breathes with the drum.
	c = f.fg.mix(white, 0.5)
	if f.playing || f.loading {
		c = f.pal[0].mix(white, 0.5)
	}
	button(2, 1.25, 1, c, func(x, y float64) (float64, float64) {
		switch {
		case f.loading:
			ang := math.Mod(math.Atan2(y, x)-f.spin+4*math.Pi, 2*math.Pi)
			if ang < 1.5*math.Pi {
				return math.Abs(math.Hypot(x, y) - 0.66), none
			}
			return none, none
		case f.playing:
			// Two bars with round ends.
			ax := math.Abs(x) - 0.34
			dx, dy := math.Abs(ax)-0.06, math.Abs(y)-0.5
			return none, math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) + math.Min(math.Max(dx, dy), 0) - 0.1
		default:
			// A triangle with soft corners.
			const ax, ay, bx, by, tx, ty = -0.42, -0.62, -0.42, 0.62, 0.66, 0.0
			d := math.Min(sdSeg(x, y, ax, ay, bx, by), math.Min(sdSeg(x, y, bx, by, tx, ty), sdSeg(x, y, tx, ty, ax, ay)))
			in := x > ax && (y-ay)*(tx-ax)-(x-ax)*(ty-ay) > 0 && (y-by)*(tx-bx)-(x-bx)*(ty-by) < 0
			if in {
				d = -d
			}
			return none, d - 0.1
		}
	})
	// Faint, and only as solid as the drum makes it.
	button(2, 1.25, 0.22+0.6*clamp01(f.kick), c, func(x, y float64) (float64, float64) {
		return math.Abs(math.Hypot(x, y) - (1.45 + 0.08*f.kick)), none
	})

	// Favourite: a heart, filled once the song is one.
	c = off
	if f.fav {
		c = vcol{1, 0.36, 0.5}.mix(f.pal[0], 0.25)
	}
	button(3, 1, 1, c, func(x, y float64) (float64, float64) {
		d := sdHeart(x*0.72, (0.62-y)*0.72) / 0.72
		if f.fav {
			return none, d
		}
		return math.Abs(d), none
	})
}

// ctlTouch starts button n's spring, unless one is still at its start (a
// held key that flips it again and again must not hold it there).
func (l *uiLines) ctlTouch(n int, now time.Time) {
	if now.Sub(l.ctlAt[n]).Seconds() > 0.32 {
		l.ctlAt[n] = now
	}
}
