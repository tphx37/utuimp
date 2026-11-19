package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"maps"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/gfx"
	"github.com/tphx37/utuimp/internal/lyrics"
	"github.com/tphx37/utuimp/internal/tui/views"
)

// The picture of the progress line: strands over the played part that
// spread with the music and close up without it, see-through elsewhere.
func TestDrawProgress(t *testing.T) {
	const w, h, cell = 600, 60, 20.0
	pal := [3]vcol{{1, 0.4, 0.5}, {0.4, 0.7, 1}, {1, 0.8, 0.4}}
	fg := vcol{0.8, 0.8, 0.8}
	spread := func(amp float64) (lit int, rows int) {
		buf := make([]byte, w*h*4)
		drawProgress(buf, w, h, 0.6, amp, 3, cell, pal, fg, lineWave{})
		used := map[int]bool{}
		for y := range h {
			for x := range w {
				if a := buf[(y*w+x)*4+3]; a > 100 {
					lit++
					if x > 100 && x < 300 {
						used[y] = true
					}
				}
			}
		}
		if dir := os.Getenv("LINES_PNG"); dir != "" {
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			for i := range w * h {
				a := float64(buf[i*4+3]) / 255
				img.Set(i%w, i/w, color.RGBA{uint8(float64(buf[i*4])*a + 20*(1-a)), uint8(float64(buf[i*4+1])*a + 16*(1-a)), uint8(float64(buf[i*4+2])*a + 18*(1-a)), 255})
			}
			name := "/still.png"
			if amp > 0 {
				name = "/loud.png"
			}
			f, _ := os.Create(dir + name) //nolint:gosec // a directory the tester asked for
			_ = png.Encode(f, img)
			_ = f.Close()
		}
		return lit, len(used)
	}
	litStill, rowsStill := spread(0)
	litLoud, rowsLoud := spread(1)
	if litStill == 0 || litLoud == 0 {
		t.Fatal("nothing drawn")
	}
	if rowsStill > 4 {
		t.Errorf("without music the strands lie in one line, got %d rows", rowsStill)
	}
	if rowsLoud <= rowsStill+3 {
		t.Errorf("with music the strands spread: %d rows against %d", rowsLoud, rowsStill)
	}
	if max := int(2*linesSway*cell) + 8; rowsLoud > max {
		t.Errorf("the strands sway only a little: %d rows, at most %d", rowsLoud, max)
	}
	// The light has no frame: nothing at all along the picture's edges and
	// just past the point where the song is.
	buf := make([]byte, w*h*4)
	drawProgress(buf, w, h, 0.6, 1, 3, cell, pal, fg, lineWave{})
	head := w * 6 / 10
	for x := range w {
		for _, y := range []int{0, h - 1} {
			if a := buf[(y*w+x)*4+3]; a > 0 {
				t.Fatalf("light reaches the picture's edge at %d,%d (alpha %d)", x, y, a)
			}
		}
	}
	for y := range h {
		if d := y - h/2; d < -4 || d > 4 {
			// It thins out a little way past the point, not at it (an end
			// that stops dead looks like a frame); well past it there is none.
			for _, x := range []int{head + 8, head + 30} {
				limit := byte(8)
				if x == head+30 {
					limit = 0
				}
				if a := buf[(y*w+x)*4+3]; a > limit {
					t.Fatalf("the light should thin out past the song's point: alpha %d at %d,%d", a, x, y)
				}
			}
		}
	}
	if litLoud > w*h/4 {
		t.Errorf("most of the picture stays see-through, %d of %d pixels are solid", litLoud, w*h)
	}
}

// In a terminal with pictures the progress line's cells are emptied and its
// place is noted; elsewhere the screen is left as it is.
func TestLinesPlace(t *testing.T) {
	m := coverModel(t, 100, 30)
	m.playerState.Playing = true
	plain := m.View().Content
	if !strings.Contains(plain, "━") && !strings.Contains(plain, "─") {
		t.Skip("no progress line in this layout")
	}
	if m.lines.seen() {
		t.Fatal("no pictures here: nothing to place")
	}
	m.lines.mode = gfx.Shm
	out := m.View().Content
	st := m.lines.strips[stripProgress]
	if !st.seen || st.cols < 4 {
		t.Fatalf("the progress line should be found: %+v", st)
	}
	vol := m.lines.strips[stripVolume]
	rows, was := strings.Split(out, "\n"), strings.Split(plain, "\n")
	if len(rows) != len(was) {
		t.Fatalf("%d rows, want %d", len(rows), len(was))
	}
	y := st.row + 1 - m.hostRows
	if ansi.StringWidth(rows[y]) != ansi.StringWidth(was[y]) {
		t.Errorf("the row keeps its width: %d, was %d", ansi.StringWidth(rows[y]), ansi.StringWidth(was[y]))
	}
	got := []rune(ansi.Strip(rows[y]))
	for x := st.col; x < st.col+st.cols; x++ {
		if got[x] != ' ' {
			t.Fatalf("cell %d of the line should be empty, got %q in %q", x, got[x], string(got))
		}
	}
	_ = vol
	// The cover, the frames and the rules are pictures too; every row
	// keeps its width whatever was emptied in it.
	for i := range rows {
		if ansi.StringWidth(rows[i]) != ansi.StringWidth(was[i]) {
			t.Errorf("row %d: width %d, was %d", i, ansi.StringWidth(rows[i]), ansi.StringWidth(was[i]))
		}
	}
	big := coverModel(t, 140, 44)
	big.coverView = true
	big.lines.mode = gfx.Shm
	big.View()
	if !big.lines.strips[stripCover].seen || !big.lines.strips[stripHalo].seen {
		t.Error("with room for it the cover should be a picture, with its light")
	}
}

// The line is found whatever its tip: the bar grows in half cells, so every
// other step ends in ╸.
func TestLinesFindHalfStepTip(t *testing.T) {
	l := newUILines()
	for _, row := range []string{
		"0:44  ━━━━━━──────────  4:04",
		"0:44  ━━━━━━╸─────────  4:04",
		"0:00  ────────────────  4:04",
		"1:02:03  ━━━━━━━━━╸────  2:00:00",
	} {
		if l.strips[stripProgress].re.FindStringSubmatchIndex(row) == nil {
			t.Errorf("not found in %q", row)
		}
	}
	if l.strips[stripVolume].re.FindStringSubmatchIndex("   ♪  ━━━━━╸──────  45% ") == nil {
		t.Error("the volume line with a half step is not found")
	}
}

// A rule is a run of ─ on its own; a frame's edge and the progress line are
// not rules.
func TestLinesRuleIn(t *testing.T) {
	for row, want := range map[string]bool{
		"│ ─────────────── │": true,
		"  Queue": false,
		"  ─────": true,
		"╭─ Now Playing ──────────╮":   false,
		"╰──────────── ♪ 80% ─╯":       false,
		"0:44  ━━━━━━╸─────────  4:04": false,
		"x─────": false,
		"  ────": false,
	} {
		if _, _, ok := linesRuleIn(row); ok != want {
			t.Errorf("%q: rule = %v, want %v", row, ok, want)
		}
	}
}

// The rule and the ribbons behind the lyrics: faint, and nothing at the
// picture's edges.
func TestDrawRuleAndBack(t *testing.T) {
	const w, h = 400, 20
	buf := make([]byte, w*h*4)
	drawRule(buf, w, h, 1, 2, 20, 1, 0, vcol{0.8, 0.8, 0.8})
	lit := 0
	for i := 3; i < len(buf); i += 4 {
		if buf[i] > 60 {
			lit++
		}
	}
	if lit < w/2 || lit > w*5 {
		t.Errorf("a rule is one fine line: %d solid pixels", lit)
	}
	if buf[(h/2*w)*4+3] != 0 || buf[(h/2*w+w-1)*4+3] != 0 {
		t.Error("a rule fades out to nothing at its ends")
	}
	drawRule(buf, w, h, 1, 2, 20, 0.2, 0, vcol{0.8, 0.8, 0.8})
	for y := range h {
		if buf[(y*w+w*3/4)*4+3] != 0 {
			t.Fatal("a rule a fifth drawn in has not reached its far part")
		}
	}

	const bw, bh = 600, 54
	back := make([]byte, bw*bh*4)
	drawWave(back, bw, bh, 3, 1, 1, 1, 0, vcol{1, 0.4, 0.5})
	seen := 0
	for y := range bh {
		for x := range bw {
			a := int(back[(y*bw+x)*4+3])
			if a > 60 {
				seen++
			}
			if (x == 0 || y == 0 || x == bw-1 || y == bh-1) && a > 1 {
				t.Fatalf("the wave reaches the picture's edge at %d,%d", x, y)
			}
		}
	}
	if seen < bw/4 || seen > bw*4 {
		t.Errorf("the wave is one fine line: %d solid pixels", seen)
	}
	drawWave(back, bw, bh, 3, 1, 1, 0.1, 0, vcol{1, 0.4, 0.5})
	for y := range bh {
		if back[(y*bw+bw*3/4)*4+3] != 0 {
			t.Fatal("a wave just begun has not reached its far part")
		}
	}
}

// A frame is found whole, with the cells of a title left out of its line;
// one that something lies over is not a frame.
func TestLinesBoxes(t *testing.T) {
	grid := func(rows ...string) [][]rune {
		g := make([][]rune, len(rows))
		for i, r := range rows {
			g[i] = cellGrid(r)
		}
		return g
	}
	boxes := linesBoxes(grid(
		"  ╭─ Title ───╮ ",
		"  │ words     │ ",
		"  ╰───────────╯ ",
	))
	if len(boxes) != 1 {
		t.Fatalf("%d frames, want 1", len(boxes))
	}
	b := boxes[0]
	if b.x0 != 2 || b.x1 != 14 || b.y0 != 0 || b.y1 != 2 {
		t.Errorf("frame at %+v", b)
	}
	if !b.top[0] || !b.top[1] || b.top[2] || b.top[4] || !b.top[12] {
		t.Errorf("the title is not line: %v", b.top)
	}
	if got := linesBoxes(grid(
		"  ╭───────────╮ ",
		"  │ words   menu ",
		"  ╰───────────╯ ",
	)); len(got) != 0 {
		t.Errorf("a covered frame was taken for a whole one: %+v", got)
	}
}

// A side of a frame: a fine line all the way, joined to its neighbours
// through the corners, and nothing over a title.
func TestDrawFrameSide(t *testing.T) {
	const cw, chh = 10, 20
	mask := make([]bool, 30)
	for i := range mask {
		mask[i] = i < 2 || i > 9
	}
	look := frameLook{amp: 1, t: 2, reveal: 1, focused: true, lit: 1, pal: [3]vcol{{1, 0.4, 0.5}, {0.4, 0.7, 1}, {1, 0.8, 0.4}}, fg: vcol{0.8, 0.8, 0.8}}
	w, h := 30*cw, chh
	buf := make([]byte, w*h*4)
	drawFrameSide(buf, w, h, sideTop, 30, 10, mask, cw, chh, look)
	col := func(x int) int {
		most := 0
		for y := range h {
			most = max(most, int(buf[(y*w+x)*4+3]))
		}
		return most
	}
	if col(5*cw+5) != 0 {
		t.Error("the line runs through the title")
	}
	if col(20*cw) < 150 {
		t.Error("no line along the top")
	}
	// The corner leaves the row at its bottom edge, in the middle of the
	// first column, where the left side takes over.
	if a := buf[((h-1)*w+cw/2)*4+3]; a < 100 {
		t.Errorf("the corner does not reach the side: alpha %d", a)
	}
	side := make([]byte, cw*100*4)
	drawFrameSide(side, cw, 100, sideLeft, 30, 10, nil, cw, chh, look)
	for _, y := range []int{0, 99} {
		if a := side[(y*cw+cw/2)*4+3]; a < 100 {
			t.Errorf("the side does not start in the middle of its column at row %d: alpha %d", y, a)
		}
	}
	look.reveal = 0.1
	drawFrameSide(buf, w, h, sideBottom, 30, 10, mask, cw, chh, look)
	for i := 3; i < len(buf); i += 4 {
		if buf[i] != 0 {
			t.Fatal("a frame just begun has not reached its bottom")
		}
	}
}

func TestFrameCornersMeetSides(t *testing.T) {
	for _, cw := range []int{9, 10} {
		const ch = 20
		const cols = 12
		mask := make([]bool, cols)
		for i := range mask {
			mask[i] = true
		}
		look := frameLook{reveal: 1, lit: 1, pad: 1}
		row := make([]byte, cols*cw*3*ch*4)
		column := make([]byte, 3*cw*ch*4)
		for _, horizontal := range []int{sideTop, sideBottom} {
			drawFrameSide(row, cols*cw, 3*ch, horizontal, cols, 3, mask, float64(cw), ch, look)
			y := 2*ch - 1
			if horizontal == sideBottom {
				y = ch
			}
			for _, vertical := range []int{sideLeft, sideRight} {
				drawFrameSide(column, 3*cw, ch, vertical, cols, 3, nil, float64(cw), ch, look)
				for x := range cw {
					rx := x
					if vertical == sideRight {
						rx += (cols - 1) * cw
					}
					a := int(row[(y*cols*cw+rx)*4+3])
					b := int(column[(cw+x)*4+3])
					if a-b > 1 || b-a > 1 {
						t.Fatalf("cell width %d, sides %d/%d, x %d: corner alpha %d, side alpha %d", cw, horizontal, vertical, x, a, b)
					}
				}
			}
		}
	}
}

// With LINES_PNG set, the pictures of a whole screen are laid out as the
// terminal would show them (without the text), to be looked at.
func TestLinesScreenPNG(t *testing.T) {
	dir := os.Getenv("LINES_PNG")
	if dir == "" {
		t.Skip("set LINES_PNG to a directory")
	}
	fakeTap(t)
	for _, name := range []string{"lyrics", "panel", "boxed", "press", "scroll", "song"} {
		m := coverModel(t, 150, 44)
		m.coverView = name == "lyrics" || name == "panel"
		m.lines.mode = gfx.Shm
		m.lines.cellW, m.lines.cellH = 9, 18
		big := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for i := range 64 * 64 {
			big.Set(i%64, i/64, color.RGBA{uint8(40 + 3*(i%64)), uint8(60 + 2*(i/64)), 160, 255})
		}
		m.artwork.img = big
		if name == "lyrics" {
			m.coverQueueOpen, m.queueMiniCursor, m.activePanel = false, -1, m.lyricsPanelIdx()
			m.lyricsP.m.SetLyrics(&lyrics.Result{Lines: []lyrics.Line{
				{Text: "first line of the song"}, {Text: "the second one is a little longer than that"},
				{Text: "third"}, {Text: "and the fourth line"}, {Text: "fifth line here"}, {Text: "sixth"}, {Text: "seventh line of it"},
			}}, nil)
		}
		if name == "panel" {
			m.queueMiniCursor = 0
			m.coverQueueOpen = true
			m.handleNormalKey(tea.KeyPressMsg{Code: 'q', Text: "q"}, "q")
		}
		screen := m.View().Content
		for i := range m.lines.strips {
			m.lines.strips[i].born = time.Now().Add(-time.Minute)
		}
		for range 40 {
			m.linesDraw(0.033, time.Now())
		}
		m.lines.bass, m.lines.amp = 1, 1
		// A touch, a little while ago: the frames are bent by it.
		ago := func(sec float64) time.Time { return time.Now().Add(time.Duration(-sec * float64(time.Second))) }
		switch name {
		case "press":
			m.lines.touches = []lineTouch{{kind: touchPress, dir: 0, at: ago(0.1)}}
		case "scroll":
			m.lines.touches = []lineTouch{{kind: touchScroll, dir: 1, at: ago(0.16)}}
		case "song":
			m.lines.touches = []lineTouch{{kind: touchSong, dir: 1, at: ago(0.3)}}
		}
		m.linesDraw(0.033, time.Now())
		m.linesDraw(0.033, time.Now())
		const cw, chh = 9, 18
		out := image.NewRGBA(image.Rect(0, 0, m.width*cw, m.height*chh))
		for i := range out.Pix {
			out.Pix[i] = 24
			if i%4 == 3 {
				out.Pix[i] = 255
			}
		}
		order := []int{stripHalo}
		for i := range m.lines.strips {
			if i != stripHalo {
				order = append(order, i)
			}
		}
		for _, i := range order {
			st := m.lines.strips[i]
			if !st.seen || st.pw == 0 || len(st.rgba) != st.pw*st.ph*4 {
				continue
			}
			W, H := st.cols*cw, st.rows*chh
			at := func(x, y, c int) float64 {
				x, y = max(0, min(st.pw-1, x)), max(0, min(st.ph-1, y))
				return float64(st.rgba[(y*st.pw+x)*4+c])
			}
			for y := range H {
				for x := range W {
					// As the terminal spreads a small picture: smoothly.
					fx := (float64(x)+0.5)*float64(st.pw)/float64(W) - 0.5
					fy := (float64(y)+0.5)*float64(st.ph)/float64(H) - 0.5
					x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
					kx, ky := fx-float64(x0), fy-float64(y0)
					var v [4]float64
					for c := range 4 {
						v[c] = (at(x0, y0, c)*(1-kx)+at(x0+1, y0, c)*kx)*(1-ky) + (at(x0, y0+1, c)*(1-kx)+at(x0+1, y0+1, c)*kx)*ky
					}
					a := v[3] / 255
					px, py := st.col*cw+x, st.row*chh+y
					if px < 0 || py < 0 || px >= m.width*cw || py >= m.height*chh {
						continue
					}
					o := out.PixOffset(px, py)
					for c := range 3 {
						out.Pix[o+c] = uint8(v[c]*a + float64(out.Pix[o+c])*(1-a))
					}
				}
			}
		}
		// The text, as a dim block a letter (and a brighter one for what
		// is left of the lines' own characters: there should be none).
		for y, row := range strings.Split(screen, "\n") {
			for x, r := range cellGrid(ansi.Strip(row)) {
				if r == ' ' || r == 0 {
					continue
				}
				c := color.RGBA{120, 120, 120, 255}
				if strings.ContainsRune("─│╭╮╰╯━╸▸", r) {
					c = color.RGBA{255, 60, 60, 255}
				}
				for py := y*chh + 5; py < y*chh+chh-4; py++ {
					for px := x*cw + 2; px < x*cw+cw-1; px++ {
						if px < m.width*cw && py < m.height*chh {
							out.Set(px, py, c)
						}
					}
				}
			}
		}
		f, err := os.Create(dir + "/screen-" + name + ".png") //nolint:gosec // a directory the tester asked for
		if err != nil {
			t.Fatal(err)
		}
		_ = png.Encode(f, out)
		_ = f.Close()
	}
}

// The mark of a chosen row (a list's │, or ▸) becomes a picture and its
// cell is emptied; a frame's sides and a row of several │ are not marks.
func TestLinesPlaceMarks(t *testing.T) {
	m := coverModel(t, 100, 30)
	m.lines.mode = gfx.Shm
	screen := func(rows ...string) ([]string, [][]rune) {
		g := make([][]rune, len(rows))
		for i, r := range rows {
			g[i] = cellGrid(r)
		}
		return rows, g
	}
	rows, g := screen(
		"╭──────────╮",
		"│ one      │",
		"│ │ two    │",
		"│ │ more   │",
		"│ three    │",
		"╰──────────╯",
	)
	cover := false
	m.linesPlaceBoxes(rows, g, time.Now())
	if !m.linesPlaceMarks(rows, g, &cover) {
		t.Fatal("no mark found")
	}
	st := m.lines.strips[stripSel]
	if !st.seen || st.col != 2 {
		t.Fatalf("the mark's column: %+v", st)
	}
	chh := float64(m.lines.cellH)
	if m.lines.selTop < 2*chh || m.lines.selTop > 2.5*chh || m.lines.selBot < 3.5*chh || m.lines.selBot > 4*chh {
		t.Errorf("the mark spans rows 2 and 3: %.0f..%.0f px (a cell is %.0f)", m.lines.selTop, m.lines.selBot, chh)
	}
	if strings.ContainsRune(rows[2][3:], '│') && []rune(rows[2])[2] == '│' {
		t.Errorf("the mark's cell should be empty: %q", rows[2])
	}
	rows, g = screen("  │  │  │  ", "  ▸ chosen")
	m.linesPlaceMarks(rows, g, &cover)
	if st := m.lines.strips[stripSel]; st.col != 2 || m.lines.selTop < chh {
		t.Errorf("▸ marks its row, a row of bars does not: col %d, top %.0f", st.col, m.lines.selTop)
	}
	buf := make([]byte, 9*200*4)
	drawSel(buf, 9, 200, 40, 70, 18, 1, vcol{1, 0.4, 0.5})
	if buf[(55*9+4)*4+3] < 200 || buf[(20*9+4)*4+3] != 0 || buf[(100*9+4)*4+3] != 0 {
		t.Error("the mark is a stroke from a to b and nothing else")
	}
}

// How much a second of the interface's lines costs, in the cover view with
// music playing.
func BenchmarkLinesSecond(b *testing.B) {
	fakeTap(&testing.T{})
	m := coverModel(&testing.T{}, 150, 44)
	m.coverView = true
	m.lines.mode = gfx.Shm
	m.lines.cellW, m.lines.cellH = 9, 18
	now := time.Now()
	for b.Loop() {
		for range 30 {
			m.View()
			now = now.Add(33 * time.Millisecond)
			m.linesDraw(0.033, now)
		}
	}
}

// The light behind the cover: all round it, under its round corners too,
// and gone before the picture's edges.
func TestHalo(t *testing.T) {
	const w, h, mx, my = 300, 260, 60, 50
	var s haloShape
	buf := make([]byte, w*h*4)
	pal := [3]vcol{{1, 0.4, 0.5}, {0.4, 0.7, 1}, {1, 0.8, 0.4}}
	s.draw(buf, w, h, mx, my, 8, 3, 1, 1, pal)
	at := func(x, y int) int { return int(buf[(y*w+x)*4+3]) }
	if at(mx-3, h/2) < 20 || at(w/2, my-3) < 20 {
		t.Error("no light beside the cover")
	}
	if at(mx+1, my+1) < 20 {
		t.Error("the light should lie under the cover's round corner")
	}
	for x := range w {
		if at(x, 0) > 2 || at(x, h-1) > 2 {
			t.Fatal("the light reaches the picture's edge")
		}
	}
	if at(w/2, h/2) != 0 {
		t.Error("nothing is drawn well under the cover")
	}
}

// A touch bends the lines and lets them go: a press pulls a frame's sides
// in, a move through a list sloshes it that way, and it is all over in
// under a second.
func TestLinesTouch(t *testing.T) {
	l := newUILines()
	now := time.Now()
	if l.frameBend(now, true) != nil || l.touching(now) {
		t.Fatal("nothing bends with no touch")
	}
	l.touches = []lineTouch{{kind: touchPress, dir: 0, at: now.Add(-100 * time.Millisecond)}}
	bend := l.frameBend(now, true)
	if bend == nil {
		t.Fatal("a press should bend the frame")
	}
	for side := range 4 {
		if v := bend(side, 0.5, 0.1); v > -0.2 {
			t.Errorf("side %d: a press pulls the line in, got %.2f", side, v)
		}
		if v := bend(side, 0, 0); math.Abs(v) > 1e-9 {
			t.Errorf("side %d: the corners hold, got %.2f", side, v)
		}
	}
	if l.frameBend(now, false) != nil {
		t.Error("a frame the hand is not on stays as it is")
	}
	l.touches = []lineTouch{{kind: touchScroll, dir: 1, at: now.Add(-100 * time.Millisecond)}}
	bend = l.frameBend(now, true)
	if top, bottom := bend(sideTop, 0.5, 0), bend(sideBottom, 0.5, 0); top >= 0 || bottom <= 0 {
		t.Errorf("moving down sloshes the frame down: top %.2f, bottom %.2f (outwards)", top, bottom)
	}
	if l.ruleLift(now) <= 0 {
		t.Error("and the rules with it")
	}
	l.touches = []lineTouch{{kind: touchPlay, dir: 0, at: now.Add(-100 * time.Millisecond)}}
	if w := l.progressWaves(now, 0.5); len(w) != 2 || w[0].at >= 0.5 || w[1].at <= 0.5 {
		t.Errorf("a pause ripples out of the song's point both ways: %+v", w)
	}
	later := now.Add(time.Second)
	if l.touching(later) || l.frameBend(later, true) != nil || l.ruleLift(later) != 0 {
		t.Error("a second later nothing is left of a touch")
	}
	for age := 0.0; age < 1; age += 0.01 {
		if v := spring(age); math.Abs(v) > 1.01 {
			t.Fatalf("the spring overshoots: %.2f at %.2f s", v, age)
		}
	}
	if spring(0) != 0 {
		t.Error("the spring starts from rest")
	}

	// The keys.
	m := coverModel(t, 100, 30)
	m.lines.mode = gfx.Shm
	for key, kind := range map[string]int{"enter": touchPress, "esc": touchBack, "j": touchScroll, "tab": touchSide, "n": touchSong, "space": touchPlay} {
		m.lines.touches = nil
		m.linesTouchKey(key)
		if len(m.lines.touches) == 0 || m.lines.touches[0].kind != kind {
			t.Errorf("%s: touches %+v, want kind %d", key, m.lines.touches, kind)
		}
	}
	m.lines.touches = nil
	m.mode = modeSearch
	m.linesTouchKey("n")
	if len(m.lines.touches) != 1 || m.lines.touches[0].kind != touchType {
		t.Errorf("a letter typed into the search is typing: %+v", m.lines.touches)
	}
}

// Each row of the screen is made to differ from its neighbours, invisibly,
// so that the terminal is never told to scroll (which moves the pictures).
func TestLinesRowMark(t *testing.T) {
	row := "\x1b[31mwords\x1b[0m      "
	seen := map[string]bool{}
	for y := range 27 {
		got := linesRowMark(row, y)
		if ansi.StringWidth(got) != ansi.StringWidth(row) {
			t.Fatalf("row %d changed its width", y)
		}
		if !strings.HasPrefix(ansi.Strip(got), "words") {
			t.Fatalf("row %d lost its words: %q", y, got)
		}
		for _, r := range ansi.Strip(got)[5:] {
			if r != ' ' && r != '\u00a0' && r != '\u2800' {
				t.Fatalf("row %d shows something: %q", y, got)
			}
		}
		seen[ansi.Strip(got)] = true
	}
	if len(seen) != 27 {
		t.Errorf("%d different rows of 27", len(seen))
	}
	if full := "words to the very end"; linesRowMark(full, 3) != full {
		t.Error("a row with no blank end is left alone")
	}
	m := coverModel(t, 120, 36)
	m.coverView = true
	m.lines.mode = gfx.Shm
	rows := strings.Split(ansi.Strip(m.View().Content), "\n")
	for y := 1; y < len(rows); y++ {
		if rows[y] == rows[y-1] {
			t.Fatalf("rows %d and %d are the same: the terminal could scroll one into the other", y-1, y)
		}
	}
}

// The player's buttons as a picture: four of them, the ones that are on in
// colour, and their row of cells left empty for them.
func TestLinesControls(t *testing.T) {
	const w, h, cell = 216, 54, 18.0
	pal := [3]vcol{{1, 0.3, 0.2}, {0.4, 0.7, 1}, {1, 0.8, 0.4}}
	draw := func(s ctlState) []byte {
		buf := make([]byte, w*h*4)
		drawControls(buf, w, h, cell, ctlLook{ctlState: s, pal: pal, fg: vcol{0.8, 0.8, 0.8}})
		return buf
	}
	lit := func(buf []byte, n int) (pixels int, red bool) {
		for y := range h {
			for x := n * w / 4; x < (n+1)*w/4; x++ {
				i := (y*w + x) * 4
				if buf[i+3] > 120 {
					pixels++
					red = red || int(buf[i]) > int(buf[i+2])+60
				}
			}
		}
		return pixels, red
	}
	off, on := draw(ctlState{}), draw(ctlState{repeat: 1, shuffle: true, playing: true, fav: true})
	for n := range 4 {
		if p, red := lit(off, n); p < 15 || (red && n != 2) {
			t.Errorf("button %d off: %d pixels, coloured %v", n, p, red)
		}
		if p, red := lit(on, n); p < 15 || !red {
			t.Errorf("button %d on: %d pixels, coloured %v", n, p, red)
		}
	}
	if a, _ := lit(off, 3); true {
		if b, _ := lit(on, 3); b <= a {
			t.Errorf("a favourite's heart is filled: %d pixels against %d", b, a)
		}
	}
	for x := range w {
		if off[x*4+3] != 0 || off[((h-1)*w+x)*4+3] != 0 {
			t.Fatal("the buttons reach the picture's edge")
		}
	}

	m := coverModel(t, 140, 44)
	m.coverView = true
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "⇄") {
		t.Skip("no buttons in this layout")
	}
	m.lines.mode = gfx.Shm
	out := ansi.Strip(m.View().Content)
	if strings.ContainsAny(out, "⇄↺♡"+string(linesCtlMark)) {
		t.Error("the buttons' characters should be gone: they are a picture")
	}
	if st := m.lines.strips[stripCtl]; !st.seen || st.cols != linesCtlCols || st.rows != linesCtlRows {
		t.Errorf("the buttons' place: %+v", st)
	}
	// A button that changes springs.
	now := time.Now()
	m.lines.ctlPops(ctlState{}, now)
	if _, live := m.lines.ctlPops(ctlState{fav: true}, now.Add(50*time.Millisecond)); !live {
		t.Error("a change should set its button springing")
	}
}

// The cover view's lyrics as a picture: words are drawn, the line being
// sung is the brightest, and a line change glides instead of jumping.
func TestLyricsPic(t *testing.T) {
	var p lyricsPic
	if !p.ready(18, 1) {
		t.Skip("no font to draw with here")
	}
	const w, h = 500, 400
	buf := make([]byte, w*h*4)
	sc := views.CoverScene{Synced: true, Current: 1, Focus: 1, Cursor: -1, Lit: -1, Lines: []views.CoverSceneLine{
		{Text: "first line of the song"}, {Text: "the line being sung now, long enough to wrap onto a second row"}, {Text: "the next one"},
	}}
	look := lyricLook{fg: vcol{0.8, 0.8, 0.8}, sung: vcol{1, 1, 1}, pick: vcol{1, 0.5, 0.5}}
	for range 80 {
		p.draw(buf, w, h, sc, 0.033, look)
	}
	bright := func(y0, y1 int) (most int) {
		for y := max(0, y0); y < min(h, y1); y++ {
			for x := range w {
				most = max(most, int(buf[(y*w+x)*4+3]))
			}
		}
		return most
	}
	cur := int(lyricAnchor * h)
	if b := bright(cur, cur+p.font.Height); b < 200 {
		t.Errorf("the line being sung should be solid: alpha up to %d", b)
	}
	if b := bright(cur-p.font.Height*3/2, cur-p.font.Height/2-2); b == 0 || b > 150 {
		t.Errorf("the line before it is there, dimmer: alpha up to %d", b)
	}
	if b := bright(0, 2); b != 0 {
		t.Errorf("the words melt away at the top edge: alpha %d", b)
	}
	if len(p.block(sc.Lines[1], w).rows) < 2 {
		t.Error("a long line wraps")
	}
	// The next line: the text is on its way, not there at once.
	sc.Current, sc.Focus = 2, 2
	was := p.scroll
	if moving := p.draw(buf, w, h, sc, 0.033, look); !moving || p.scroll == was || p.scroll >= float64(p.tops[2])-lyricAnchor*h {
		t.Errorf("a line change glides: moving %v, scroll %.1f from %.1f", moving, p.scroll, was)
	}
	// Loading: a word where the lyrics would be.
	p.draw(buf, w, h, views.CoverScene{Message: "fetching lyrics…", Current: -1, Cursor: -1}, 0.033, look)
	if bright(cur, cur+p.small.Height) == 0 {
		t.Error("the message should be drawn")
	}
}

// The player's words as a picture: their cells are left empty, the progress
// line is still found between its two times, and the simple style keeps
// the characters.
func TestLinesLabels(t *testing.T) {
	m := coverModel(t, 140, 44)
	m.coverView = true
	m.lines.mode = gfx.Shm
	m.lines.cellW, m.lines.cellH = 9, 18
	if !m.lyricsPicOn() {
		t.Skip("no font to draw with here")
	}
	out := ansi.Strip(m.View().Content)
	for _, word := range []string{"3:00", "♪"} { // the title is in the queue too
		if strings.Contains(out, word) {
			t.Errorf("%q should be drawn, not written in cells", word)
		}
	}
	for _, r := range out {
		if r >= labelMark0 && r <= labelMarkEnd {
			t.Fatal("a label's mark was left on the screen")
		}
	}
	l := m.lines
	if !l.strips[stripLabels].seen || !l.strips[stripProgress].seen || !l.strips[stripVolume].seen {
		t.Fatalf("the labels, the progress line and the volume line should all be found")
	}
	kinds := map[int]bool{}
	for _, lb := range l.labels {
		if lb.found {
			kinds[lb.kind] = true
		}
	}
	for _, k := range []int{labelTitle, labelArtist, labelAlbum, labelTime, labelSmall, labelSpeaker} {
		if !kinds[k] {
			t.Errorf("label kind %d not on the screen", k)
		}
	}
	// A track reveal must leave every picture label available to the same
	// crossfade, including its first frame.
	m.fxState().trackAt = time.Now()
	_ = m.View()
	kinds = map[int]bool{}
	for _, lb := range l.labels {
		if lb.found && isSongWord(lb) {
			kinds[lb.kind] = true
		}
	}
	for _, k := range []int{labelTitle, labelArtist, labelAlbum} {
		if !kinds[k] {
			t.Errorf("song label kind %d missing during track reveal", k)
		}
	}
	for _, i := range []int{stripProgress, stripVolume, stripCtl} {
		if !l.strips[i].seen {
			t.Errorf("player picture %d missing during track reveal", i)
		}
	}
	m.linesDraw(0.033, time.Now())
	st := l.strips[stripLabels]
	solid := 0
	for i := 3; i < len(st.rgba); i += 4 {
		if st.rgba[i] > 40 {
			solid++
		}
	}
	if solid < 300 {
		t.Errorf("the words should be drawn: %d solid pixels", solid)
	}
	m.cfg.UIStyle = config.StyleSimple
	if out := ansi.Strip(m.View().Content); !strings.Contains(out, "Night Song") || !strings.Contains(out, "♪") {
		t.Error("the simple style writes the words in cells")
	}
}

// A held key repeats many times a second: its answer runs on instead of
// being begun again with every repeat, and the progress line has room at
// its ends for its light.
func TestLinesHeldKey(t *testing.T) {
	l := newUILines()
	l.touch(touchScroll, 1)
	first := l.touches[0].at
	for range 10 {
		l.touch(touchScroll, 1) // the repeats
	}
	if len(l.touches) != 1 || !l.touches[0].at.Equal(first) {
		t.Fatalf("repeats begin the answer again: %+v", l.touches)
	}
	l.touch(touchScroll, -1) // the other way is another touch
	if len(l.touches) != 2 {
		t.Errorf("a touch the other way should count: %+v", l.touches)
	}
	if l.touches[0].heldAt.IsZero() {
		t.Error("a repeated touch is a held key")
	}
	// Held, it pushes steadily; let go, it comes back to rest.
	held := l.touches[0]
	held.at, held.heldAt = first.Add(-2*time.Second), first.Add(-time.Second)
	held.last = first
	if v := held.level(first.Add(50 * time.Millisecond)); v < 0.3 {
		t.Errorf("a held key keeps pushing: %.2f", v)
	}
	if v := held.level(first.Add(1200 * time.Millisecond)); math.Abs(v) > 0.02 || held.alive(first.Add(1200*time.Millisecond)) {
		t.Errorf("let go, it is over within a second: %.2f", v)
	}
	now := time.Now()
	l.ctlTouch(0, now)
	l.ctlTouch(0, now.Add(50*time.Millisecond))
	if !l.ctlAt[0].Equal(now) {
		t.Error("a button flipped again at once keeps its spring going")
	}

	// The point where the song is, at the very start: whole, not cut in
	// half by the picture's edge.
	const bw, pad, h, cell = 300, 27, 54, 18.0
	w := bw + 2*pad
	buf := make([]byte, w*h*4)
	pal := [3]vcol{{1, 0.4, 0.5}, {0.4, 0.7, 1}, {1, 0.8, 0.4}}
	drawProgressPad(buf, w, h, pad, 0, 1, 3, cell, pal, vcol{0.8, 0.8, 0.8})
	mid := h / 2
	if a := buf[(mid*w+pad-2)*4+3]; a < 60 {
		t.Errorf("the point should reach left of the line's start: alpha %d", a)
	}
	if a := buf[(mid*w)*4+3]; a != 0 {
		t.Errorf("and be gone by the picture's edge: alpha %d", a)
	}
	if a := buf[(mid*w+pad+bw+5)*4+3]; a != 0 {
		t.Errorf("nothing is drawn past the line's end: alpha %d", a)
	}
}

// In the boxed view's player bar the volume line is above the progress
// line, and with the words drawn both start with a label's mark: each is
// still told for what it is, not by which comes first.
func TestLinesBarVolumeAndProgress(t *testing.T) {
	m := coverModel(t, 160, 44)
	m.coverView = false
	m.lines.mode = gfx.Shm
	m.lines.cellW, m.lines.cellH = 9, 18
	if !m.lyricsPicOn() {
		t.Skip("no font to draw with here")
	}
	m.playerState.Volume = 1
	m.playerState.Position = 9 * time.Second
	_ = m.View()
	l := m.lines
	vol, prog := l.strips[stripVolume], l.strips[stripProgress]
	if !vol.seen || !prog.seen {
		t.Fatalf("both lines should be found: volume %v, progress %v", vol.seen, prog.seen)
	}
	if vol.row >= prog.row {
		t.Fatalf("the bar should have the volume (row %d) above the progress (row %d)", vol.row, prog.row)
	}
	if vol.want != 1 || prog.want > 0.1 {
		t.Errorf("volume line at %.2f (want 1), progress line at %.2f (want 0.05)", vol.want, prog.want)
	}
}

// The fill of a line goes where it should at once while it only moves on,
// and on its way there when it jumps.
func TestLinesEaseFill(t *testing.T) {
	st := &lineStrip{want: 0.3}
	if st.easeFill(1.0/60) || st.frac != 0.3 {
		t.Fatal("a line just come is filled as it should be")
	}
	st.shown = true
	st.want = 0.3005
	if st.easeFill(1.0/60) || st.frac != 0.3005 {
		t.Error("a song playing on: at once")
	}
	st.want = 0.8 // a seek
	if !st.easeFill(1.0/60) || st.frac <= 0.3005 || st.frac >= 0.8 {
		t.Errorf("a jump: on its way, at %.3f", st.frac)
	}
	for range 30 { // half a second
		st.easeFill(1.0 / 60)
	}
	if st.frac != 0.8 {
		t.Errorf("there within half a second: %.3f", st.frac)
	}
}

// Each part of the pixel interface goes to characters on its own, and the
// others stay pictures.
func TestLinesParts(t *testing.T) {
	look := func(set func(c *config.Config)) (*Model, string) {
		m := coverModel(t, 160, 44)
		m.coverView = false
		m.lines.mode = gfx.Shm
		m.lines.cellW, m.lines.cellH = 9, 18
		if set != nil {
			set(m.cfg)
		}
		return m, ansi.Strip(m.View().Content)
	}
	m, out := look(nil)
	if !m.lyricsPicOn() {
		t.Skip("no font to draw with here")
	}
	l := m.lines
	if !l.strips[stripCover].seen || !l.strips[stripCtl].seen || !l.strips[stripLabels].seen || len(l.boxes) == 0 {
		t.Fatalf("all pictures: cover %v, buttons %v, words %v, frames %d", l.strips[stripCover].seen, l.strips[stripCtl].seen, l.strips[stripLabels].seen, len(l.boxes))
	}
	if strings.ContainsAny(out, "╭⏸") { // (▶ marks the song in the queue)
		t.Error("all pictures: frame and button characters left in the cells")
	}

	m, out = look(func(c *config.Config) { c.UILines = config.PartCells })
	if len(m.lines.boxes) > 0 || !strings.Contains(out, "╭") {
		t.Error("lines as characters: the frames should stay ╭─╮")
	}
	if !m.lines.strips[stripCover].seen || !m.lines.strips[stripCtl].seen {
		t.Error("lines as characters: the cover and buttons stay pictures")
	}

	m, out = look(func(c *config.Config) { c.UIButtons = config.PartCells })
	if m.lines.strips[stripCtl].seen || !strings.Contains(out, "⏸") {
		t.Error("buttons as characters: ⏸ in the cells, no picture")
	}

	m, _ = look(func(c *config.Config) { c.UICover = config.PartCells })
	if m.lines.strips[stripCover].seen || m.lines.strips[stripHalo].seen {
		t.Error("cover as characters: no cover picture, no light")
	}

	m, _ = look(func(c *config.Config) { c.UIText = config.PartCells })
	if m.lyricsPicOn() || m.lines.strips[stripLabels].seen {
		t.Error("text in the terminal's letters: no drawn words")
	}

	m, _ = look(func(c *config.Config) { c.UILight = config.LightOff })
	if !m.lines.strips[stripCover].seen || m.lines.strips[stripHalo].seen {
		t.Error("no light: the cover without its light")
	}
}

func TestCoverFadesBackAfterHiddenLayout(t *testing.T) {
	m := coverModel(t, 90, 30)
	m.lines.mode = gfx.Shm
	m.lines.cellW, m.lines.cellH = 9, 18
	m.coverView = true
	_ = m.View()
	l := m.lines
	st := &l.strips[stripCover]
	if !st.seen {
		t.Fatal("cover view should show the cover")
	}
	start := time.Now()
	m.linesDraw(0.016, start)
	m.linesDraw(0.016, start.Add(time.Second))
	m.coverView = false
	_ = m.View()
	if st.seen {
		t.Fatal("boxed view should hide the cover at this size")
	}
	m.linesDraw(0.016, start.Add(2*time.Second))
	m.linesDraw(0.016, start.Add(3*time.Second))
	m.coverView = true
	_ = m.View()
	if !st.seen {
		t.Fatal("cover should return in cover view")
	}
	returned := start.Add(4 * time.Second)
	m.linesDraw(0.016, returned)
	if !l.coverAt.Equal(returned) {
		t.Fatalf("cover fade started at %v, want %v", l.coverAt, returned)
	}
	if len(l.coverFade) == 0 {
		t.Fatal("returning cover should start transparent")
	}
	m.linesDraw(0.016, returned.Add(250*time.Millisecond))
	if len(l.coverFade) == 0 {
		t.Fatal("returning cover should still be fading halfway through")
	}
	m.linesDraw(0.016, returned.Add(600*time.Millisecond))
	if len(l.coverFade) != 0 {
		t.Fatal("cover should finish fading")
	}
}

func TestTitleScrollPausesAndReturnsSmoothly(t *testing.T) {
	at := time.Now()
	for _, tc := range []struct {
		elapsed time.Duration
		want    int
	}{
		{0, 0},
		{1700 * time.Millisecond, 20},
		{2200 * time.Millisecond, 40},
		{3400 * time.Millisecond, 40},
		{3900 * time.Millisecond, 20},
		{4400 * time.Millisecond, 0},
	} {
		if got := scrollOffset(40, 40, at, at.Add(tc.elapsed)); got != tc.want {
			t.Errorf("at %v: offset %d, want %d", tc.elapsed, got, tc.want)
		}
	}
}

// Still lines do not sway with the music nor bend to keys (the colours of
// the frame in focus still run round it).
func TestLinesCalm(t *testing.T) {
	m := coverModel(t, 160, 44)
	m.coverView = false
	m.lines.mode = gfx.Shm
	m.lines.cellW, m.lines.cellH = 9, 18
	m.cfg.UILines = config.LinesCalm
	_ = m.View()
	for i := range m.lines.strips {
		m.lines.strips[i].born = time.Now().Add(-time.Minute) // drawn in long ago
	}
	for range 30 {
		m.linesDraw(0.033, time.Now())
	}
	m.lines.amp, m.lines.bass, m.lines.kick = 1, 1, 1
	m.linesTouchKey("down")
	var first [][]byte
	for n := range 3 {
		m.lines.amp, m.lines.bass, m.lines.kick = 1, 1, 1
		m.linesDraw(0.033, time.Now())
		for i := stripFrame; i < stripCount; i++ {
			st := m.lines.strips[i]
			if !st.seen {
				continue
			}
			shape := make([]byte, 0, len(st.rgba)/4)
			for j := 3; j < len(st.rgba); j += 4 {
				shape = append(shape, st.rgba[j])
			}
			if n == 0 {
				first = append(first, shape)
				continue
			}
			if string(first[0]) != string(shape) {
				t.Fatal("a still frame moved")
			}
			break
		}
	}
	if len(first) == 0 {
		t.Fatal("still lines are pictures too")
	}
}

// Another song's words melt in as the last one's melt away; the times and
// the volume stay.
func TestLinesLabelsCrossfade(t *testing.T) {
	m := coverModel(t, 140, 44)
	m.coverView = true
	m.lines.mode = gfx.Shm
	m.lines.cellW, m.lines.cellH = 9, 18
	if !m.lyricsPicOn() {
		t.Skip("no font to draw with here")
	}
	l := m.lines
	_ = m.View()
	l.labelsAt = time.Now().Add(-time.Minute)
	m.linesDraw(0.016, time.Now())
	m.linesDraw(0.016, time.Now())
	l.labelsAt = time.Now().Add(-time.Minute) // the first song's words came long ago
	tr := *m.playerState.Track
	tr.ID, tr.Title, tr.Artist = "2", "Another Tune", "Someone"
	m.playerState.Track = &tr
	_ = m.View()
	m.linesDraw(0.016, time.Now())
	found := false
	for _, lb := range l.goneWords {
		found = found || lb.text == "Night Song"
	}
	if !found {
		t.Fatalf("the last song's title should be going: %+v", l.goneWords)
	}
	tr.Title = "Third Tune"
	m.playerState.Track = &tr
	_ = m.View()
	m.linesDraw(0.016, time.Now())
	found = false
	for _, lb := range l.goneWords {
		found = found || lb.text == "Another Tune"
	}
	if l.titleOf != "Third Tune" || !found {
		t.Fatalf("a rapid change should start a new transition: title %q, old labels %+v", l.titleOf, l.goneWords)
	}
	solid := func(look labelLook) int {
		st := l.strips[stripLabels]
		buf := make([]byte, len(st.rgba))
		l.drawLabels(buf, st.pw, st.ph, float64(st.col*l.cellW), float64(st.row*l.cellH), m.hostRows, look)
		n := 0
		for i := 3; i < len(buf); i += 4 {
			if buf[i] > 40 {
				n++
			}
		}
		return n
	}
	base := labelLook{fg: vcol{0.9, 0.9, 0.9}, accent: vcol{1, 0.5, 0.5}, volume: 0.5}
	none, stay := base, base
	stay.gone = 1
	both := stay
	both.fade = 1
	if n0, n1, n2 := solid(none), solid(stay), solid(both); n0 == 0 || n1 <= n0 || n2 <= n1 {
		t.Errorf("times and volume always (%d), the old words while they go (%d), and the new ones too (%d)", n0, n1, n2)
	}
	// Some players supply the artist after the title. It must also start
	// the same transition, even when the title did not change.
	tr.Artist = "Another Artist"
	m.playerState.Track = &tr
	_ = m.View()
	m.linesDraw(0.016, time.Now())
	found = false
	for _, lb := range l.goneWords {
		found = found || lb.kind == labelArtist && lb.text == "Someone"
	}
	if !found {
		t.Fatalf("a late artist update should crossfade the previous artist: %+v", l.goneWords)
	}
}

// Another song's lyrics (or a word in their place) melt in as the last
// ones melt away.
func TestLyricsPicFade(t *testing.T) {
	var p lyricsPic
	if !p.ready(18, 1) {
		t.Skip("no font to draw with here")
	}
	const w, h = 500, 400
	buf := make([]byte, w*h*4)
	look := lyricLook{fg: vcol{0.8, 0.8, 0.8}, sung: vcol{1, 1, 1}, pick: vcol{1, 0.5, 0.5}}
	song := func(words string) views.CoverScene {
		return views.CoverScene{Synced: true, Current: 0, Focus: 0, Cursor: -1, Lit: -1, Lines: []views.CoverSceneLine{{Text: words}, {Text: words + " again"}}}
	}
	solid := func() (n int) {
		for i := 3; i < len(buf); i += 4 {
			if buf[i] > 30 {
				n++
			}
		}
		return n
	}
	for range 30 {
		p.draw(buf, w, h, song("one song's words"), 0.016, look)
	}
	alone := solid()
	wait := views.CoverScene{Message: "fetching lyrics…"}
	if !p.draw(buf, w, h, wait, 0.016, look) {
		t.Error("melting: it moves")
	}
	if solid() < alone/2 {
		t.Errorf("a frame after the change the last words are mostly still there: %d of %d", solid(), alone)
	}
	for range 30 {
		p.draw(buf, w, h, wait, 0.016, look)
	}
	if n := solid(); n >= alone/2 {
		t.Errorf("half a second later only the word is left: %d", n)
	}
	if p.draw(buf, w, h, wait, 0.016, look) {
		t.Error("melted: nothing moves")
	}
}

// A frame that changes its size goes over from the old one to the new,
// placed by the pixel on the way, instead of drawing itself in anew.
func TestLinesFrameMorph(t *testing.T) {
	m := coverModel(t, 140, 40)
	m.coverView = false
	m.lines.mode = gfx.Shm
	m.lines.cellW, m.lines.cellH = 9, 18
	m.cfg.UIMotion = config.MotionCalm
	m.noAnim = false
	_ = m.View()
	l := m.lines
	if len(l.boxGeo) == 0 {
		t.Fatal("no frames on the screen")
	}
	low := func() [4]int { // the lowest frame
		var k [4]int
		for key := range l.boxGeo {
			if key[3] > k[3] {
				k = key
			}
		}
		return k
	}
	before := low()
	slotsBefore := maps.Clone(l.boxSlot)
	m.height -= 4
	_ = m.View()
	after := low()
	if after == before {
		t.Fatal("the frame should have changed its size")
	}
	// Its sides are drawn in the strips they were drawn in, and the other
	// frames' in theirs.
	if l.boxSlot[after] != slotsBefore[before] {
		t.Errorf("the frame went over to strips %d from %d", l.boxSlot[after], slotsBefore[before])
	}
	for key, s := range l.boxSlot {
		if was, ok := slotsBefore[key]; ok && was != s {
			t.Errorf("the frame %v that stayed went over to strips %d from %d", key, s, was)
		}
	}
	geo := l.boxGeo[after]
	if geo.from == geo.to || !geo.moving(time.Now()) {
		t.Fatalf("it should go over from where it was: %+v", geo)
	}
	if born, ok := l.boxes[after]; !ok || time.Since(born) < time.Second && !born.Before(geo.start) {
		t.Error("a frame that only moves is not drawn in anew")
	}
	// A quarter of the way:
	// the side picture is between the two heights.
	geo.start = time.Now().Add(-time.Duration(linesMorphFor * 0.25 * float64(time.Second)))
	l.boxGeo[after] = geo
	for i := stripFrame; i < stripCount; i++ {
		if st := &l.strips[i]; st.seen && st.morph.to == geo.to {
			st.morph = geo
		}
	}
	m.linesDraw(0.016, time.Now())
	found := false
	for i := stripFrame; i < stripCount; i++ {
		st := l.strips[i]
		if !st.seen || st.morph.to != geo.to || st.side != sideLeft {
			continue
		}
		found = true
		hOld, hNew := float64(before[3]-before[1]-1)*18, float64(after[3]-after[1]-1)*18
		if !st.img.Native || float64(st.ph) >= hOld || float64(st.ph) <= hNew {
			t.Errorf("on its way the side is %d pixels high (native %v), between %.0f and %.0f", st.ph, st.img.Native, hNew, hOld)
		}
	}
	if !found {
		t.Fatal("no side of the frame on its way")
	}
	// A moving side must redraw even when it is still within the same cell
	// and the playback is quiet (its pixel geometry and colour still change).
	var moving *lineStrip
	for i := stripFrame; i < stripCount; i++ {
		if st := &l.strips[i]; st.seen && st.morph.to == geo.to && st.side == sideLeft {
			moving = st
			break
		}
	}
	previous := moving.sentAt
	m.linesDraw(0.001, previous.Add(time.Millisecond))
	if !moving.sentAt.After(previous) {
		t.Fatal("moving frame was skipped by the still-frame cache")
	}
	// Settled: whole cells again.
	geo.start = time.Now().Add(-time.Second)
	for i := stripFrame; i < stripCount; i++ {
		if st := &l.strips[i]; st.seen && st.morph.to == geo.to {
			st.morph = geo
		}
	}
	// Finish without another layout: the last animated cell position can
	// be different from the destination and must not survive the morph.
	for i := stripFrame; i < stripCount; i++ {
		if st := &l.strips[i]; st.seen && st.morph.to == geo.to {
			st.col, st.row = 0, 0
		}
	}
	m.linesDraw(0.016, time.Now())
	if moving.col != int(geo.to[0])-moving.pad || moving.row != int(geo.to[1])+1 {
		t.Fatalf("frame did not reach destination: %d,%d", moving.col, moving.row)
	}
	for i := stripFrame; i < stripCount; i++ {
		// (Its pictures are placed by the pixel all the same: they are
		// sent without their clear rows and columns.)
		if st := l.strips[i]; st.seen && st.morph.to == geo.to && (st.morph.moving(time.Now()) || st.pw != st.cols*9 || st.ph != st.rows*18) {
			t.Errorf("a frame at its place is drawn over its own cells: %dx%d for %dx%d cells", st.pw, st.ph, st.cols, st.rows)
		}
	}
}

// The lyrics a line a picture look the same as the whole picture, pixel
// for pixel, and a glide only moves the lines clear of the edges' melting:
// nothing of theirs is sent again.
func TestLyricLines(t *testing.T) {
	var p lyricsPic
	if !p.ready(18, 1) {
		t.Skip("no font to draw with here")
	}
	const w, h = 500, 400
	buf := make([]byte, w*h*4)
	var lines []views.CoverSceneLine
	for i := range 14 {
		lines = append(lines, views.CoverSceneLine{Text: fmt.Sprintf("line %d of the song, a few words", i)})
	}
	sc := views.CoverScene{Synced: true, Current: 4, Focus: 4, Cursor: -1, Lit: -1, Lines: lines, Status: "lrclib"}
	look := lyricLook{fg: vcol{0.8, 0.8, 0.8}, sung: vcol{1, 1, 1}, pick: vcol{1, 0.5, 0.5}}
	for range 60 {
		p.frame(buf, w, h, sc, 0.033, look, false)
	}
	items, whole, _ := p.frame(buf, w, h, sc, 0.0001, look, true)
	if whole || len(items) == 0 {
		t.Fatalf("settled, the lines come one by one: whole %v, %d items", whole, len(items))
	}
	// The lines put together, each cut as it is sent.
	got := make([]byte, w*h*4)
	for i := range items {
		it := &items[i]
		y0, y1, iw := it.crop(w, h)
		if y1 <= y0 {
			continue
		}
		part := make([]byte, iw*(y1-y0)*4)
		it.paint(part, iw, y1-y0, it.y-y0, y0, h)
		for y := y0; y < y1; y++ {
			for x := range iw {
				src, dst := ((y-y0)*iw+x)*4, (y*w+x)*4
				if part[src+3] != 0 {
					copy(got[dst:dst+4], part[src:src+4])
				}
			}
		}
	}
	want := p.paintAll(nil, w, h, items)
	diff := 0
	for i := 0; i+3 < len(got) && i+3 < len(want); i += 4 {
		// (The colour of a pixel nothing of shows doesn't count.)
		if got[i+3] != want[i+3] || want[i+3] != 0 && !bytes.Equal(got[i:i+4], want[i:i+4]) {
			diff++
		}
	}
	if diff != 0 {
		t.Errorf("a line a picture differs from the whole picture in %d pixels", diff)
	}

	// Gliding to the next line: the lines in the middle are only moved.
	l := newUILines()
	l.mode, l.cellW, l.cellH = gfx.Direct, 10, 20
	st := &l.strips[stripLyrics]
	st.col, st.row, st.cols, st.rows = 0, 0, w/10, h/20
	l.lyrItems = items
	first := l.sendLyricLines(st, w, h, time.Now(), false)
	if strings.Count(first, "a=T") == 0 {
		t.Fatal("the lines should be sent at first")
	}
	// The whole glide, as the terminal gets it: against all the lines sent
	// every frame, as the bands were.
	sc.Current, sc.Focus = 5, 5
	total, sent, moved := 0, 0, 0
	const frames = 50
	for f := range frames {
		items, whole, _ = p.frame(buf, w, h, sc, 1.0/72, look, true)
		if whole {
			t.Fatal("a glide is shown a line a picture")
		}
		l.lyrItems = items
		l.frameN = f
		seq := l.sendLyricLines(st, w, h, time.Now(), false)
		total += len(seq)
		sent += strings.Count(seq, "a=T")
		moved += strings.Count(seq, "a=p")
	}
	if moved == 0 || total > frames*len(first)/2 {
		t.Errorf("gliding: %d bytes in %d frames (all lines each frame: %d), %d lines drawn again, %d moved",
			total, frames, frames*len(first), sent, moved)
	}
}

// A travelling cover is shown at the size it is on its way, whatever the
// pixel offset in its first cell: the terminal stretches the source from
// the offset to the far edge of the last cell (measured in kitty: ten
// cells of 9 pixels with X=4 show 86 pixels, not 90).
func TestCoverSpan(t *testing.T) {
	const cell, n = 9, 477
	have := n + n/6 + 2*cell
	worst := 0.0
	for want := 396.0; want <= 477; want += 0.37 {
		for off := range cell {
			cells, src := coverSpan(want, n, have, cell, off)
			if src > have || src <= 0 {
				t.Fatalf("want %.2f off %d: source %d of %d", want, off, src, have)
			}
			shown := float64(cells*cell-off) * n / float64(src)
			worst = math.Max(worst, math.Abs(shown-want))
		}
	}
	if worst > 0.5 {
		t.Errorf("shown up to %.2f pixels off the size it should be", worst)
	}
}

func BenchmarkLyricWordHighlight(b *testing.B) {
	row := lyricRow{w: 1000, h: 50, mask: make([]byte, 1000*50)}
	for i := range row.mask {
		if i%7 != 0 {
			row.mask[i] = byte(50 + i%205)
		}
	}
	item := lyricItem{puts: []lyricPut{{row: &row, c: vcol{0.7, 0.8, 0.9}, c2: vcol{1, 1, 1}, a: 0.5, a2: 1, litEdge: 15}}}
	buf := make([]byte, row.w*row.h*4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item.puts[0].litX = float64(i % row.w)
		item.paint(buf, row.w, row.h, 0, 100, 400)
	}
}

func TestReturningPopupFramePreventsCachedView(t *testing.T) {
	m := coverModel(t, 100, 40)
	m.lines.mode = gfx.File
	m.lines.ticking = true
	m.viewHas = true
	m.viewSec = int(m.smoothPosition().Seconds())
	m.coverView = true
	m.activePanel = -1
	now := time.Now()
	m.lines.boxGeo = map[[4]int]boxMorph{{1, 1, 80, 30}: {from: [4]float64{20, 10, 50, 20}, to: [4]float64{1, 1, 80, 30}, start: now}}
	if m.viewCanReuse() {
		t.Fatal("returning frame must refresh fading border cells")
	}
	m.lines.boxGeo = map[[4]int]boxMorph{{1, 1, 80, 30}: {from: [4]float64{20, 10, 50, 20}, to: [4]float64{1, 1, 80, 30}, start: now.Add(-time.Second)}}
	if m.returningFrame(time.Now()) {
		t.Fatal("settled frame should stop invalidating the view")
	}
}

func TestReturningFrameBorderDoesNotCreateRules(t *testing.T) {
	m := coverModel(t, 80, 24)
	m.coverView = true
	m.lines.mode = gfx.File
	m.lines.cellW, m.lines.cellH = 9, 18
	m.noAnim = false
	m.cfg.UIMotion = config.MotionCalm
	now := time.Now()
	old := [4]int{15, 8, 40, 15}
	m.lines.boxGeo = map[[4]int]boxMorph{old: {from: [4]float64{15, 8, 40, 15}, to: [4]float64{15, 8, 40, 15}}}
	m.lines.boxes = map[[4]int]time.Time{old: now.Add(-time.Second)}
	m.lines.boxSlot = map[[4]int]int{old: 0}
	rows := make([]string, m.height)
	for i := range rows {
		rows[i] = strings.Repeat(" ", m.width)
	}
	rows[2] = "  ╭" + strings.Repeat("─", 65) + "╮" + strings.Repeat(" ", 11)
	for y := 3; y < 20; y++ {
		rows[y] = "  │" + strings.Repeat(" ", 65) + "│" + strings.Repeat(" ", 11)
	}
	rows[20] = "  ╰" + strings.Repeat("─", 65) + "╯" + strings.Repeat(" ", 11)
	result := m.linesPlace(strings.Join(rows, "\n"))
	if !m.returningFrame(time.Now()) {
		t.Fatal("expected a returning popup frame")
	}
	if m.lines.strips[stripRule].seen || len(m.lines.rules) != 0 {
		t.Fatal("fading frame borders were incorrectly turned into separate graphic rules")
	}
	if !strings.Contains(ansi.Strip(result), "╭") {
		t.Fatal("destination frame should remain visible during the transition")
	}
}

func TestGrowingPopupHasOnlyAnimatedBorder(t *testing.T) {
	m := coverModel(t, 80, 24)
	m.lines.mode = gfx.File
	m.lines.cellW, m.lines.cellH = 9, 18
	m.noAnim = false
	m.cfg.UIMotion = config.MotionCalm
	old := [4]int{20, 9, 59, 14}
	from := [4]float64{20, 9, 59, 14}
	m.lines.boxGeo = map[[4]int]boxMorph{old: {from: from, to: from}}
	m.lines.boxes = map[[4]int]time.Time{old: time.Now().Add(-time.Second)}
	m.lines.boxSlot = map[[4]int]int{old: 0}
	m.lines.overlayRect = fxRect{20, 7, 60, 18}
	rows := make([]string, 24)
	for y := range rows {
		rows[y] = strings.Repeat(" ", 80)
	}
	rows[7] = strings.Repeat(" ", 20) + "╭" + strings.Repeat("─", 38) + "╮" + strings.Repeat(" ", 20)
	for y := 8; y < 17; y++ {
		rows[y] = strings.Repeat(" ", 20) + "│" + strings.Repeat(" ", 38) + "│" + strings.Repeat(" ", 20)
	}
	rows[17] = strings.Repeat(" ", 20) + "╰" + strings.Repeat("─", 38) + "╯" + strings.Repeat(" ", 20)
	result := ansi.Strip(m.linesPlace(strings.Join(rows, "\n")))
	geo := m.lines.boxGeo[[4]int{20, 7, 59, 17}]
	if !geo.moving(time.Now()) || geo.from != from {
		t.Fatalf("popup should animate from its previous size: %+v", geo)
	}
	if strings.ContainsAny(result, "╭╮╰╯│─") {
		t.Fatal("terminal border overlaps the animated popup frame")
	}
}

func TestClosingPopupReturnsFrameToMostOverlappingPanel(t *testing.T) {
	m := coverModel(t, 100, 24)
	m.lines.mode = gfx.File
	m.lines.cellW, m.lines.cellH = 9, 18
	m.noAnim = false
	m.cfg.UIMotion = config.MotionCalm
	popup := [4]int{17, 6, 72, 13}
	m.lines.boxGeo = map[[4]int]boxMorph{popup: {from: [4]float64{17, 6, 72, 13}, to: [4]float64{17, 6, 72, 13}}}
	m.lines.boxes = map[[4]int]time.Time{popup: time.Now().Add(-time.Second)}
	m.lines.boxSlot = map[[4]int]int{popup: 0}
	cells := make([][]rune, 24)
	for y := range cells {
		cells[y] = []rune(strings.Repeat(" ", 100))
	}
	draw := func(x0, y0, x1, y1 int) {
		cells[y0][x0] = '╭'
		cells[y0][x1] = '╮'
		cells[y1][x0] = '╰'
		cells[y1][x1] = '╯'
		for x := x0 + 1; x < x1; x++ {
			cells[y0][x] = '─'
			cells[y1][x] = '─'
		}
		for y := y0 + 1; y < y1; y++ {
			cells[y][x0] = '│'
			cells[y][x1] = '│'
		}
	}
	draw(11, 1, 23, 6)
	draw(37, 1, 90, 18)
	rows := make([]string, len(cells))
	for y := range cells {
		rows[y] = string(cells[y])
	}
	m.linesPlaceBoxes(rows, cells, time.Now())
	queue := m.lines.boxGeo[[4]int{37, 1, 90, 18}]
	if queue.from != [4]float64{17, 6, 72, 13} || queue.to != [4]float64{37, 1, 90, 18} {
		t.Fatalf("popup should return to queue, got %+v", queue)
	}
	cover := m.lines.boxGeo[[4]int{11, 1, 23, 6}]
	if cover.from != cover.to {
		t.Fatalf("cover must not take the popup frame: %+v", cover)
	}
}

func TestClosingPopupWithoutPanelAnimates(t *testing.T) {
	m := coverModel(t, 80, 24)
	m.coverView = true
	m.lines.mode = gfx.File
	m.lines.cellW, m.lines.cellH = 9, 18
	m.noAnim = false
	m.cfg.UIMotion = config.MotionCalm
	popup := [4]int{20, 8, 59, 15}
	from := [4]float64{20, 8, 59, 15}
	m.lines.boxGeo = map[[4]int]boxMorph{popup: {from: from, to: from}}
	m.lines.boxes = map[[4]int]time.Time{popup: time.Now().Add(-time.Second)}
	m.lines.boxSlot = map[[4]int]int{popup: 0}
	m.lines.menuShown = true
	m.lines.menuKey = [7]int{20, 8, 40, 8}
	rows := make([]string, m.height)
	grid := make([][]rune, m.height)
	for y := range rows {
		rows[y] = strings.Repeat(" ", m.width)
		grid[y] = []rune(rows[y])
	}
	now := time.Now()
	m.linesPlaceBoxes(rows, grid, now)
	if !m.lines.closingActive || !m.lines.closingFrame.moving(now) {
		t.Fatal("popup without a destination should animate closed")
	}
	for side := range 4 {
		if st := &m.lines.strips[stripFrame+side]; !st.seen || st.morph.from != from {
			t.Fatalf("side %d did not retain the popup geometry: %+v", side, st.morph)
		}
	}
	m.linesDraw(0.016, now.Add(100*time.Millisecond))
	if st := &m.lines.strips[stripFrame]; !st.morphed || !st.shown {
		t.Fatal("closing frame was not drawn at intermediate geometry")
	}
	m.lines.menuShown = false
	m.linesPlaceBoxes(rows, grid, now.Add(time.Second))
	if m.lines.closingActive {
		t.Fatal("closing animation should finish")
	}
}

func TestPopupKeepsCoveredPanelFrame(t *testing.T) {
	m := coverModel(t, 80, 24)
	m.lines.mode = gfx.File
	m.lines.cellW, m.lines.cellH = 9, 18
	m.cfg.UIMotion = config.MotionCalm
	m.noAnim = false
	makeGrid := func() [][]rune {
		g := make([][]rune, 24)
		for y := range g {
			g[y] = []rune(strings.Repeat(" ", 80))
		}
		return g
	}
	draw := func(g [][]rune, x0, y0, x1, y1 int) {
		for x := x0 + 1; x < x1; x++ {
			g[y0][x], g[y1][x] = '─', '─'
		}
		for y := y0 + 1; y < y1; y++ {
			g[y][x0], g[y][x1] = '│', '│'
		}
		g[y0][x0], g[y0][x1], g[y1][x0], g[y1][x1] = '╭', '╮', '╰', '╯'
	}
	rowsOf := func(g [][]rune) []string {
		rows := make([]string, len(g))
		for y := range g {
			rows[y] = string(g[y])
		}
		return rows
	}
	base := makeGrid()
	draw(base, 2, 2, 45, 20)
	baseRows := rowsOf(base)
	now := time.Now()
	m.linesPlaceBoxes(append([]string(nil), baseRows...), base, now)
	base = makeGrid()
	draw(base, 2, 2, 45, 20)
	panel := [4]int{2, 2, 45, 20}
	slot := m.lines.boxSlot[panel]
	overlaid := makeGrid()
	for y := range base {
		copy(overlaid[y], base[y])
	}
	for y := 8; y <= 15; y++ {
		for x := 30; x <= 65; x++ {
			overlaid[y][x] = ' '
		}
	}
	draw(overlaid, 30, 8, 65, 15)
	m.lines.baseFrames = strings.Join(baseRows, "\n")
	m.lines.overlayRect = fxRect{30, 8, 66, 16}
	m.linesPlaceBoxes(rowsOf(overlaid), overlaid, now.Add(time.Second))
	if got := m.lines.boxGeo[panel]; got.from != got.to || m.lines.boxSlot[panel] != slot {
		t.Fatalf("covered panel frame moved or changed slot: %+v", got)
	}
	popup := [4]int{30, 8, 65, 15}
	if _, ok := m.lines.boxGeo[popup]; !ok {
		t.Fatal("popup frame was not retained separately")
	}
	m.lines.menuShown = true
	m.lines.menuKey = [7]int{30, 8, 36, 8}
	m.lines.baseFrames = strings.Join(baseRows, "\n")
	m.lines.overlayRect = fxRect{}
	// A fading menu letter can briefly cover a corner in the composed
	// screen. The panel must still be found in the underlying layout.
	base[2][2] = 'x'
	m.linesPlaceBoxes(rowsOf(base), base, now.Add(2*time.Second))
	if got := m.lines.boxGeo[panel]; got.from != got.to || m.lines.boxSlot[panel] != slot {
		t.Fatalf("panel frame restarted when popup closed: %+v", got)
	}
	if !m.lines.closingActive || m.lines.closingFrame.to[0] != m.lines.closingFrame.to[2] || m.lines.closingFrame.to[1] != m.lines.closingFrame.to[3] {
		t.Fatal("popup frame should close all the way to its centre")
	}
}
