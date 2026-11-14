package tui

import (
	"math"
	"time"
)

// What the hand does shows in the interface's lines (see lines.go). A
// frame is four strings: enter presses it in and it springs back, esc lets
// it out, moving through a list sloshes it the way of the move and sends a
// swell down its sides, tab and the arrows send one along its top and
// bottom, a new song sends one all the way round. The progress line
// ripples out of the song's point on a pause and towards where a seek
// goes; the mark of the chosen row is squeezed by a press. All of it is
// over in well under a second, and only the pictures move.

// The kinds of touch.
const (
	touchPress  = iota // enter, opening something
	touchBack          // esc, closing
	touchScroll        // up and down a list; dir -1 up, 1 down
	touchSide          // tab, left and right; dir -1 left, 1 right
	touchSong          // another song; dir -1 the previous one
	touchType          // a letter typed
	touchPlay          // play, pause
	touchSeek          // a seek; dir -1 back
)

// linesTouchFor is how long a touch shows, at the most.
const linesTouchFor = 0.9

// linesHeldGap is how long after a key's last repeat it counts as let go.
const linesHeldGap = 0.12

// lineTouch is one touch: when it came, and, for a key held down (it
// repeats many times a second), since when it is held and when it last
// repeated.
type lineTouch struct {
	kind   int
	dir    float64
	at     time.Time
	heldAt time.Time // zero: not held
	last   time.Time
}

// alive reports whether the touch still shows.
func (t lineTouch) alive(now time.Time) bool {
	if now.Sub(t.at).Seconds() < linesTouchFor {
		return true
	}
	return !t.heldAt.IsZero() && now.Sub(t.last).Seconds() < linesHeldGap+linesTouchFor
}

// level is how far the touch pushes right now: the spring of the first
// press, and, while the key is held, a steady push the same way (not a new
// spring with every repeat: that is a shake), which springs back once when
// the key is let go.
func (t lineTouch) level(now time.Time) float64 {
	v := spring(now.Sub(t.at).Seconds())
	if t.heldAt.IsZero() {
		return v
	}
	hold := 0.42 * smooth(0, 0.18, now.Sub(t.heldAt).Seconds())
	if since := now.Sub(t.last).Seconds() - linesHeldGap; since > 0 {
		hold *= math.Exp(-8*since) * math.Cos(13*since) * smooth(linesTouchFor, 0.6*linesTouchFor, since)
	}
	return v + hold
}

// touch notes a touch, for the lines to answer.
func (l *uiLines) touch(kind int, dir float64) {
	now := time.Now()
	keep := l.touches[:0]
	fresh := true
	for _, t := range l.touches {
		if !t.alive(now) {
			continue
		}
		// The same touch again while it still shows is the key held
		// down: it goes on, held, instead of starting anew.
		// (Play and pause are each a touch of their own, however fast:
		// the progress line ripples at every one.)
		if t.kind == kind && t.dir == dir && kind != touchPlay && now.Sub(t.last).Seconds() < 0.6 {
			if t.heldAt.IsZero() {
				t.heldAt = now
			}
			t.last = now
			fresh = false
		}
		keep = append(keep, t)
	}
	l.touches = keep
	if fresh {
		if len(l.touches) >= 8 {
			l.touches = l.touches[1:]
		}
		l.touches = append(l.touches, lineTouch{kind: kind, dir: dir, at: now, last: now})
	}
	if kind == touchPlay || kind == touchSong {
		l.kick = 1 // the song itself was touched: the light behind the cover answers
	}
}

// linesTouchKey notes what a key does to the lines. It only looks at the
// key: what the key then does is not its business.
func (m *Model) linesTouchKey(k string) {
	l := m.lines
	if l == nil || !m.linesOn() || m.vizOpen() || m.introStep != introDone {
		return
	}
	typing := m.mode == modeSearch || m.mode == modeCommand || m.prompt != nil
	switch k {
	case "enter":
		l.touch(touchPress, 0)
	case "esc":
		l.touch(touchBack, 0)
	case "up", "pgup", "home":
		l.touch(touchScroll, -1)
	case "down", "pgdown", "end":
		l.touch(touchScroll, 1)
	case "tab":
		l.touch(touchSide, 1)
	case "shift+tab":
		l.touch(touchSide, -1)
	case "left", "right":
		dir := 1.0
		if k == "left" {
			dir = -1
		}
		if typing {
			l.touch(touchType, 0)
			return
		}
		l.touch(touchSide, dir)
		if m.activePanel < 0 {
			l.touch(touchSeek, dir) // with no panel open the arrows seek
		}
	default:
		if typing {
			l.touch(touchType, 0)
			return
		}
		switch k {
		case "k":
			l.touch(touchScroll, -1)
		case "j":
			l.touch(touchScroll, 1)
		case "n":
			l.touch(touchSong, 1)
		case "p":
			l.touch(touchSong, -1)
		case "space", " ":
			l.touch(touchPlay, 0)
		case "f":
			l.kick = 1
		case "q", "backspace":
			l.touch(touchBack, 0)
		case "l", "F", "e", "i", "P", "y", "c", "T", "t", "?", "v", ":", "/", ",", "s", "r", "R":
			l.touch(touchPress, 0)
		}
	}
}

// spring is a string let go at age seconds: out, back past its place and
// to rest. It starts from nothing, so nothing jumps.
func spring(age float64) float64 {
	if age < 0 || age > linesTouchFor {
		return 0
	}
	return 1.6 * math.Exp(-5.5*age) * math.Sin(13*age) * smooth(linesTouchFor, 0.6*linesTouchFor, age)
}

// swell is a crest that runs once along something 0..1 long: its height at
// u when it is p (0..1) of its way, going up (dir > 0) or down.
func swell(u, p, dir, width float64) float64 {
	if p <= 0 || p >= 1 {
		return 0
	}
	at := p
	if dir < 0 {
		at = 1 - p
	}
	return math.Exp(-sq((u-at)/width)) * math.Sin(math.Pi*p)
}

// touching reports whether a touch is still showing.
func (l *uiLines) touching(now time.Time) bool {
	for _, t := range l.touches {
		if t.alive(now) {
			return true
		}
	}
	return false
}

// frameBend is how far the touches push a frame's line out of its place
// right now: a function of the side, how far along it (u, 0..1) and how
// far round the frame (s, 0..1 clockwise from the top left corner), in
// cells, outwards. mine is false for a frame the hand is not on (when
// another is in focus): only a new song moves it.
func (l *uiLines) frameBend(now time.Time, mine bool) func(side int, u, s float64) float64 {
	type live struct {
		kind          int
		dir, age, lvl float64
	}
	var ts []live
	for _, t := range l.touches {
		if t.alive(now) && (mine || t.kind == touchSong) {
			ts = append(ts, live{t.kind, t.dir, now.Sub(t.at).Seconds(), t.level(now)})
		}
	}
	if len(ts) == 0 {
		return nil
	}
	return func(side int, u, s float64) float64 {
		bow := math.Sin(math.Pi * u) // a string held at both ends
		upright := side == sideLeft || side == sideRight
		// Which way "the way of the move" is for this side, as outwards.
		down, right := 0.0, 0.0
		switch side {
		case sideTop:
			down = -1
		case sideBottom:
			down = 1
		case sideLeft:
			right = -1
		case sideRight:
			right = 1
		}
		out := 0.0
		for _, t := range ts {
			sp := t.lvl
			switch t.kind {
			case touchPress:
				out -= 0.7 * bow * sp
			case touchBack:
				out += 0.7 * bow * sp
			case touchType:
				out -= 0.2 * bow * sp
			case touchPlay:
				out -= 0.35 * bow * sp
			case touchScroll:
				// The frame sloshes the way of the move, and a swell
				// runs along its sides.
				out += 0.4 * bow * sp * down * t.dir
				if upright {
					out += 0.7 * swell(u, t.age/0.4, t.dir, 0.13)
				}
			case touchSide:
				out += 0.4 * bow * sp * right * t.dir
				if !upright {
					dir := t.dir
					if side == sideBottom {
						dir = -dir // the bottom runs right to left
					}
					out += 0.7 * swell(u, t.age/0.4, dir, 0.1)
				}
			case touchSong:
				// A swell goes all the way round.
				p := t.age / 0.8
				if p < 1 {
					at := p
					if t.dir < 0 {
						at = 1 - p
					}
					d := math.Abs(s - at)
					d = math.Min(d, 1-d)
					out += 0.8 * math.Exp(-sq(d/0.045)) * math.Sin(math.Pi*p)
				}
			}
		}
		return math.Max(-1.15, math.Min(1.15, out))
	}
}

// ruleLift is how far the touches push a rule down right now, in cells.
func (l *uiLines) ruleLift(now time.Time) float64 {
	lift := 0.0
	for _, t := range l.touches {
		sp := t.level(now)
		switch t.kind {
		case touchPress, touchPlay:
			lift += 0.3 * sp
		case touchBack:
			lift -= 0.3 * sp
		case touchScroll:
			lift += 0.3 * sp * t.dir
		case touchType:
			lift += 0.12 * sp
		}
	}
	return math.Max(-0.36, math.Min(0.36, lift))
}

// selSqueeze is how much of its length the mark of the chosen row gives up
// to a press right now, -0.5..0.5.
func (l *uiLines) selSqueeze(now time.Time) float64 {
	k := 0.0
	for _, t := range l.touches {
		if t.kind == touchPress || t.kind == touchPlay {
			k += 0.45 * t.level(now)
		}
	}
	return math.Max(-0.5, math.Min(0.5, k))
}

// progressWaves is the waves the touches send along the progress line,
// whose song is at frac of it.
func (l *uiLines) progressWaves(now time.Time, frac float64) []lineWave {
	var out []lineWave
	for _, t := range l.touches {
		p := now.Sub(t.at).Seconds() / 0.6
		if p >= 1 {
			continue
		}
		amp := 0.32 * (1 - p) * smooth(0, 0.08, p)
		switch t.kind {
		case touchPlay:
			// A ripple out of the song's point, both ways.
			out = append(out, lineWave{at: frac - 0.45*p, amp: amp}, lineWave{at: frac + 0.45*p, amp: amp})
		case touchSeek:
			out = append(out, lineWave{at: frac - t.dir*0.3*(1-p), amp: amp})
		}
	}
	return out
}
