package tui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/cmplx"
	"os"
	"runtime"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/player/tap"
	"github.com/tphx37/utuimp/internal/tui/art"
)

// The visualizer takes the whole terminal: no tabs, no hints, only the
// picture (two pixels per cell). Esc leaves it; a short hint saying so shows
// when it opens and on any key it doesn't use.
//
// It listens to the audio itself when the backend shares its samples (the
// GStreamer one: Yandex Music, Apple Music, YouTube Music, SoundCloud, local
// files); otherwise it dances to a beat of its own while the music plays.
//
// A frame is made in three steps: the scene draws into fb in bright, unbounded
// colours (it may look at prev, the frame before, to leave trails); post adds
// the glow around bright parts, the kick of the beat and the dark corners;
// the encoder turns the result into cells: true colour half-blocks, or, on
// the Linux console, its 16 colours (see visualizer_out.go).

const (
	vizFFT      = 2048                  // samples per spectrum
	vizRead     = 8192                  // samples taken each frame (the beat tracker looks further back)
	vizBands    = 48                    // spectrum bands, bass first
	vizWave     = 256                   // points of the waveform
	vizHistory  = 72                    // spectrum rows kept for the terrain
	vizRowRate  = 24.0                  // terrain rows per second
	vizFrame    = 33 * time.Millisecond // ~30 frames per second
	vizHintFor  = 2500 * time.Millisecond
	vizLabelFor = 1500 * time.Millisecond
	vizFadeFor  = 600 * time.Millisecond
	vizFluxLen  = 40 // frames of history the made-up beat's detector compares with

	vizGainFloor = -34.0 // dB: the quietest level the spectrum is scaled to

	// vizLag is how long a frame takes from being worked out to being seen:
	// drawing, Bubble Tea's output and the terminal's own redraw. The frame
	// shows the sound of that much later, so the two arrive together.
	vizLag = 0.045
)

// The scenes, cycled with tab.
const (
	sceneTunnel = iota
	sceneTerrain
	sceneNebula
	sceneSpectrum
	sceneOrb
	sceneRibbons
	sceneCount
)

var vizSceneNames = [sceneCount]string{"tunnel", "terrain", "nebula", "spectrum", "orb", "ribbons"}

type vcol struct{ r, g, b float64 }

var vizWhite = vcol{1, 1, 1}

func (c vcol) add(o vcol) vcol    { return vcol{c.r + o.r, c.g + o.g, c.b + o.b} }
func (c vcol) mul(k float64) vcol { return vcol{c.r * k, c.g * k, c.b * k} }
func (c vcol) mix(o vcol, k float64) vcol {
	return vcol{c.r + (o.r-c.r)*k, c.g + (o.g-c.g)*k, c.b + (o.b-c.b)*k}
}

type vec3 struct{ x, y, z float64 }

type vizStar struct{ x, y, z, tw float64 }

// The voices: parts of the spectrum that mostly belong to one kind of
// sound. The split is by frequency, so it is rough (a low voice reaches into
// the body, a snare into everything), but each part does move with its own
// instruments.
const (
	voiceBass   = iota // up to 150 Hz: the bass drum and the bass
	voiceBody          // 150–500 Hz: toms, the body of guitars and keys
	voiceVocal         // 500 Hz–2.5 kHz: singing and lead instruments
	voiceBright        // 2.5–6 kHz: the snare's crack, the edge of guitars and synths
	voiceAir           // above 6 kHz: hats and cymbals
	voiceCount
)

// voiceEdges are the spectrum bands where each voice starts (the 48 bands
// run from 35 Hz to 16 kHz in equal steps of pitch).
var voiceEdges = [voiceCount + 1]int{0, 11, 21, 34, 40, vizBands}

// vizDue is a hit to show when the clock reaches at.
type vizDue struct{ at, strength float64 }

// vizPart is a spark thrown out on a beat.
type vizPart struct {
	x, y, z, vx, vy, vz float64
	life, hue           float64
}

type visualizer struct {
	open       bool
	gen        int // tick chain id: a stale chain stops on its next tick
	scene      int
	drawn      int // the scene prev belongs to
	hintUntil  time.Time
	label      string
	labelUntil time.Time
	last       time.Time

	t      float64 // animation clock in seconds; runs faster on loud parts
	beatT  float64 // time since the last detected beat
	onBeat bool    // a beat landed on this frame
	live   bool    // the spectrum comes from real samples
	seed   uint32

	samples []float32
	window  []float64
	spec    []complex128

	bands  []float64 // smoothed band levels, 0..1
	target []float64
	peaks  []float64 // falling peak marks of the spectrum scene
	gain   float64   // auto gain: loudest recent band, in dB

	wave     []float64 // the waveform, about -1..1
	waveGain float64

	level, bass, mid, treble float64
	// The sound split into five voices, each driving elements of its own in
	// every scene: how loud each is (0..1, against its own loudest lately)
	// and how sharply it has just risen.
	voice      [voiceCount]float64
	pulse      [voiceCount]float64
	voiceRef   [voiceCount]float64
	voiceWas   [voiceCount]float64
	beat       float64 // 1 on a beat, fading within a fraction of a second
	kick       float64 // the bass drum: jumps with the lows, falls fast
	snap       float64 // the same for the highs: hats, snares
	beatN      int     // beats so far
	bigT, bigS float64 // time since the last strong beat, and how strong it was
	drive      float64 // how hard the song goes right now, 0..1
	hue        float64 // every beat moves the colours on; eased
	hueTo      float64
	twirl      float64 // turning added by beats
	prevT      []float64
	bt         beatTracker
	clock      float64             // seconds the visualizer has run
	info       bool                // show what is heard (i)
	ahead      float64             // how far ahead of the sound the last samples ran
	hits       []float64           // when the last strong hits were shown
	queue      []vizDue            // hits found ahead of time, waiting for their moment
	lastPos    time.Duration       // to notice a new song
	flux       [vizFluxLen]float64 // how much the lows rose, frame by frame
	fluxAt     int

	hist     [][]float64 // hist[0] is the newest row
	histFrac float64     // progress towards the next row, 0..1
	histN    int         // rows pushed so far, for the scrolling grid

	pal    [3]vcol
	palURL string
	palImg image.Image

	w, h, h2 int     // terminal cells; h2 is the picture's height in pixels
	ya, sc   float64 // pixel height/width; pixels per scene unit
	cx, cy   float64 // the centre, in pixels

	fb, prev, out []vcol
	glowA, glowB  []vcol // half-size buffers of the glow
	stars         []vizStar
	orb           []vec3
	parts         []vizPart
	frame         string

	console bool      // draw for the Linux console
	con     *vizCon   // its palette and colour tables
	rawSeq  string    // a palette sequence waiting to be sent
	texts   []vizText // lines of text laid over the picture (the intro's logo)
	edge    [3]uint8  // the picture's colour along its edges (see edgeColour)

	// strength scales how hard the picture moves (1 = full swing): the
	// shake, the push and turn of a beat, how fast the clock runs, the
	// zoom of the trails. Less of it for anyone it makes dizzy.
	strength float64
	// bright scales the finished picture's brightness (1 = as drawn).
	bright float64
	// backdrop is drawn behind every scene: "" none, "stars" (standing
	// still) or "sparks" (drifting up); see drawBackdrop.
	backdrop string
	dots     []vizStar
}

func newVisualizer() *visualizer {
	v := &visualizer{
		samples:  make([]float32, vizRead),
		window:   make([]float64, vizFFT),
		spec:     make([]complex128, vizFFT),
		bands:    make([]float64, vizBands),
		target:   make([]float64, vizBands),
		prevT:    make([]float64, vizBands),
		peaks:    make([]float64, vizBands),
		wave:     make([]float64, vizWave),
		waveGain: 0.05,
		gain:     vizGainFloor,
		seed:     0x9e3779b9,
		pal:      defaultVizPalette,
		drawn:    -1,
		strength: 1,
		bright:   1,
	}
	for i := range v.window {
		v.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(vizFFT-1))
	}
	v.hist = make([][]float64, vizHistory)
	for i := range v.hist {
		v.hist[i] = make([]float64, vizBands)
	}
	v.stars = make([]vizStar, 220)
	for i := range v.stars {
		v.stars[i] = vizStar{v.rnd()*2 - 1, v.rnd()*2 - 1, v.rnd()*4 + 0.1, v.rnd() * 6.28}
	}
	// Points spread evenly over a sphere (golden-angle spiral).
	const n = 1400
	v.orb = make([]vec3, n)
	for i := range n {
		y := 1 - 2*(float64(i)+0.5)/n
		r := math.Sqrt(1 - y*y)
		a := float64(i) * math.Pi * (3 - math.Sqrt(5))
		v.orb[i] = vec3{math.Cos(a) * r, y, math.Sin(a) * r}
	}
	return v
}

// rnd is a small xorshift generator: the scenes only need cheap noise.
func (v *visualizer) rnd() float64 {
	v.seed ^= v.seed << 13
	v.seed ^= v.seed >> 17
	v.seed ^= v.seed << 5
	return float64(v.seed) / float64(math.MaxUint32)
}

type vizTickMsg struct{ gen int }

func vizTick(gen int) tea.Cmd {
	return tea.Tick(vizFrame, func(time.Time) tea.Msg { return vizTickMsg{gen} })
}

// openVisualizer shows the visualizer and starts its frame clock.
func (m *Model) openVisualizer() tea.Cmd {
	if m.viz == nil {
		m.viz = newVisualizer()
		m.viz.console = art.ConsoleMode()
	}
	v := m.viz
	for i, name := range vizSceneNames {
		if name == m.cfg.VisualizerScene {
			v.scene = i
		}
	}
	v.strength = m.cfg.VizStrength()
	v.bright = m.cfg.VizBrightness()
	v.backdrop = m.cfg.VisualizerBackground
	v.open = true
	v.gen++
	v.last = time.Now()
	v.hintUntil = v.last.Add(vizHintFor)
	v.labelUntil = time.Time{}
	v.con = nil // the console palette is set anew
	m.vizStep()
	return tea.Batch(vizTick(v.gen), m.vizRaw())
}

// vizRaw sends the console palette the visualizer asked for, if any.
func (m *Model) vizRaw() tea.Cmd {
	if m.viz == nil || m.viz.rawSeq == "" {
		return nil
	}
	seq := m.viz.rawSeq
	m.viz.rawSeq = ""
	return tea.Raw(seq)
}

// closeVisualizer hides the visualizer; on the Linux console the returned
// command gives the palette back to the rest of the app.
func (m *Model) closeVisualizer() tea.Cmd {
	if m.viz == nil {
		return nil
	}
	v := m.viz
	v.open = false
	v.frame, v.fb, v.prev, v.out, v.glowA, v.glowB = "", nil, nil, nil, nil, nil // the picture is big: let it go
	v.rawSeq = ""
	if seq := art.ConsolePaletteSeq(); v.console && seq != "" {
		return tea.Raw(seq)
	}
	return nil
}

func (m *Model) vizOpen() bool { return m.viz != nil && m.viz.open }

// handleVizKey handles keys while the visualizer is up: esc (or V) leaves,
// tab switches the scene, and the playback keys keep working.
func (m *Model) handleVizKey(k string) tea.Cmd {
	v := m.viz
	switch k {
	case "esc", "V":
		return m.closeVisualizer()
	case "i":
		// What the visualizer hears, for when it seems to miss the music.
		v.info = !v.info
		return nil
	case "[", "]":
		// The strength, in steps of a quarter, saved for next time.
		pct := int(math.Round(v.strength * 100))
		if k == "[" {
			pct = max(0, (pct-1)/25*25)
		} else {
			pct = min(200, pct/25*25+25)
		}
		v.strength = float64(pct) / 100
		m.cfg.VisualizerStrength = &pct
		if pct == 100 {
			m.cfg.VisualizerStrength = nil
		}
		m.vizSay(fmt.Sprintf("%s %d%%", i18n.T("strength"), pct))
		return nil
	case "{", "}":
		// The brightness, in steps of a tenth, saved for next time.
		pct := int(math.Round(v.bright * 100))
		if k == "{" {
			pct = max(10, (pct-1)/10*10)
		} else {
			pct = min(150, pct/10*10+10)
		}
		v.bright = float64(pct) / 100
		m.cfg.VisualizerBrightness = &pct
		if pct == 100 {
			m.cfg.VisualizerBrightness = nil
		}
		m.vizSay(fmt.Sprintf("%s %d%%", i18n.T("brightness"), pct))
		return nil
	case "b":
		next := map[string]string{"": "stars", "stars": "sparks", "sparks": ""}
		v.backdrop = next[v.backdrop]
		m.cfg.VisualizerBackground = v.backdrop
		name := v.backdrop
		if name == "" {
			name = "no background"
		}
		m.vizSay(i18n.T(name))
		return nil
	case "tab", "shift+tab":
		step := 1
		if k == "shift+tab" {
			step = sceneCount - 1
		}
		v.scene = (v.scene + step) % sceneCount
		v.label = i18n.T(vizSceneNames[v.scene])
		v.labelUntil = time.Now().Add(vizLabelFor)
		m.vizStep()
		m.cfg.VisualizerScene = vizSceneNames[v.scene]
		if err := m.cfg.Save(""); err != nil {
			m.appendLog("[visualizer] config save error: " + err.Error())
		}
		return nil
	}
	if cmd, ok := m.playbackKey(k); ok {
		return cmd
	}
	v.hintUntil = time.Now().Add(vizHintFor)
	return nil
}

// vizSay shows label over the picture for a moment and saves the config
// (the setting it tells of has just changed).
func (m *Model) vizSay(label string) {
	v := m.viz
	v.label = label
	v.labelUntil = time.Now().Add(vizLabelFor)
	if err := m.cfg.Save(""); err != nil {
		m.appendLog("[visualizer] config save error: " + err.Error())
	}
}

// vizStep advances the animation by one frame and renders it.
func (m *Model) vizStep() {
	v := m.viz
	now := time.Now()
	dt := min(0.1, max(0.001, now.Sub(v.last).Seconds()))
	v.last = now

	if m.artwork.img != nil && m.artwork.url != v.palURL {
		if v.palURL != "" {
			v.newSong()
		}
		v.palURL = m.artwork.url
		v.pal = vizPalette(m.artwork.img)
		v.con = nil // the cover took some console colours: take them back
	} else if m.artwork.url == "" && v.palURL != "" {
		// A song with no cover: not the last one's colours.
		v.palURL, v.pal, v.con = "", defaultVizPalette, nil
	}
	v.listen(m.playerState.Playing && !m.playerState.Loading, m.playerState.Position, dt)
	v.analyse(dt)

	w, h := m.width, m.height+m.hostRows
	if w <= 0 || h <= 0 {
		return
	}
	v.resize(w, h)
	if v.scene != v.drawn {
		v.drawn = v.scene
		clear(v.prev)
		v.parts = v.parts[:0]
	}
	asp := m.artCellAsp
	if asp <= 0 {
		asp = 2
	}
	v.ya = asp / 2
	s := v.strength
	v.sc = float64(v.h2) * v.ya / 2 * (1 + (0.1*v.beat*v.beat+0.03*v.kick)*s) // a beat pushes the picture at you
	// …and a hard one shakes it (not at all when the strength is turned
	// down: a shake is the worst for the eye).
	shake := max(0, v.beat-0.5) * v.sc * 0.05 * smooth(0.5, 1, s) * s
	v.cx = float64(w)/2 + shake*math.Sin(v.t*61)
	v.cy = float64(v.h2)/2 + shake*math.Cos(v.t*47)/v.ya

	switch v.scene {
	case sceneTerrain:
		v.drawTerrain()
	case sceneNebula:
		v.drawNebula(dt)
	case sceneSpectrum:
		v.drawSpectrum()
	case sceneOrb:
		v.drawOrb(dt)
	case sceneRibbons:
		v.drawRibbons()
	default:
		v.drawTunnel()
	}
	v.post()
	v.drawBackdrop()
	if v.bright != 1 {
		for i := range v.out {
			v.out[i] = v.out[i].mul(v.bright)
		}
	}
	v.edge = v.edgeColour()
	v.fb, v.prev = v.prev, v.fb

	label := ""
	if v.info {
		label = v.infoLine(m.playerState.Playing && !m.playerState.Loading)
	}
	if path := os.Getenv("UTUIMP_VIZ_INFO"); path != "" && int(v.clock) != int(v.clock-dt) {
		// For debugging: the same line once a second into a file.
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil { //nolint:gosec // the user's own path
			_, _ = f.WriteString(v.infoLine(m.playerState.Playing && !m.playerState.Loading) + "\n")
			_ = f.Close()
		}
	}
	if now.Before(v.labelUntil) {
		label = v.label
	}
	if m.errMsg != "" {
		label = m.errMsg
	}
	if v.console {
		if v.con == nil {
			v.con = newVizCon(v.pal)
			v.rawSeq = v.con.seq
		}
		v.frame = v.encodeConsole(label, now)
	} else {
		v.frame = v.encode(label, now)
	}
}

// ── Listening ───────────────────────────────────────────────────────────────

// listen fills target with the spectrum of what's playing now, and wave with
// its waveform.
func (v *visualizer) listen(playing bool, pos time.Duration, dt float64) {
	v.live = false
	v.clock += dt
	if !playing {
		v.queue = v.queue[:0]
		clear(v.target)
		for i := range v.wave {
			v.wave[i] *= 0.8
		}
		return
	}
	rate, ahead, end := tap.Read(v.samples)
	v.ahead = ahead
	if rate <= 0 {
		v.fakeSpectrum(pos)
		return
	}
	v.live = true
	if pos+2*time.Second < v.lastPos {
		v.newSong()
	}
	v.lastPos = pos
	// The samples may run ahead of the sound (see tap.Reader). The tracker
	// takes all of them, and what it finds waits in the queue for its
	// moment; a sound source with nothing in hand gets its hits shown as
	// soon as they are found, a little late.
	v.bt.feed(v.samples, rate, end)
	for _, e := range v.bt.events {
		v.queue = append(v.queue, vizDue{
			at:       v.clock + ahead - float64(e.back)/float64(rate) - vizLag,
			strength: e.strength,
		})
	}
	// The spectrum and the waveform are of the sound the frame will be seen
	// with (a spectrum tells of the middle of its window).
	back := int((ahead - vizLag - 0.5*vizFFT/float64(rate)) * float64(rate))
	back = min(max(back, 0), vizRead-vizFFT)
	tail := v.samples[vizRead-back-vizFFT : vizRead-back]
	v.readWave(tail, dt)
	for i, s := range tail {
		v.spec[i] = complex(float64(s)*v.window[i], 0)
	}
	fft(v.spec)
	nyq := float64(rate) / 2
	hi := min(16000, nyq*0.95)
	const lo = 35.0
	binHz := float64(rate) / vizFFT
	loudest := -200.0
	for b := range vizBands {
		f0 := lo * math.Pow(hi/lo, float64(b)/vizBands)
		f1 := lo * math.Pow(hi/lo, float64(b+1)/vizBands)
		i0 := int(f0 / binHz)
		i1 := max(i0+1, int(f1/binHz))
		p := 0.0
		for i := i0; i < i1 && i < vizFFT/2; i++ {
			p = max(p, cmplx.Abs(v.spec[i]))
		}
		db := 20*math.Log10(p/vizFFT*4+1e-9) + 4*math.Log2((f0+f1)/2/200) // tilt: treble is quieter
		loudest = max(loudest, db)
		v.target[b] = db
	}
	// The scale is set by the loudest the song has been and forgets it very
	// slowly: a quiet passage must look quiet, not be stretched to fill the
	// screen. Nor is anything below -34 dB ever taken for "loud".
	if loudest > v.gain {
		v.gain = loudest
	} else {
		v.gain = max(vizGainFloor, v.gain-dt*0.4)
	}
	floor := v.gain - 42
	for b, db := range v.target {
		v.target[b] = clamp01((db - floor) / 42)
	}
}

// readWave takes the waveform from the samples: it starts at a rising zero
// crossing, so a steady tone stands still, and is scaled to the recent peak.
func (v *visualizer) readWave(samples []float32, dt float64) {
	const step = 4
	start := 0
	for i := 1; i < len(samples)-vizWave*step; i++ {
		if samples[i-1] < 0 && samples[i] >= 0 {
			start = i
			break
		}
	}
	peak := 0.0
	for i := range vizWave {
		s := 0.0
		for _, x := range samples[start+i*step : start+i*step+step] {
			s += float64(x)
		}
		s /= step
		peak = max(peak, math.Abs(s))
		v.target0(i, s)
	}
	if peak > v.waveGain {
		v.waveGain = peak
	} else {
		v.waveGain = max(0.02, v.waveGain-(v.waveGain-peak)*min(1, dt*0.05))
	}
}

// target0 eases point i of the waveform towards the raw sample s.
func (v *visualizer) target0(i int, s float64) {
	v.wave[i] += (s/max(v.waveGain, 0.02) - v.wave[i]) * 0.65
}

// newSong forgets the last song's loudness: the next one sets its own scale.
func (v *visualizer) newSong() {
	v.gain, v.waveGain = vizGainFloor, 0.05
	v.queue = v.queue[:0]
	v.bt.newSong()
}

// fakeSpectrum makes up a lively spectrum (with a kick on every beat) for
// backends that keep their audio to themselves.
func (v *visualizer) fakeSpectrum(pos time.Duration) {
	const bpm = 120.0
	sec := pos.Seconds()
	if sec <= 0 {
		sec = v.t
	}
	phase := sec * bpm / 60
	kick := math.Exp(-6 * (phase - math.Floor(phase)))
	hat := math.Exp(-14 * (phase*2 - math.Floor(phase*2)))
	for b := range vizBands {
		x := float64(b) / vizBands
		n := 0.5 + 0.25*math.Sin(sec*1.7+float64(b)*0.45) + 0.15*math.Sin(sec*3.1-float64(b)*0.9)
		val := n * (0.85 - 0.35*x)
		val += kick * math.Max(0, 1-x*5) * 0.6
		val += hat * math.Max(0, x-0.6) * 0.6
		v.target[b] = clamp01(val)
	}
	for i := range v.wave {
		x := float64(i) / vizWave
		s := (0.35+0.5*kick)*math.Sin(2*math.Pi*(x*2+v.t*0.7)) +
			0.22*math.Sin(2*math.Pi*(x*7-v.t*1.3)) + 0.12*hat*math.Sin(2*math.Pi*x*31+v.t*9)
		v.wave[i] += (s - v.wave[i]) * 0.5
	}
}

// analyse smooths the bands, measures bass/mid/treble, spots beats and
// pushes spectrum rows for the terrain.
//
// A beat is a sudden rise of the lows: the rise (over the bands, since the
// frame before) is compared with what it has been lately, so a drum stands
// out of a steady bass line, and quiet and loud songs are judged alike.
func (v *visualizer) analyse(dt float64) {
	rise := func(a, b int) float64 {
		s := 0.0
		for i := a; i < b; i++ {
			s += math.Max(0, v.target[i]-v.prevT[i])
		}
		return s / float64(b-a)
	}
	low, high := rise(0, 10), rise(30, vizBands)
	copy(v.prevT, v.target)

	up, down := min(1, dt*45), min(1, dt*10)
	for b, t := range v.target {
		if t > v.bands[b] {
			v.bands[b] += (t - v.bands[b]) * up
		} else {
			v.bands[b] += (t - v.bands[b]) * down
		}
		v.peaks[b] = max(v.bands[b], v.peaks[b]-dt*0.3)
	}
	avg := func(a, b int) float64 {
		s := 0.0
		for _, x := range v.bands[a:b] {
			s += x
		}
		return s / float64(b-a)
	}
	v.bass, v.mid, v.treble = avg(0, 8), avg(8, 28), avg(28, vizBands)
	v.level = avg(0, vizBands)
	for i := range voiceCount {
		lvl := avg(voiceEdges[i], voiceEdges[i+1])
		// Each voice against its own loudest of the last while: the hats are
		// far quieter than the bass, and should still move their element
		// all the way.
		v.voiceRef[i] = math.Max(lvl, v.voiceRef[i]*math.Exp(-dt/20))
		now := lvl / math.Max(v.voiceRef[i], 0.25)
		v.pulse[i] = math.Max(v.pulse[i]*math.Exp(-dt*8), clamp01((now-v.voiceWas[i])*4))
		v.voiceWas[i] = now
		v.voice[i] += (now - v.voice[i]) * min(1, dt*18)
	}

	v.beatT += dt
	v.onBeat = false
	hit, raw := 0.0, 0.0 // how hard it moves the picture; how big it is in the song
	if v.live {
		// The real sound: the tracker has listened to it.
		// How much any of it shows depends on how hard the song goes: the
		// same drum is a nudge in a verse and a blow in the drop.
		v.drive += (v.bt.level() - v.drive) * min(1, dt*2.5)
		ctx := 0.3 + 0.7*v.drive
		// Whatever is due by the time this frame is seen.
		keep := v.queue[:0]
		for _, e := range v.queue {
			if e.at > v.clock+dt/2 {
				keep = append(keep, e)
			} else {
				hit = math.Max(hit, e.strength*ctx)
				raw = math.Max(raw, e.strength)
			}
		}
		v.queue = keep
		v.kick = math.Max(v.kick*math.Exp(-dt*9), hit)
		v.snap = math.Max(v.snap*math.Exp(-dt*12), clamp01(high*9)*ctx)
	} else {
		// The made-up spectrum: its kicks are plain to see.
		mean, dev := 0.0, 0.0
		for _, f := range v.flux {
			mean += f
		}
		mean /= vizFluxLen
		for _, f := range v.flux {
			dev += sq(f - mean)
		}
		dev = math.Sqrt(dev / vizFluxLen)
		v.flux[v.fluxAt%vizFluxLen] = low
		v.fluxAt++
		if low > mean+1.4*dev+0.015 && low > 0.035 && v.beatT > 0.19 {
			hit, raw = 1, 1
		}
		v.kick = math.Max(v.kick*math.Exp(-dt*9), clamp01(low*7))
		v.snap = math.Max(v.snap*math.Exp(-dt*12), clamp01(high*9))
		v.drive += (v.level - v.drive) * min(1, dt*2.5)
	}
	v.bigT += dt
	if hit > 0 {
		// A weak hit moves the picture a little; only a real beat throws
		// sparks, turns the scene and changes the colours. What is a real
		// beat is judged by its size in the song, not by how hard the
		// song goes right now: in a verse the beat is still the beat.
		v.beat, v.beatT = math.Max(v.beat, hit), 0
		if raw > 0.35 {
			v.onBeat = true
			v.beatN++
			v.hueTo += 0.37 * min(1, v.strength)
			v.bigT, v.bigS = 0, hit
			v.hits = append(v.hits, v.clock)
		}
	}
	v.beat *= math.Exp(-dt * 6)
	v.hue += (v.hueTo - v.hue) * min(1, dt*14)
	// Every beat turns the scene on the same way: turning back and forth
	// would take two beats to a swing and look like half the tempo. The
	// way changes only every 32 beats.
	dir := 1.0
	if v.beatN/32%2 == 1 {
		dir = -1
	}
	s := v.strength
	v.twirl += dt * dir * v.beat * 1.6 * s

	// The clock lurches forward on every beat (the strength scales how
	// much: everything that flies, turns or scrolls runs on this clock).
	v.t += dt * (0.3*min(1, 0.4+s) + (v.level*1.2+v.drive*0.8+v.kick*1.2+v.beat*1.6)*s)

	v.histFrac += dt * vizRowRate
	for v.histFrac >= 1 {
		v.histFrac--
		last := v.hist[len(v.hist)-1]
		copy(v.hist[1:], v.hist[:len(v.hist)-1])
		copy(last, v.bands)
		v.hist[0] = last
		v.histN++
	}
}

// bandAt reads the spectrum at x in [0, 1] (bass to treble), interpolated.
func (v *visualizer) bandAt(x float64) float64 {
	return lerpRow(v.bands, x)
}

func lerpRow(row []float64, x float64) float64 {
	f := clamp01(x) * float64(len(row)-1)
	i := int(f)
	if i >= len(row)-1 {
		return row[len(row)-1]
	}
	return row[i] + (row[i+1]-row[i])*(f-float64(i))
}

// palAt walks the three palette colours as a loop.
func (v *visualizer) palAt(x float64) vcol {
	x += v.hue
	x -= math.Floor(x/3) * 3
	i := int(x)
	f := x - float64(i)
	f = f * f * (3 - 2*f)
	return v.pal[i%3].mix(v.pal[(i+1)%3], f)
}

// ── Drawing tools ───────────────────────────────────────────────────────────

// par runs fn over [0, n) split between the processors. fn must only write
// its own part of the picture.
func par(n int, fn func(a, b int)) {
	k := min(runtime.GOMAXPROCS(0), 8, n/8)
	if k <= 1 {
		fn(0, n)
		return
	}
	var wg sync.WaitGroup
	for i := range k {
		a, b := n*i/k, n*(i+1)/k
		wg.Go(func() { fn(a, b) })
	}
	wg.Wait()
}

// unit is the centre of pixel (x, y) in scene units: the origin in the
// middle, the picture's height spanning -1..1.
func (v *visualizer) unit(x, y int) (float64, float64) {
	return (float64(x) + 0.5 - v.cx) / v.sc, (float64(y) + 0.5 - v.cy) * v.ya / v.sc
}

// pix is where the scene point (u, w) falls, in pixels (a whole number is
// the centre of a pixel).
func (v *visualizer) pix(u, w float64) (float64, float64) {
	return v.cx + u*v.sc - 0.5, v.cy + w*v.sc/v.ya - 0.5
}

// splat adds colour c at (px, py), shared between the four pixels around it.
func (v *visualizer) splat(px, py float64, c vcol) {
	if px < -1 || py < -1 || px >= float64(v.w) || py >= float64(v.h2) {
		return
	}
	x0, y0 := int(math.Floor(px)), int(math.Floor(py))
	fx, fy := px-float64(x0), py-float64(y0)
	put := func(x, y int, k float64) {
		if x >= 0 && x < v.w && y >= 0 && y < v.h2 && k > 0.01 {
			i := y*v.w + x
			v.fb[i] = v.fb[i].add(c.mul(k))
		}
	}
	put(x0, y0, (1-fx)*(1-fy))
	put(x0+1, y0, fx*(1-fy))
	put(x0, y0+1, (1-fx)*fy)
	put(x0+1, y0+1, fx*fy)
}

// line adds a line of colour c between two points given in pixels.
func (v *visualizer) line(x0, y0, x1, y1 float64, c vcol) {
	dx, dy := x1-x0, y1-y0
	n := int(math.Max(math.Abs(dx), math.Abs(dy))) + 1
	if n > 4*(v.w+v.h2) {
		return
	}
	for i := range n {
		f := float64(i) / float64(n)
		v.splat(x0+dx*f, y0+dy*f, c)
	}
}

// sample reads the previous frame at (px, py), between pixels.
func (v *visualizer) sample(px, py float64) vcol { return v.sampleOf(v.prev, px, py) }

// sampleOf reads buf at (px, py), between pixels.
func (v *visualizer) sampleOf(buf []vcol, px, py float64) vcol {
	mx, my := float64(v.w-1), float64(v.h2-1)
	if px < -1 || py < -1 || px > mx+1 || py > my+1 {
		return vcol{}
	}
	px, py = math.Max(0, math.Min(mx, px)), math.Max(0, math.Min(my, py))
	x0, y0 := int(px), int(py)
	x1, y1 := min(x0+1, v.w-1), min(y0+1, v.h2-1)
	fx, fy := px-float64(x0), py-float64(y0)
	a := buf[y0*v.w+x0].mix(buf[y0*v.w+x1], fx)
	b := buf[y1*v.w+x0].mix(buf[y1*v.w+x1], fx)
	return a.mix(b, fy)
}

// warp fills the picture with the previous frame, dimmed by fade and seen
// through from: it tells where the point (u, w) of the new frame was in the
// old one. Whatever a scene draws on top then leaves a trail that moves.
func (v *visualizer) warp(fade float64, from func(u, w float64) (float64, float64)) {
	w := v.w
	par(v.h2, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := range w {
				su, sw := from(v.unit(x, y))
				c := v.sample(v.pix(su, sw)).mul(fade)
				c.r, c.g, c.b = math.Min(c.r, 1.3), math.Min(c.g, 1.3), math.Min(c.b, 1.3) // trails must not pile up into a white blot
				v.fb[y*w+x] = c
			}
		}
	})
}

// post makes out from fb: a glow spreads from the bright parts and the
// corners go darker.
func (v *visualizer) post() {
	w, h2 := v.w, v.h2
	bw, bh := (w+1)/2, (h2+1)/2
	a, b := v.glowA, v.glowB
	// The bright parts, at half size.
	par(bh, func(y0, y1 int) {
		for by := y0; by < y1; by++ {
			for bx := range bw {
				var s vcol
				n := 0.0
				for dy := range 2 {
					for dx := range 2 {
						if x, y := bx*2+dx, by*2+dy; x < w && y < h2 {
							s = s.add(v.fb[y*w+x])
							n++
						}
					}
				}
				s = s.mul(1 / n)
				a[by*bw+bx] = s.mul(smooth(0.35, 1.5, math.Max(s.r, math.Max(s.g, s.b))))
			}
		}
	})
	// Blurred across, then down.
	kern := [7]float64{1, 6, 15, 20, 15, 6, 1}
	blur := func(src, dst []vcol, sx, sy int) {
		par(bh, func(y0, y1 int) {
			for y := y0; y < y1; y++ {
				for x := range bw {
					var s vcol
					for k, wt := range kern {
						xx := min(bw-1, max(0, x+(k-3)*sx))
						yy := min(bh-1, max(0, y+(k-3)*sy))
						s = s.add(src[yy*bw+xx].mul(wt))
					}
					dst[y*bw+x] = s.mul(1.0 / 64)
				}
			}
		})
	}
	blur(a, b, 1, 0)
	blur(b, a, 0, 1)

	par(h2, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			gy := math.Max(0, (float64(y)+0.5)/2-0.5)
			gy0 := min(int(gy), bh-1)
			gy1 := min(gy0+1, bh-1)
			fy := gy - float64(gy0)
			ny := (float64(y)+0.5)/float64(h2)*2 - 1
			for x := range w {
				c := v.fb[y*w+x]
				gx := math.Max(0, (float64(x)+0.5)/2-0.5)
				gx0 := min(int(gx), bw-1)
				gx1 := min(gx0+1, bw-1)
				fx := gx - float64(gx0)
				g := a[gy0*bw+gx0].mix(a[gy0*bw+gx1], fx).mix(a[gy1*bw+gx0].mix(a[gy1*bw+gx1], fx), fy)
				nx := (float64(x)+0.5)/float64(w)*2 - 1
				vig := 1 - 0.22*(nx*nx+ny*ny)
				v.out[y*w+x] = c.add(g.mul(0.75)).mul(vig)
			}
		}
	})
}

// infoLine says what the visualizer hears: where the sound comes from, how
// far ahead of the speakers it is read, how hard the song goes and how many
// strong hits a second are shown.
func (v *visualizer) infoLine(playing bool) string {
	for len(v.hits) > 0 && v.hits[0] < v.clock-5 {
		v.hits = v.hits[1:]
	}
	switch {
	case !playing:
		return i18n.T("paused")
	case !v.live:
		return i18n.T("no sound to hear: own beat")
	}
	return fmt.Sprintf("%s %+d ms · %s %d%% · %.1f %s", i18n.T("sound"), int(v.ahead*1000),
		i18n.T("drive"), int(v.drive*100), float64(len(v.hits))/5, i18n.T("hits/s"))
}

// resize makes the picture w×h cells (two pixels each, one above the
// other).
func (v *visualizer) resize(w, h int) {
	if v.w == w && v.h == h && len(v.fb) == w*h*2 {
		return
	}
	v.w, v.h, v.h2 = w, h, h*2
	n := w * h * 2
	v.fb, v.prev, v.out = make([]vcol, n), make([]vcol, n), make([]vcol, n)
	g := ((w + 1) / 2) * ((v.h2 + 1) / 2)
	v.glowA, v.glowB = make([]vcol, g), make([]vcol, g)
}

// edgeColour is the mean colour of the picture's outermost cells. The
// terminal's background is set to it while the visualizer is open: many
// terminals keep a margin around the cells (kitty's window_padding_width)
// that is painted in that colour, and nothing else can be drawn there. So
// the margin carries the picture on instead of framing it.
func (v *visualizer) edgeColour() [3]uint8 {
	w, h2 := v.w, v.h2
	var s vcol
	n := 0.0
	add := func(x, y int) {
		s = s.add(v.out[y*w+x])
		n++
	}
	for x := range w {
		add(x, 0)
		add(x, 1)
		add(x, h2-2)
		add(x, h2-1)
	}
	for y := 2; y < h2-2; y++ {
		add(0, y)
		add(w-1, y)
	}
	if n == 0 {
		return [3]uint8{}
	}
	return toByte3(s.mul(1 / n))
}

// vizBackground is what the terminal's background is set to while the
// visualizer is open (see edgeColour); nil gives the terminal its own back.
func (m *Model) vizBackground() color.Color {
	if !m.vizOpen() || m.viz.console || len(m.viz.out) == 0 {
		return nil
	}
	e := m.viz.edge
	return color.RGBA{R: e[0], G: e[1], B: e[2], A: 255}
}

// renderVisualizer is the whole screen while the visualizer is open.
func (m *Model) renderVisualizer() string {
	v := m.viz
	if v.frame == "" || v.w != m.width || v.h != m.height+m.hostRows {
		m.vizStep()
	}
	return v.frame
}

// ── Palette ─────────────────────────────────────────────────────────────────

// defaultVizPalette is used before any cover loads: green, cyan, magenta.
var defaultVizPalette = [3]vcol{{0.25, 1, 0.3}, {0.1, 0.75, 1}, {0.95, 0.25, 0.9}}

// coverHues picks the colours that stand for a cover: up to three of its
// hues, by how much of it they colour (vivid pixels count most), each the
// mean of the pixels of that hue. n is how many there are; none for a grey
// cover, whose tint (the hue of all of it, and how strong) comes instead.
func coverHues(img image.Image) (hues [3]vcol, n int, tint vcol) {
	const buckets = 12
	var weight [buckets]float64
	var sum [buckets]vcol
	var all vcol
	b := img.Bounds()
	const grid = 24
	for gy := range grid {
		for gx := range grid {
			x := b.Min.X + gx*b.Dx()/grid
			y := b.Min.Y + gy*b.Dy()/grid
			r, g, bl, _ := img.At(x, y).RGBA()
			c := vcol{float64(r) / 65535, float64(g) / 65535, float64(bl) / 65535}
			all = all.add(c)
			h, s, val := hsv(c)
			wgt := s * s * val
			if wgt < 0.02 {
				continue
			}
			i := int(h*buckets) % buckets
			weight[i] += wgt
			sum[i] = sum[i].add(c.mul(wgt))
		}
	}
	tint = all.mul(1.0 / (grid * grid))
	var picked [3]int
	for n < 3 {
		best, bw := -1, 0.0
		for i, wt := range weight {
			far := true
			for _, p := range picked[:n] {
				if d := (i - p + buckets) % buckets; d < 2 || d > buckets-2 {
					far = false
				}
			}
			if far && wt > bw {
				best, bw = i, wt
			}
		}
		// A hue that colours next to nothing of the cover is not its colour.
		if best < 0 || bw < 0.3 || n > 0 && bw < 0.06*weight[picked[0]] {
			break
		}
		picked[n] = best
		hues[n] = sum[best].mul(1 / weight[best])
		n++
	}
	return hues, n, tint
}

// vizPalette gives the visualizer three colours from the cover, bright
// enough to glow. A cover of one hue gives shades of it (paler, deeper,
// nudged a little round the wheel), not hues it doesn't have; a grey one
// gives greys with its tint.
func vizPalette(img image.Image) [3]vcol {
	hues, n, tint := coverHues(img)
	if n == 0 {
		h, s, _ := hsv(tint)
		s = math.Min(s, 0.25)
		return [3]vcol{hsvColor(h, s, 1), hsvColor(h, s*0.6, 0.7), hsvColor(math.Mod(h+0.03, 1), s*1.3, 0.85)}
	}
	var out [3]vcol
	for i := range n {
		out[i] = neon(hues[i])
	}
	h, s, _ := hsv(hues[0])
	s = math.Max(0.5, math.Min(1, s*1.2))
	if n < 2 {
		out[1] = hsvColor(math.Mod(h+0.05, 1), s*0.5, 1)
	}
	if n < 3 {
		out[2] = hsvColor(math.Mod(h+0.96, 1), math.Min(1, s*1.15), 0.8)
	}
	return out
}

// coverAccents gives the interface's text and progress line colours from
// the cover, as "#rrggbb", most of the cover first (see coverHues); shades
// of one hue fill in for missing ones. A grey cover gives greys with its
// tint.
func coverAccents(img image.Image) []string {
	hues, n, tint := coverHues(img)
	if n == 0 {
		h, s, _ := hsv(tint)
		s = math.Min(s, 0.2)
		return []string{toHex(hsvColor(h, s, 0.95)), toHex(hsvColor(h, s*0.7, 0.7)), toHex(hsvColor(h, s, 0.85))}
	}
	h, s, _ := hsv(hues[0])
	s = math.Max(0.45, math.Min(0.9, s))
	cols := make([]vcol, 0, 3)
	for i := range n {
		hi, si, _ := hsv(hues[i])
		cols = append(cols, hsvColor(hi, math.Max(0.45, math.Min(0.9, si)), 1))
	}
	if n < 2 {
		cols = append(cols, hsvColor(math.Mod(h+0.05, 1), s*0.55, 1))
	}
	if n < 3 {
		cols = append(cols, hsvColor(math.Mod(h+0.96, 1), math.Min(1, s*1.2), 0.9))
	}
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = toHex(c)
	}
	return out
}

func toHex(c vcol) string {
	f := func(x float64) int { return int(math.Round(clamp01(x) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", f(c.r), f(c.g), f(c.b))
}

func neon(c vcol) vcol {
	h, s, _ := hsv(c)
	return hsvColor(h, math.Max(0.65, math.Min(1, s*1.3)), 1)
}

func hsv(c vcol) (h, s, v float64) {
	mx := math.Max(c.r, math.Max(c.g, c.b))
	mn := math.Min(c.r, math.Min(c.g, c.b))
	v = mx
	if mx <= 0 {
		return 0, 0, 0
	}
	d := mx - mn
	s = d / mx
	if d == 0 {
		return 0, s, v
	}
	switch mx {
	case c.r:
		h = math.Mod((c.g-c.b)/d, 6)
	case c.g:
		h = (c.b-c.r)/d + 2
	default:
		h = (c.r-c.g)/d + 4
	}
	h /= 6
	if h < 0 {
		h++
	}
	return h, s, v
}

func hsvColor(h, s, v float64) vcol {
	h = math.Mod(h, 1) * 6
	i := int(h)
	f := h - float64(i)
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	switch i % 6 {
	case 0:
		return vcol{v, t, p}
	case 1:
		return vcol{q, v, p}
	case 2:
		return vcol{p, v, t}
	case 3:
		return vcol{p, q, v}
	case 4:
		return vcol{t, p, v}
	default:
		return vcol{v, p, q}
	}
}

// ── Maths ───────────────────────────────────────────────────────────────────

// fft transforms a in place (radix 2; len(a) must be a power of two).
func fft(a []complex128) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wn := complex(1, 0)
			for k := range size / 2 {
				u := a[start+k]
				t := wn * a[start+k+size/2]
				a[start+k] = u + t
				a[start+k+size/2] = u - t
				wn *= step
			}
		}
	}
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }
func sq(x float64) float64      { return x * x }

func smooth(a, b, x float64) float64 {
	t := clamp01((x - a) / (b - a))
	return t * t * (3 - 2*t)
}

// hash1 is a fixed pseudo-random number in [0, 1) for x.
func hash1(x float64) float64 {
	s := math.Sin(x*127.1+311.7) * 43758.5453
	return s - math.Floor(s)
}
