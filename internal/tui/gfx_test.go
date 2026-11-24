package tui

import (
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

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/gfx"
)

// Tests draw in cells whatever terminal they are run from.
func init() { gfxDetect = func(string) gfx.Mode { return gfx.Off } }

// In a terminal that shows pictures the visualizer sends each frame as one,
// at several pixels a cell, and the cells under it stay empty.
func TestVisualizer_Pixels(t *testing.T) {
	gfxDetect = func(string) gfx.Mode { return gfx.Shm }
	t.Cleanup(func() { gfxDetect = func(string) gfx.Mode { return gfx.Off } })
	fakeTap(t)
	m := vizModel(t)
	m.cfg.VisualizerDetail = os.Getenv("VIZ_DETAIL")
	m.handleNormalKey(tea.KeyPressMsg{Code: 'V', Text: "V"}, "V")
	v := m.viz
	if v.px < 2 {
		t.Fatalf("px = %d, want a finer picture", v.px)
	}
	cells := m.width * m.height
	// The first opaque frame stays above the old text until the view has
	// switched to the visualizer; only the following frame goes below it.
	v.openedAt = time.Now().Add(-time.Duration((vizOpenFor - 0.1) * float64(time.Second)))
	m.vizStep()
	if !v.over {
		t.Fatal("opening frame should be over the interface")
	}
	v.openedAt = time.Now().Add(-time.Duration((vizOpenFor + 0.1) * float64(time.Second)))
	m.vizStep()
	if v.over || v.img.Z != 1 {
		t.Fatal("first complete frame must cover stale interface text")
	}
	m.vizStep()
	if v.img.Z != -1 {
		t.Fatal("later frames should be behind visualizer text")
	}
	// Opened: no longer a circle growing over the interface.
	v.openedAt = time.Now().Add(-time.Duration((vizOpenFor + 1) * float64(time.Second)))
	for scene := range sceneCount {
		v.scene = scene
		var took time.Duration
		for range 45 {
			v.last = time.Now().Add(-screenFrame)
			start := time.Now()
			m.vizStep()
			took += time.Since(start)
		}
		if v.w != m.width*v.px || v.h2 != m.height*2*v.px {
			t.Fatalf("%s: picture %dx%d for %dx%d cells at %d", vizSceneNames[scene], v.w, v.h2, m.width, m.height, v.px)
		}
		if !strings.Contains(v.rawSeq, "a=T,f=24") || !strings.Contains(v.rawSeq, "t=s") {
			t.Fatalf("%s: no picture sent: %q", vizSceneNames[scene], v.rawSeq)
		}
		lines := strings.Split(v.frame, "\n")
		if len(lines) != m.height {
			t.Fatalf("%s: %d rows of cells, want %d", vizSceneNames[scene], len(lines), m.height)
		}
		lit := 0
		for i := 0; i < len(v.rgb); i += 3 {
			if int(v.rgb[i])+int(v.rgb[i+1])+int(v.rgb[i+2]) > 60 {
				lit++
			}
		}
		if lit < len(v.rgb)/3/200 {
			t.Errorf("%s: the picture is all but black (%d lit pixels)", vizSceneNames[scene], lit)
		}
		t.Logf("%-9s %dx%d px (%d per cell pixel, %d cells): %.1f ms a frame", vizSceneNames[scene], v.w, v.h2, v.px, cells, took.Seconds()*1000/45)
		if dir := os.Getenv("VIZ_PNG"); dir != "" {
			img := image.NewRGBA(image.Rect(0, 0, v.w, v.h2))
			for i := 0; i < v.w*v.h2; i++ {
				img.Set(i%v.w, i/v.w, color.RGBA{v.rgb[i*3], v.rgb[i*3+1], v.rgb[i*3+2], 255})
			}
			f, err := os.Create(filepath.Join(dir, vizSceneNames[scene]+".png")) //nolint:gosec // a directory the tester asked for
			if err != nil {
				t.Fatal(err)
			}
			_ = png.Encode(f, img)
			_ = f.Close()
		}
	}
	if cmd := m.closeVisualizer(); cmd == nil {
		t.Error("closing should take the picture off the screen")
	}
	if v.px != 1 {
		t.Error("closing goes back to cells")
	}
}

// The picture gets coarser while frames take too long and finer again once
// they don't: it must not stay coarse after a busy minute. A frame costs as
// many pixels as it has: at px it takes at(px).
func TestVizAdaptPx(t *testing.T) {
	frame := screenFrame.Seconds()
	// run gives the px after that many frames, starting at px, of a scene
	// that takes per (in frames) at px 5, and how many frames it took to
	// get there.
	run := func(px int, per float64, frames int) (int, int) {
		slow, fast, settled := 0, 0, 0
		for n := range frames {
			was := px
			px, slow, fast = vizAdaptPx(px, 5, per*frame*sq(float64(px)/5), slow, fast)
			if px != was {
				settled = n + 1
			}
		}
		return px, settled
	}
	if got, _ := run(5, 1.3, 60); got != 4 {
		t.Errorf("a little slow: px = %d, want a step down to 4", got)
	}
	if got, n := run(5, 2.2, 60); got != 3 || n > 10 {
		t.Errorf("far too slow: px = %d after %d frames, want 3 within a few", got, n)
	}
	if got, _ := run(2, 12, 500); got != 2 {
		t.Errorf("px = %d, must not go under 2", got)
	}
	if got, _ := run(2, 0.1, 1000); got != 5 {
		t.Errorf("quick frames: px = %d, want it back at 5", got)
	}
	if got, _ := run(4, 0.56*25.0/16, 1000); got != 4 {
		t.Errorf("px = %d: a step up would not fit, want 4", got)
	}
	if got, _ := run(4, 0.8*25.0/16, 1000); got != 4 {
		t.Errorf("px = %d: a frame that fits with a little room is no reason to coarsen", got)
	}
	if got, _ := run(4, 0.92*25.0/16, 100); got != 3 {
		t.Errorf("px = %d: a frame that leaves the rest of the frame no room should coarsen", got)
	}
	if got, n := run(5, 1.25, 60); got != 4 || n > 6 {
		t.Errorf("a heavy scene opened too fine: px = %d after %d frames, want 4 within a few", got, n)
	}
}

// The detail a scene settled at is where it starts the next time, on the
// same terminal; another scene starts at its own, a new size afresh.
func TestVizScenePx(t *testing.T) {
	v := &visualizer{pxMax: 5}
	if got := v.scenePx(); got != 5 {
		t.Fatalf("nothing known: px = %d, want the finest", got)
	}
	v.pxScene[sceneTunnel], v.pxOf = 3, 5
	v.scene = sceneTunnel
	if got := v.scenePx(); got != 3 {
		t.Errorf("tunnel: px = %d, want the 3 it kept up with", got)
	}
	v.scene = sceneSpectrum
	if got := v.scenePx(); got != 5 {
		t.Errorf("spectrum: px = %d, want its own (the finest)", got)
	}
	v.scene, v.pxMax = sceneTunnel, 4
	if got := v.scenePx(); got != 4 {
		t.Errorf("another size: px = %d, want the finest again", got)
	}
}

// With effects on, the picture closes back into the middle of the screen
// over the interface, and only then goes; a new scene melts out of the
// last one; another song's colours come over in a moment.
func TestVisualizer_Transitions(t *testing.T) {
	gfxDetect = func(string) gfx.Mode { return gfx.Shm }
	t.Cleanup(func() { gfxDetect = func(string) gfx.Mode { return gfx.Off } })
	fakeTap(t)
	m := vizModel(t)
	m.cfg.UIMotion = config.MotionCalm
	m.noAnim = false
	m.handleNormalKey(tea.KeyPressMsg{Code: 'V', Text: "V"}, "V")
	v := m.viz
	v.openedAt = time.Now().Add(-time.Minute)
	m.vizStep()

	// The scene: the first frame of the next one is still mostly the last.
	before := append([]vcol(nil), v.out...)
	m.handleVizKey("tab")
	melted := append([]vcol(nil), v.out...)
	if len(v.fade) == 0 {
		t.Fatal("a new scene melts out of the last")
	}
	v.fadeAt = time.Now().Add(-time.Minute) // the new scene alone
	v.last = time.Now().Add(-screenFrame)
	m.vizStep()
	toOld, toNew := 0.0, 0.0
	for i := range melted {
		toOld += math.Abs(melted[i].r - before[i].r)
		toNew += math.Abs(melted[i].r - v.out[i].r)
	}
	if len(melted) != len(before) || toOld >= toNew {
		t.Errorf("the first frame of the new scene is still mostly the last: %.1f from it, %.1f from the new", toOld, toNew)
	}

	// The colours: part way over, then there.
	v.palTo = [3]vcol{{1, 0, 0}, {1, 0, 0}, {1, 0, 0}}
	v.pal = [3]vcol{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}}
	v.last = time.Now().Add(-screenFrame)
	m.vizStep()
	if v.pal[0].r <= 0 || v.pal[0].r >= 0.5 {
		t.Errorf("another song's colours come over in a moment: %.2f of the way after a frame", v.pal[0].r)
	}

	// Closing.
	if cmd := m.closeVisualizer(); cmd != nil || !v.closing || m.vizOpen() {
		t.Fatal("it closes over the interface first, not at once")
	}
	v.closedAt = time.Now().Add(-time.Duration(vizCloseFor * 0.5 * float64(time.Second)))
	m.Update(vizTickMsg{v.gen})
	if !v.over || !v.img.Alpha || v.img.Z != 1 || len(v.alpha) == 0 {
		t.Error("half closed: a see-through picture over the interface")
	}
	mid, corner := v.alpha[(v.h2/2)*v.w+v.w/2], v.alpha[0]
	if mid < 0.9 || corner > 0.1 {
		t.Errorf("half closed: still there in the middle (%.2f), gone at the edges (%.2f)", mid, corner)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "esc  exit") {
		t.Error("the visualizer cells should remain until its closing image is placed")
	}
	m.Update(vizTickMsg{v.gen})
	m.Update(tea.RawMsg{Msg: "frame sent"})
	if strings.Contains(ansi.Strip(m.View().Content), "esc  exit") {
		t.Error("the interface should appear under the closing image")
	}
	v.closedAt = time.Now().Add(-time.Second)
	_, cmd := m.Update(vizTickMsg{v.gen})
	if v.closing || v.px != 1 || cmd == nil {
		t.Error("closed: the picture goes")
	}
}
