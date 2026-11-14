package tui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/player/tap"
)

// fakeTap feeds a bass kick, a chord and some noise, like music would.
func fakeTap(t *testing.T) {
	t.Helper()
	start := time.Now()
	tap.Register(func(dst []float32) (int, float64, int64) {
		const rate = 48000
		now := time.Since(start).Seconds()
		seed := uint32(7)
		for i := range dst {
			ts := now - float64(len(dst)-i)/rate
			beat := math.Mod(ts*2, 1)
			kick := (0.3 + math.Exp(-beat*12)) * math.Sin(2*math.Pi*55*ts)
			chord := 0.2 * (math.Sin(2*math.Pi*440*ts) + math.Sin(2*math.Pi*554*ts) + math.Sin(2*math.Pi*1320*ts))
			seed ^= seed << 13
			seed ^= seed >> 17
			seed ^= seed << 5
			noise := (float64(seed)/math.MaxUint32 - 0.5) * 0.05
			dst[i] = float32(0.6*kick + chord + noise)
		}
		return rate, 0, int64(now * rate)
	})
	t.Cleanup(func() { tap.Register(func([]float32) (int, float64, int64) { return 0, 0, 0 }) })
}

func vizModel(t *testing.T) *Model {
	t.Helper()
	m := coverModel(t, 100, 30)
	if os.Getenv("VIZ_DUMP") != "" {
		m.width, m.height = 200, 60 // a framebuffer console's size
	}
	m.playerState.Playing = true
	return m
}

func TestVisualizer_FullscreenFrame(t *testing.T) {
	fakeTap(t)
	m := vizModel(t)
	m.handleNormalKey(tea.KeyPressMsg{Code: 'V', Text: "V"}, "V")
	if !m.vizOpen() {
		t.Fatal("V should open the visualizer")
	}
	// Just opened, the picture is still opening out of the middle: its
	// corners are dark, its middle is not.
	m.viz.openedAt = time.Now().Add(-time.Duration(vizOpenFor * 0.45 * float64(time.Second)))
	m.vizStep()
	v0 := m.viz
	sum := func(c vcol) float64 { return c.r + c.g + c.b }
	if corner, mid := sum(v0.out[0]), sum(v0.out[(v0.h2/2)*v0.w+v0.w/2]); corner > 0.001 {
		t.Errorf("while it opens the corners are still dark: corner %.3f, middle %.3f", corner, mid)
	}
	m.viz.openedAt = time.Now().Add(-time.Minute) // open
	m.viz.hintUntil = time.Now().Add(time.Minute) // and its hint up, however slow this machine draws
	for scene := range sceneCount {
		m.viz.scene = scene
		for range 90 { // three seconds at 30 fps
			m.viz.last = time.Now().Add(-screenFrame)
			m.vizStep()
		}
		if !m.viz.live {
			t.Fatal("the visualizer should read the tapped samples")
		}
		frame := m.View().Content
		lines := strings.Split(frame, "\n")
		if len(lines) != m.height {
			t.Fatalf("%s: %d rows, want %d", vizSceneNames[scene], len(lines), m.height)
		}
		plain := ansi.Strip(frame)
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != m.width {
				t.Fatalf("%s: row %d is %d cells wide, want %d", vizSceneNames[scene], i, w, m.width)
			}
		}
		// Nothing but the picture and the esc hint: no tabs, no status bar.
		for _, gone := range []string{"Yandex", "play/pause", "Night Song"} {
			if strings.Contains(plain, gone) {
				t.Errorf("%s: the visualizer shows %q", vizSceneNames[scene], gone)
			}
		}
		if !strings.Contains(plain, "esc") {
			t.Errorf("%s: the esc hint should show right after opening", vizSceneNames[scene])
		}
		lit := 0
		for _, c := range m.viz.out {
			if c.r+c.g+c.b > 0.3 {
				lit++
			}
		}
		if lit < len(m.viz.out)/50 {
			t.Errorf("%s: the picture is almost black (%d lit pixels)", vizSceneNames[scene], lit)
		}
		dumpViz(t, m.viz, vizSceneNames[scene])
	}
	if m.viz.bass < 0.2 {
		t.Errorf("the kick should register as bass, got %.2f", m.viz.bass)
	}

	// The hint fades away.
	m.viz.hintUntil = time.Now().Add(-time.Second)
	m.vizStep()
	if strings.Contains(ansi.Strip(m.View().Content), "esc") {
		t.Error("the esc hint should be gone after a moment")
	}
}

func TestVisualizer_Keys(t *testing.T) {
	m := vizModel(t)
	mp := m.player.(*mockPlayer)
	cmd := m.handleNormalKey(tea.KeyPressMsg{Code: 'V', Text: "V"}, "V")
	if cmd == nil {
		t.Fatal("opening the visualizer should start its frame clock")
	}

	m.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.viz.scene != 1 || m.cfg.VisualizerScene != vizSceneNames[1] {
		t.Errorf("tab should switch to the next scene and remember it, got %d %q", m.viz.scene, m.cfg.VisualizerScene)
	}
	m.vizStep() // the next frame
	if !strings.Contains(ansi.Strip(m.View().Content), vizSceneNames[1]) {
		t.Error("switching scenes should name the new one")
	}

	// Keys of the rest of the app don't leak through…
	m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if m.activePanel >= 0 {
		t.Error("l should do nothing inside the visualizer")
	}
	// …but the playback ones work.
	if c := m.handleKey(tea.KeyPressMsg{Code: 'n', Text: "n"}); c != nil {
		runCmd(c)
	}
	if !mp.nextCalled {
		t.Error("n should still skip to the next song")
	}

	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.vizOpen() {
		t.Fatal("esc should close the visualizer")
	}
	if strings.Contains(m.View().Content, "▀▀▀▀▀▀▀▀▀▀") {
		t.Error("the player should be back after esc")
	}
	// A stale tick from the closed visualizer stops its clock.
	if _, c := m.Update(vizTickMsg{gen: m.viz.gen}); c != nil {
		for _, msg := range runCmd(c) {
			if _, ok := msg.(vizTickMsg); ok {
				t.Error("a closed visualizer should not keep ticking")
			}
		}
	}

	// Reopening restores the remembered scene.
	m.viz = nil
	m.handleNormalKey(tea.KeyPressMsg{Code: 'V', Text: "V"}, "V")
	if m.viz.scene != 1 {
		t.Errorf("the remembered scene should come back, got %d", m.viz.scene)
	}
}

func TestVisualizer_NoSamplesStillDances(t *testing.T) {
	tap.Register(func([]float32) (int, float64, int64) { return 0, 0, 0 })
	m := vizModel(t)
	m.openVisualizer()
	m.playerState.Position = 1250 * time.Millisecond
	for range 5 {
		m.viz.listen(true, m.playerState.Position, 0.05)
		m.viz.analyse(0.05)
	}
	if m.viz.live || m.viz.level < 0.1 {
		t.Errorf("without samples the visualizer should make up a spectrum (live=%v level=%.2f)", m.viz.live, m.viz.level)
	}
	m.playerState.Playing = false
	for range 60 {
		m.viz.analyse(0.05)
		m.viz.listen(false, 0, 0.05)
	}
	if m.viz.level > 0.05 {
		t.Errorf("paused, the spectrum should fall silent, got %.2f", m.viz.level)
	}
}

func TestVizPaletteFromCover(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 30, 30))
	for y := range 30 {
		for x := range 30 {
			c := color.RGBA{200, 30, 30, 255} // red
			if x >= 15 {
				c = color.RGBA{30, 40, 210, 255} // blue
			}
			img.Set(x, y, c)
		}
	}
	pal := vizPalette(img)
	var red, blue bool
	for _, c := range pal {
		red = red || (c.r > 0.9 && c.b < 0.5)
		blue = blue || (c.b > 0.9 && c.r < 0.5)
	}
	if !red || !blue {
		t.Errorf("the palette should have the cover's red and blue, got %+v", pal)
	}
	grey := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range grey.Pix {
		grey.Pix[i] = 128
	}
	for _, c := range vizPalette(grey) {
		if _, sat, _ := hsv(c); sat > 0.3 {
			t.Errorf("a grey cover should give greys, got %+v", vizPalette(grey))
		}
	}

	// A cover of one hue gives shades of it, not hues it doesn't have.
	mono := image.NewRGBA(image.Rect(0, 0, 30, 30))
	for y := range 30 {
		for x := range 30 {
			mono.Set(x, y, color.RGBA{uint8(120 + 4*x), 20, 30, 255}) // reds
		}
	}
	for _, c := range vizPalette(mono) {
		if h := firstHueOf(c); h > 0.1 && h < 0.9 {
			t.Errorf("a red cover gave a colour of hue %.2f: %+v", h, vizPalette(mono))
		}
	}
	for _, hex := range coverAccents(mono) {
		var r, g, b int
		_, _ = fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
		if h := firstHueOf(vcol{float64(r) / 255, float64(g) / 255, float64(b) / 255}); h > 0.1 && h < 0.9 {
			t.Errorf("the text of a red cover took %s", hex)
		}
	}
	for _, hex := range coverAccents(grey) {
		var r, g, b int
		_, _ = fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
		if max(r, g, b)-min(r, g, b) > 60 {
			t.Errorf("the text of a grey cover took %s", hex)
		}
	}
}

func TestFFTFindsTheTone(t *testing.T) {
	a := make([]complex128, 256)
	for i := range a {
		a[i] = complex(math.Sin(2*math.Pi*16*float64(i)/256), 0)
	}
	fft(a)
	best := 0
	for i := 1; i < 128; i++ {
		if abs(a[i]) > abs(a[best]) {
			best = i
		}
	}
	if best != 16 {
		t.Errorf("peak at bin %d, want 16", best)
	}
}

func abs(c complex128) float64 { return math.Hypot(real(c), imag(c)) }

// dumpViz writes the frame as a PNG when VIZ_DUMP names a directory.
func dumpViz(t *testing.T, v *visualizer, name string) {
	dir := os.Getenv("VIZ_DUMP")
	if dir == "" {
		return
	}
	const k = 4
	img := image.NewRGBA(image.Rect(0, 0, v.w*k*2, v.h*2*k*2))
	for y := range v.h * 2 {
		for x := range v.w {
			c := toByte3(v.out[y*v.w+x])
			for dy := range k * 2 {
				for dx := range k * 2 {
					img.Set(x*k*2+dx, y*k*2+dy, color.RGBA{c[0], c[1], c[2], 255})
				}
			}
		}
	}
	f, err := os.Create(filepath.Join(dir, name+".png")) //nolint:gosec // a directory the tester asked for
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// On the Linux console the frame is drawn with the 16 colours the
// visualizer programs itself, and the app's palette comes back on closing.
func TestVisualizer_Console(t *testing.T) {
	fakeTap(t)
	m := vizModel(t)
	m.viz = newVisualizer()
	m.viz.console = true
	cmd := m.openVisualizer()
	if cmd == nil || !strings.Contains(m.viz.con.seq, "\x1b]Pf") {
		t.Fatal("opening on the console should program the palette")
	}
	if m.viz.rawSeq != "" {
		t.Error("the palette sequence should have been handed over")
	}
	for scene := range sceneCount {
		m.viz.scene = scene
		for range 45 {
			m.viz.last = time.Now().Add(-screenFrame)
			m.vizStep()
		}
		frame := m.View().Content
		if strings.Contains(frame, "38;2") || strings.Contains(frame, "48;2") {
			t.Fatalf("%s: true colour on the console", vizSceneNames[scene])
		}
		lines := strings.Split(frame, "\n")
		if len(lines) != m.height {
			t.Fatalf("%s: %d rows, want %d", vizSceneNames[scene], len(lines), m.height)
		}
		glyphs := map[rune]int{}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != m.width {
				t.Fatalf("%s: row %d is %d cells wide, want %d", vizSceneNames[scene], i, w, m.width)
			}
			if i == 1 || i == m.height-2 {
				continue // the texts
			}
			for _, r := range ansi.Strip(l) {
				if !strings.ContainsRune(" ▀▄█░▒▓", r) {
					t.Fatalf("%s: glyph %q is not in the console font", vizSceneNames[scene], r)
				}
				glyphs[r]++
			}
		}
		if len(glyphs) < 4 {
			t.Errorf("%s: only %d kinds of glyphs, the picture is flat", vizSceneNames[scene], len(glyphs))
		}
		dumpVizConsole(t, m.viz, vizSceneNames[scene]+"-console")
	}
	// A new cover changes the colours: the palette is sent again.
	m.viz.palURL = "other"
	m.vizStep()
	if m.viz.rawSeq == "" {
		t.Error("a new cover should reprogram the console palette")
	}
}

func TestVizConPalette(t *testing.T) {
	c := newVizCon(defaultVizPalette)
	if c.pal[0] != [3]float64{} || c.pal[15] != [3]float64{255, 255, 255} {
		t.Errorf("the palette should run from black to white, got %v … %v", c.pal[0], c.pal[15])
	}
	// A flat cell is drawn as one colour; a sharp edge as two halves.
	if _, _, ch := c.cell([3]uint8{0, 0, 0}, [3]uint8{0, 0, 0}); ch != " " {
		t.Errorf("black should be an empty cell, got %q", ch)
	}
	if fg, bg, ch := c.cell([3]uint8{255, 255, 255}, [3]uint8{0, 0, 0}); ch != "▀" || fg != 15 || bg != 0 {
		t.Errorf("white over black should be a white ▀ on black, got %q %d %d", ch, fg, bg)
	}
	if fg, bg, ch := c.cell([3]uint8{0, 0, 0}, [3]uint8{255, 255, 255}); ch != "▄" || fg != 15 || bg != 0 {
		t.Errorf("black over white should be a white ▄ on black, got %q %d %d", ch, fg, bg)
	}
	// A background never gets a bright colour.
	for _, m := range c.mixes {
		if m.bg >= 8 {
			t.Fatalf("background %d is out of the console's reach", m.bg)
		}
	}
}

// dumpVizConsole writes the console frame as a PNG when VIZ_DUMP names a
// directory: every cell as the console would show it, shade glyphs as
// checkered dots.
func dumpVizConsole(t *testing.T, v *visualizer, name string) {
	dir := os.Getenv("VIZ_DUMP")
	if dir == "" {
		return
	}
	const cw, ch = 8, 16
	img := image.NewRGBA(image.Rect(0, 0, v.w*cw, v.h*ch))
	rgb := func(i int) color.RGBA {
		if i < 0 {
			i = 0
		}
		q := v.con.pal[i]
		return color.RGBA{uint8(q[0]), uint8(q[1]), uint8(q[2]), 255}
	}
	for row := range v.h {
		for col := range v.w {
			fg, bg, g := v.con.cell(toByte3(v.out[(row*2)*v.w+col]), toByte3(v.out[(row*2+1)*v.w+col]))
			for y := range ch {
				for x := range cw {
					on := false
					switch g {
					case "▀":
						on = y < ch/2
					case "▄":
						on = y >= ch/2
					case "█":
						on = true
					case "░":
						on = x%2 == 0 && y%2 == 0 && (x/2+y/2)%2 == 0 || x%4 == 2 && y%4 == 2
					case "▒":
						on = (x+y)%2 == 0
					case "▓":
						on = (x%2 != 0 || y%2 != 0 || (x/2+y/2)%2 != 0) && (x%4 != 2 || y%4 != 2)
					}
					c := rgb(bg)
					if on {
						c = rgb(fg)
					}
					img.Set(col*cw+x, row*ch+y, c)
				}
			}
		}
	}
	f, err := os.Create(filepath.Join(dir, name+".png")) //nolint:gosec // a directory the tester asked for
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// A drum over a steady bass line and a chord: every hit must be caught and
// nothing else taken for one. When the player hands the sound out ahead of
// time, the hit is shown so that it is seen (vizLag after the frame is made)
// when it sounds; when it can't, as soon as the hit is found.
func TestVisualizer_BeatsLandOnTheDrum(t *testing.T) {
	const rate, period = 48000, 0.5
	for _, ahead := range []float64{0.18, 0} {
		now := 0.0
		tap.Register(func(dst []float32) (int, float64, int64) {
			for i := range dst {
				ts := now + ahead - float64(len(dst)-i)/rate
				since := math.Mod(ts, period)
				drum := math.Exp(-since*14) * math.Sin(2*math.Pi*60*since)
				line := 0.25 * math.Sin(2*math.Pi*98*ts)
				chord := 0.15 * (math.Sin(2*math.Pi*440*ts) + math.Sin(2*math.Pi*660*ts) + math.Sin(2*math.Pi*2640*ts))
				dst[i] = float32(0.7*drum + line + chord)
			}
			return rate, ahead, int64((now + ahead) * rate)
		})
		t.Cleanup(func() { tap.Register(func([]float32) (int, float64, int64) { return 0, 0, 0 }) })

		v := newVisualizer()
		const dt = 1.0 / 30
		beats := 0
		early, late := 1.0, -1.0
		for now = 1; now < 10.5; now += dt {
			v.listen(true, 0, dt)
			v.analyse(dt)
			if !v.onBeat || now < 4.5 { // the first seconds teach it the song
				continue
			}
			beats++
			seen := now + vizLag
			off := seen - math.Round(seen/period)*period
			early, late = min(early, off), max(late, off)
		}
		t.Logf("%.0f ms ahead: %d beats, seen from %+.0f to %+.0f ms around the hit", ahead*1000, beats, early*1000, late*1000)
		if beats < 11 || beats > 13 {
			t.Errorf("%.0f ms ahead: %d beats in six seconds of a 120 bpm drum, want 12", ahead*1000, beats)
		}
		if ahead > 0 && (early < -0.025 || late > 0.025) {
			t.Errorf("with the sound in hand the hits should be seen within a frame of the drum, got %+.0f…%+.0f ms", early*1000, late*1000)
		}
		if ahead == 0 && (early < 0 || late > 0.15) {
			t.Errorf("without it they can only be late, and not by much: got %+.0f…%+.0f ms", early*1000, late*1000)
		}
	}
}

// i shows what the visualizer hears, and hides it again.
func TestVisualizer_InfoLine(t *testing.T) {
	fakeTap(t)
	m := vizModel(t)
	m.openVisualizer()
	m.handleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})
	for range 10 {
		m.viz.last = time.Now().Add(-screenFrame)
		m.vizStep()
	}
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "sound +0 ms") || !strings.Contains(plain, "hits/s") {
		t.Errorf("i should show where the sound comes from, got %q", strings.Split(plain, "\n")[1])
	}
	m.handleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m.viz.hintUntil = time.Time{}
	m.vizStep()
	if strings.Contains(ansi.Strip(m.View().Content), "hits/s") {
		t.Error("i again should hide it")
	}
}

// The terminal's own margin (kitty's window_padding_width, say) takes the
// colour of the picture's edges while the visualizer is open, so it doesn't
// frame the picture; when it closes the terminal gets its background back.
func TestVisualizer_MarginTakesThePicturesColour(t *testing.T) {
	fakeTap(t)
	m := vizModel(t)
	if m.View().BackgroundColor != nil {
		t.Fatal("the background must be left alone while the visualizer is closed")
	}
	m.openVisualizer()
	for range 10 {
		m.viz.last = time.Now().Add(-screenFrame)
		m.vizStep()
	}
	bg := m.View().BackgroundColor
	if bg == nil {
		t.Fatal("the visualizer should set the terminal's background")
	}
	r, g, b, _ := bg.RGBA()
	e := m.viz.edge
	if uint8(r>>8) != e[0] || uint8(g>>8) != e[1] || uint8(b>>8) != e[2] { //nolint:gosec // 16-bit channels: their top byte
		t.Errorf("background %v, want the edge colour %v", bg, e)
	}
	m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.View().BackgroundColor != nil {
		t.Error("esc should give the terminal its background back")
	}
}

func firstHueOf(c vcol) float64 { h, _, _ := hsv(c); return h }

func TestViz_BrightnessKeys(t *testing.T) {
	m := newModel(nil)
	m.viz = newVisualizer()
	m.viz.open = true
	m.handleVizKey("{")
	if m.viz.bright != 0.9 || m.cfg.VisualizerBrightness == nil || *m.cfg.VisualizerBrightness != 90 {
		t.Fatalf("{ should dim to 90%%: %v %v", m.viz.bright, m.cfg.VisualizerBrightness)
	}
	m.handleVizKey("}")
	if m.viz.bright != 1 || m.cfg.VisualizerBrightness != nil {
		t.Errorf("} should come back to 100%% (unset in the config): %v", m.viz.bright)
	}
	for range 20 {
		m.handleVizKey("{")
	}
	if m.viz.bright != 0.1 {
		t.Errorf("the floor is 10%%: %v", m.viz.bright)
	}
}

// The scenes' quick remainders give what math.Mod does.
func TestVisualizer_Fmod(t *testing.T) {
	seed := uint32(7)
	next := func() float64 {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		return (float64(seed)/math.MaxUint32 - 0.5) * 2000
	}
	for range 100000 {
		x := next()
		if a, b := fmod1(x), math.Mod(x, 1); a != b {
			t.Fatalf("fmod1(%v) = %v, math.Mod gives %v", x, a, b)
		}
		if a, b := fmodN(x, 2), math.Mod(x, 2); a != b {
			t.Fatalf("fmodN(%v, 2) = %v, math.Mod gives %v", x, a, b)
		}
		if a, b := fmodN(math.Floor(x), 72), math.Mod(math.Floor(x), 72); a != b {
			t.Fatalf("fmodN(%v, 72) = %v, math.Mod gives %v", math.Floor(x), a, b)
		}
	}
}

// The scenes' quick sine keeps to math's far closer than a colour can show.
func TestVisualizer_FastSin(t *testing.T) {
	worst := 0.0
	for i := range 200000 {
		x := (float64(i)/200000 - 0.5) * 2000
		worst = math.Max(worst, math.Abs(fsin(x)-math.Sin(x)))
	}
	if worst > 1e-6 {
		t.Errorf("off by up to %.2g", worst)
	}
}
