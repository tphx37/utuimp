package tui

import "math"

// Finding the hits.
//
// The visualizer's picture is redrawn 30 times a second, which is too coarse
// to hear a rhythm with, so the tracker cuts the sound into steps of its own
// (about 11 ms) and for each one measures how much the spectrum rose since
// the step before: a drum hit is a sudden rise in many bands at once.
//
// Nothing is guessed or predicted: a hit is shown only where the sound has
// one, and as strongly as it stands out. Two things make that mean the same
// in any music:
//
//   - loudness is measured against the loudest the song has been, not
//     against the last second: a quiet passage gives small numbers and is
//     left alone instead of being blown up to fill the screen;
//   - a hit is weighed against what a big hit in this song measures, so the
//     ripple of a quiet part is nothing and a fill is less than the drop.

const (
	btWin   = 2048 // samples per step's spectrum: long enough to hold a few waves of the bass
	btHop   = 512  // samples between steps
	btBands = 24
	btLow   = 5   // bands below about 150 Hz: the bass drum's
	btHist  = 640 // steps of history: about 7 seconds
)

// beatEvent is a hit that happened back samples before the newest sample
// fed.
type beatEvent struct {
	back     int
	strength float64
}

// btOnsetBack is how far before the end of its window a hit really is.
const btOnsetBack = 1030

type beatTracker struct {
	rate  int
	hps   float64 // steps per second
	win   [btWin]float64
	buf   []complex128
	edges [btBands + 1]int
	prev  [btBands]float64
	pos   int64 // the stream's sample number the last step ended at

	// The loudness of the sound above the bass in slices of 6 ms: fine
	// enough to tell how fast a sound comes in.
	lp     float64
	slices [16]float64
	sliceN int

	ref  float64 // the loudest the song has been, slowly forgotten
	loud float64 // how loud it is now, against ref (0..1)

	env [btHist]float64 // the rise at each step
	n   int             // steps so far
	big float64         // what a big hit in this song measures

	lastBeat int     // step of the last hit given out
	lastRise float64 // its rise

	fast float64 // loudness over the last third of a second
	top  float64 // the biggest hit of the last while: full strength

	// What the last feed found, each with the moment it happened.
	events []beatEvent
	back   int // samples between the step being analysed and the newest one
}

func (b *beatTracker) reset(rate int) {
	*b = beatTracker{rate: rate, hps: float64(rate) / btHop, buf: make([]complex128, btWin)}
	for i := range b.win {
		b.win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(btWin-1))
	}
	const lo, hi = 40.0, 11000.0
	top := math.Min(hi, float64(rate)*0.45)
	for k := range b.edges {
		f := lo * math.Pow(top/lo, float64(k)/btBands)
		b.edges[k] = max(1, int(f/float64(rate)*btWin+0.5))
		if k > 0 && b.edges[k] <= b.edges[k-1] {
			b.edges[k] = b.edges[k-1] + 1
		}
	}
}

// newSong forgets how loud the last song was.
func (b *beatTracker) newSong() {
	b.ref, b.big, b.top = 0, b.big*0.3, b.top*0.3
}

// feed takes samples that end at sample number end of the stream (see
// tap.Reader) and fills events. Every step is analysed exactly once, at its
// own place in the stream, however unevenly the calls come.
func (b *beatTracker) feed(s []float32, rate int, end int64) {
	if rate != b.rate || b.buf == nil {
		b.reset(rate)
	}
	b.events = b.events[:0]
	if b.pos == 0 || end < b.pos || end-b.pos > int64(len(s)-btWin) {
		b.pos = end - btHop // the stream jumped: take it up from here
	}
	for b.pos+btHop <= end {
		b.pos += btHop
		b.back = int(end - b.pos)
		e := len(s) - b.back
		b.step(s[e-btWin : e])
	}
}

// attack takes the newest samples of a step and tells how suddenly the
// sound of the last 50 ms came in, 0..1: the biggest jump between two
// neighbouring 6 ms slices, as a part of all the change there was. A drum
// brings nearly all of its loudness in one slice; a voice spreads it over
// ten. The bass is left out: its slow waves would look like jumps.
func (b *beatTracker) attack(fresh []float32) float64 {
	const size = 256
	k := 1 - math.Exp(-2*math.Pi*200/float64(b.rate))
	for i := 0; i+size <= len(fresh); i += size {
		e := 0.0
		for _, x := range fresh[i : i+size] {
			b.lp += (float64(x) - b.lp) * k
			e += sq(float64(x) - b.lp)
		}
		b.slices[b.sliceN%len(b.slices)] = e / size
		b.sliceN++
	}
	lo, hi, jump := math.Inf(1), 0.0, 0.0
	prev := -1.0
	for i := b.sliceN - 9; i < b.sliceN; i++ {
		if i < 0 {
			continue
		}
		e := b.slices[i%len(b.slices)]
		lo, hi = math.Min(lo, e), math.Max(hi, e)
		if prev >= 0 {
			jump = math.Max(jump, e-prev)
		}
		prev = e
	}
	// A change smaller than a fifth of the level is the sound's own
	// flutter, not something coming in.
	return smooth(0.18, 0.45, jump/math.Max(hi-lo, 0.2*hi+1e-12))
}

func (b *beatTracker) at(i int) float64 { return b.env[((i%btHist)+btHist)%btHist] }

// step analyses one window of sound.
func (b *beatTracker) step(w []float32) {
	for i, x := range w {
		b.buf[i] = complex(float64(x)*b.win[i], 0)
	}
	fft(b.buf)
	var e [btBands]float64
	total := 0.0
	for k := range btBands {
		for i := b.edges[k]; i < b.edges[k+1] && i < btWin/2; i++ {
			e[k] += sq(real(b.buf[i])) + sq(imag(b.buf[i]))
		}
		e[k] /= btWin * btWin
		total += e[k]
	}
	// 1e-5 is a whisper: below it there is nothing to follow.
	b.ref = math.Max(1e-5, math.Max(total, b.ref*math.Pow(0.5, 1/(25*b.hps))))
	b.loud = total / b.ref
	// What counts as a hit is a drum, a bass note or some other sudden
	// noise; not a voice or a violin coming in, however loud. They differ
	// in two ways: a struck sound rises in most bands at once, a sung or
	// bowed one in the few bands its harmonics fall in; and a struck sound
	// is there within a few milliseconds, while a voice swells in over
	// tens (see attack). So the rise counts in full only when it is wide
	// and sudden; and the lows count on their own, for the bass.
	rise, low, wide := 0.0, 0.0, 0
	for k := range btBands {
		l := math.Log1p(e[k] / b.ref * 300)
		if d := l - b.prev[k]; d > 0 && b.n > 0 {
			rise += d
			if d > 0.3 {
				wide++
			}
			if k < btLow {
				low += d
			}
		}
		b.prev[k] = l
	}
	// A rise in a third of the bands is already a wide one: a dense mix
	// leaves a drum few bands where it stands out.
	flux := rise*smooth(0.1, 0.35, float64(wide)/btBands)*b.attack(w[len(w)-btHop:]) + 2.2*low
	b.env[b.n%btHist] = flux
	b.n++
	n := b.n
	fade := math.Pow(0.5, 1/(12*b.hps))
	// What a big hit measures: followed half-way at a time, so that one
	// freak (the very first sound after silence, say) doesn't make every
	// real hit after it look small.
	if b.big *= fade; flux > b.big && n > 30 {
		b.big += (flux - b.big) * 0.5
	}
	if n < 4 {
		return
	}
	b.fast += (b.loud - b.fast) / (0.3 * b.hps)
	if n > 40 {
		b.top = math.Max(b.top*math.Pow(0.5, 1/(20*b.hps)), flux)
	}
	b.pick(n)
}

// The hit picker looks at a step btPost steps back: the steps after it tell
// whether it is a peak. The sound runs far enough ahead of the speakers
// (see tap.Reader) for that wait to cost nothing.
const (
	btPost    = 3   // steps after the peak it must stand above (35 ms)
	btPreMax  = 3   // steps before it, the same
	btPreAvg  = 10  // steps before it that make up its surroundings (115 ms)
	btRing    = 0.2 // seconds a hit's ringing takes to fall to a third
	btDelta   = 0.12
	btMinRise = 0.8
)

// pick gives out a hit where the rise peaks and stands above its
// surroundings: the tenth of a second around it, not the last second. A
// beat must not have to outshout the beats before it: in a fast or dense
// song they would raise the bar so high that every other one went unseen.
// A little of what a big hit measures in this song is added to the bar, so
// that the ripple of a quiet part is not taken for drums.
func (b *beatTracker) pick(n int) {
	c := n - 1 - btPost
	if c <= btPreAvg {
		return
	}
	p := b.at(c)
	// After a hit its own ringing rises and falls too: for a moment only
	// something nearly as big is a new hit.
	if p < btMinRise || p < b.lastRise*math.Exp(-float64(c-b.lastBeat)/(btRing*b.hps)) {
		return
	}
	for i := c - btPreMax; i <= c+btPost; i++ {
		if q := b.at(i); q > p || (q == p && i < c) {
			return
		}
	}
	avg := 0.0
	for i := c - btPreAvg; i <= c+btPost; i++ {
		avg += b.at(i)
	}
	avg /= btPreAvg + btPost + 1
	if p < avg+btDelta*b.big {
		return
	}
	// As strong as it is big next to the biggest hit lately: the usual beat
	// of a song comes out around a half, its accents near one.
	b.events = append(b.events, beatEvent{
		back:     b.back + btPost*btHop + btOnsetBack,
		strength: math.Max(0.1, clamp01(p/math.Max(b.top, 4))),
	})
	b.lastBeat, b.lastRise = c, p
}

// level is how loud the music is now for this song, 0..1: about a half in
// an ordinary part, near one where it is at its loudest.
func (b *beatTracker) level() float64 { return clamp01(math.Pow(b.fast, 0.75) * 1.25) }
