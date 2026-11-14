package tui

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// testSong makes sound sample by sample: gen(t) is the sample at time t.
// run feeds it to a tracker the way the visualizer does, 30 times a second,
// and reports every beat as (time, strength).
type testBeat struct{ at, strength float64 }

func runTracker(gen func(t float64) float64, from, to float64) (*beatTracker, []testBeat) {
	const rate, dt = 44100, 1.0 / 30
	var b beatTracker
	var beats []testBeat
	buf := make([]float32, vizRead)
	song := make([]float32, int(to*rate)+1)
	for i := range song {
		song[i] = float32(gen(float64(i) / rate))
	}
	for now := from; now < to; now += dt {
		end := int(now * rate)
		for i := range buf {
			if j := end - len(buf) + i; j >= 0 {
				buf[i] = song[j]
			} else {
				buf[i] = 0
			}
		}
		b.feed(buf, rate, int64(end))
		for _, e := range b.events {
			beats = append(beats, testBeat{now - float64(e.back)/rate, e.strength})
		}
	}
	return &b, beats
}

func noise(seed *uint32) float64 {
	*seed ^= *seed << 13
	*seed ^= *seed >> 17
	*seed ^= *seed << 5
	return float64(*seed)/math.MaxUint32*2 - 1
}

// denseSong is a wall of sound: a loud bass line and a noise wash that never
// stop, with a bass drum on every beat and a snare on every other.
func denseSong(bpm float64) func(float64) float64 {
	seed := uint32(99)
	period := 60 / bpm
	return func(t float64) float64 {
		since := math.Mod(t, period)
		drum := math.Exp(-since*18) * math.Sin(2*math.Pi*(55+40*math.Exp(-since*30))*since)
		bass := 0.35 * math.Sin(2*math.Pi*82*t) * (0.8 + 0.2*math.Sin(2*math.Pi*0.7*t))
		wash := 0.22 * noise(&seed)
		snare := 0.0
		if int(t/period)%2 == 1 {
			snare = 0.3 * math.Exp(-since*25) * noise(&seed)
		}
		guitar := 0.2 * (math.Sin(2*math.Pi*196*t) + math.Sin(2*math.Pi*294*t) + math.Sin(2*math.Pi*392*t+3*math.Sin(2*math.Pi*5*t)))
		return 0.45*drum + bass + wash + snare + guitar
	}
}

// In a dense mix nearly every beat must be shown, on time, with few beats
// that aren't there.
func TestBeatTracker_DenseMix(t *testing.T) {
	for _, bpm := range []float64{92, 128, 140, 174} {
		_, beats := runTracker(denseSong(bpm), 0, 16)
		period := 60 / bpm
		caught := map[int]bool{}
		stray := 0
		total := 0
		offSum := 0.0
		for _, bt := range beats {
			if bt.at < 6 || bt.strength < 0.2 {
				continue
			}
			total++
			k := math.Round(bt.at / period)
			if off := bt.at - k*period; math.Abs(off) < 0.03 {
				caught[int(k)] = true
				offSum += off
			} else {
				stray++
			}
		}
		want := int(10 / period)
		mean := offSum / float64(max(1, len(caught))) * 1000
		t.Logf("%v bpm: %d of %d beats caught, %d stray; timed %+.1f ms from the drum on average", bpm, len(caught), want, stray, mean)
		if math.Abs(mean) > 8 {
			t.Errorf("%v bpm: hits are timed %+.1f ms from the drum", bpm, mean)
		}
		if len(caught) < want*85/100 {
			t.Errorf("%v bpm: only %d of %d beats caught", bpm, len(caught), want)
		}
		if stray > total/8 {
			t.Errorf("%v bpm: %d of %d beats are off the beat", bpm, stray, total)
		}
	}
}

// After a loud part, a quiet one (a soft pad with a little hiss, 26 dB
// down) must be left in peace: no beats, no kicks.
func TestBeatTracker_QuietPassageIsCalm(t *testing.T) {
	loud := denseSong(128)
	seed := uint32(5)
	gen := func(t float64) float64 {
		if t < 8 {
			return loud(t)
		}
		trem := 0.7 + 0.3*math.Sin(2*math.Pi*3*t)
		return 0.03*trem*(math.Sin(2*math.Pi*220*t)+math.Sin(2*math.Pi*277*t)+math.Sin(2*math.Pi*330*t)) + 0.004*noise(&seed)
	}
	b, beats := runTracker(gen, 0, 16)
	n := 0
	for _, bt := range beats {
		if bt.at > 8.5 && bt.strength > 0.3 {
			n++
			t.Logf("a beat at %.2fs, strength %.2f", bt.at, bt.strength)
		}
	}
	if n > 0 {
		t.Errorf("%d beats in a quiet passage with no rhythm", n)
	}
	if b.loud > 0.02 {
		t.Errorf("the quiet passage measures %.3f of the song's loudness, want next to nothing", b.loud)
	}
}

// With VIZ_PCM naming a raw mono float32 44.1 kHz file (ffmpeg -f f32le -ac 1
// -ar 44100) and VIZ_DUMP a directory, the tracker runs over real music and
// draws what it found: per row 10 seconds; the lows (red) and the whole sound
// (grey) as they rise and fall; above them a white tick for every hit, as
// tall as it is strong, and how hard the song goes (green).
func TestBeatTracker_DrawRealMusic(t *testing.T) {
	path, dir := os.Getenv("VIZ_PCM"), os.Getenv("VIZ_DUMP")
	if path == "" || dir == "" {
		t.Skip("VIZ_PCM / VIZ_DUMP not set")
	}
	raw, err := os.ReadFile(path) //nolint:gosec // a recording the tester gave
	if err != nil {
		t.Fatal(err)
	}
	const rate, dt = 44100, 1.0 / 30
	song := make([]float32, len(raw)/4)
	for i := range song {
		song[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	secs := float64(len(song)) / rate
	const pps, rowSecs, rowH = 120, 10, 90
	rows := int(secs/rowSecs) + 1
	img := image.NewRGBA(image.Rect(0, 0, pps*rowSecs, rows*rowH))
	for i := range img.Pix {
		img.Pix[i] = 255 * uint8(i%4/3)
	}
	set := func(sec float64, y int, c color.RGBA) {
		row := int(sec / rowSecs)
		x := int((sec - float64(row)*rowSecs) * pps)
		if y >= 0 && y < rowH-2 {
			img.Set(x, row*rowH+rowH-3-y, c)
		}
	}
	// The sound itself: energy below 150 Hz and overall, in 5 ms slices.
	lp := 0.0
	k := 1 - math.Exp(-2*math.Pi*150/rate)
	peakL, peakA := 1e-9, 1e-9
	const slice = rate / 200
	type en struct{ l, a float64 }
	var ens []en
	for i := 0; i+slice <= len(song); i += slice {
		var l, a float64
		for _, x := range song[i : i+slice] {
			lp += (float64(x) - lp) * k
			l += lp * lp
			a += float64(x) * float64(x)
		}
		ens = append(ens, en{l, a})
		peakL, peakA = max(peakL, l), max(peakA, a)
	}
	for i, e := range ens {
		sec := float64(i) / 200
		for y := range int(math.Sqrt(e.a/peakA) * 40) {
			set(sec, y, color.RGBA{70, 70, 70, 255})
		}
		for y := range int(math.Sqrt(e.l/peakL) * 40) {
			set(sec, y, color.RGBA{200, 40, 40, 255})
		}
	}
	var b beatTracker
	buf := make([]float32, vizRead)
	hits := 0
	for now := 0.0; now < secs; now += dt {
		end := int(now * rate)
		for i := range buf {
			if j := end - len(buf) + i; j >= 0 {
				buf[i] = song[j]
			} else {
				buf[i] = 0
			}
		}
		b.feed(buf, rate, int64(end))
		for y := range int(b.level() * 16) {
			set(now, 60+y, color.RGBA{40, 160, 60, 255})
		}
		for _, e := range b.events {
			at := now - float64(e.back)/rate
			hits++
			for y := range int(e.strength*36) + 2 {
				set(at, 46+y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	t.Logf("%.0f s, %d hits (%.1f a second)", secs, hits, float64(hits)/secs)
	f, err := os.Create(filepath.Join(dir, "real.png")) //nolint:gosec // a directory the tester asked for
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// singer is a voice with no drums: notes of a tune with vibrato, each
// swelling in over a twentieth of a second while the one before dies away,
// some of them loud, over strings.
func singer(t float64) float64 {
	notes := []float64{392, 440, 523, 587, 659, 587, 523, 440, 698, 659}
	const noteLen = 0.6
	i := int(t / noteLen)
	since := t - float64(i)*noteLen
	tone := func(n int) float64 {
		f := notes[((n%len(notes))+len(notes))%len(notes)]
		// Vibrato: the pitch sways by 1.2% five and a half times a second.
		phase := 2*math.Pi*f*t - 0.012*f/5.5*math.Cos(2*math.Pi*5.5*t)
		v := 0.0
		for h := 1; h <= 8; h++ {
			v += math.Sin(phase*float64(h)) / float64(h)
		}
		return v
	}
	loud := 0.6 + 0.4*math.Sin(2*math.Pi*t/7) // crescendos
	voice := (1-math.Exp(-since/0.05))*tone(i) + math.Exp(-since/0.06)*tone(i-1)
	strings := 0.12 * (math.Sin(2*math.Pi*196*t) + math.Sin(2*math.Pi*247*t) + math.Sin(2*math.Pi*294*t))
	return 0.3*loud*voice + strings
}

// Singing, however loud, is not a beat: opera must not make the picture
// jump. A drum under the same singing still does.
func TestBeatTracker_AVoiceIsNotADrum(t *testing.T) {
	_, beats := runTracker(singer, 0, 20)
	strong := 0
	for _, bt := range beats {
		if bt.at > 4 && bt.strength > 0.35 {
			strong++
		}
	}
	t.Logf("singing alone: %d hits in all, %d strong ones in 16 s", len(beats), strong)
	if strong > 3 {
		t.Errorf("%d strong hits in 16 s of singing with no drums", strong)
	}

	const period = 0.5
	withDrum := func(t float64) float64 {
		since := math.Mod(t, period)
		return singer(t) + 0.5*math.Exp(-since*18)*math.Sin(2*math.Pi*(55+40*math.Exp(-since*30))*since)
	}
	_, beats = runTracker(withDrum, 0, 20)
	caught := map[int]bool{}
	for _, bt := range beats {
		k := math.Round(bt.at / period)
		if bt.at > 6 && bt.strength > 0.35 && math.Abs(bt.at-k*period) < 0.03 {
			caught[int(k)] = true
		}
	}
	t.Logf("singing over a drum: %d of 28 drum hits caught", len(caught))
	if len(caught) < 24 {
		t.Errorf("only %d of 28 drum hits caught under the singing", len(caught))
	}
}
