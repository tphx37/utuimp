package tui

import (
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/tui/styles"
)

// The start screen: while "♪ utuimp" types itself out and the services
// connect, the ribbons of the visualizer dance behind it, in the theme's
// colours, on the terminal's own background. They come in with a splash
// (the ribbons swell, shiver and throw sparks), sway to a beat of their own
// for as long as the loading takes, and melt into the background towards
// the screen's edges, so nothing frames them. When the player is ready it
// opens out of the logo: a circle of the interface grows from the middle,
// its colours blending in from the ribbons', while the ribbons swell and
// fade around it. On the Linux console the logo stands alone, as before.

const (
	introFrame  = 33 * time.Millisecond // ~30 frames a second
	introFadeIn = 0.15                  // seconds
	introReveal = 0.7                   // seconds the interface takes to open
	introTempo  = 60.0 / 126            // something to bounce to
	introDim    = 0.50                  // the ribbons stay behind the logo
	introMaxCov = 0.45                  // the most a ribbon covers the background
	introSoft   = 0.3                   // width of the edge of the opening circle, as a part of the screen
)

type introVizTickMsg struct{}

func introVizTick() tea.Cmd {
	return tea.Tick(introFrame, func(time.Time) tea.Msg { return introVizTickMsg{} })
}

// introDance is the start screen's own visualizer and its clock.
type introDance struct {
	v      *visualizer
	start  time.Time
	last   time.Time
	beatN  int
	fadeAt time.Time // when the player was ready and the interface began to open
}

// introRibbons reports whether the start screen draws the ribbons: not on
// the Linux console, whose 16 colours the visualizer would have to take.
func introRibbons() bool { return !styles.Console }

// introPalette is the theme's accents (primary, accent, love), as the
// ribbons' three colours. The system theme's colours are indices into the
// terminal's own palette, so the real ones it reported are used, as they
// are (only lifted if too dark to see); any other theme's are softened.
func introPalette() [3]vcol {
	var pal [3]vcol
	for i, c := range []any{styles.ColorPrimary, styles.ColorAccent, styles.ColorLove} {
		if bc, ok := c.(ansi.BasicColor); ok && styles.PaletteKnown {
			rgb := styles.Palette[int(bc)&15]
			h, s, v := hsv(vcol{float64(rgb[0]) / 255, float64(rgb[1]) / 255, float64(rgb[2]) / 255})
			pal[i] = hsvColor(h, s, math.Max(0.6, v))
			continue
		}
		col, ok := c.(interface{ RGBA() (r, g, b, a uint32) })
		if !ok || col == nil {
			return defaultVizPalette
		}
		r, g, b, _ := col.RGBA()
		h, s, v := hsv(vcol{float64(r) / 65535, float64(g) / 65535, float64(b) / 65535})
		// Softer than the theme: half its saturation, a little dimmer,
		// but still a colour, not grey.
		pal[i] = hsvColor(h, math.Max(0.25, math.Min(0.5, s*0.5)), math.Max(0.7, v*0.85))
	}
	return pal
}

// introBegin starts the dance; the returned command drives its frames.
func (m *Model) introBegin() tea.Cmd {
	if !introRibbons() {
		return nil
	}
	now := time.Now()
	v := newVisualizer()
	v.stars = v.stars[:90]
	v.pal = introPalette()
	m.intro = &introDance{v: v, start: now, last: now}
	return introVizTick()
}

// introFinish ends the start screen: at once on the console, by opening
// the interface out of it elsewhere.
func (m *Model) introFinish() {
	if m.intro == nil {
		m.introStep = introDone
		return
	}
	if m.intro.fadeAt.IsZero() {
		m.intro.fadeAt = time.Now()
	}
}

// introStepDance moves the ribbons one frame on and ends the start screen
// once the interface has opened.
func (m *Model) introStepDance() tea.Cmd {
	d := m.intro
	if d == nil || m.introStep == introDone {
		return nil
	}
	now := time.Now()
	if d.leaving(now) >= 1 {
		m.introStep = introDone
		m.intro = nil
		return nil
	}
	dt := min(0.1, now.Sub(d.last).Seconds())
	d.last = now
	d.move(now.Sub(d.start).Seconds(), dt, d.leaving(now))
	return introVizTick()
}

// leaving is how far the interface has opened, 0..1.
func (d *introDance) leaving(now time.Time) float64 {
	if d.fadeAt.IsZero() {
		return 0
	}
	return clamp01(now.Sub(d.fadeAt).Seconds() / introReveal)
}

// move sets what the ribbons "hear" at t seconds: a splash at the start,
// then a steady, cheerful beat; as the interface opens (leave, 0..1) they
// swell and rush, as if pushed aside.
func (d *introDance) move(t, dt, leave float64) {
	v := d.v
	// The splash: up in a fifth of a second, gone in about a second.
	splash := clamp01(t/0.2) * math.Exp(-math.Max(0, t-0.2)*2.2)
	beat := math.Mod(t, introTempo) / introTempo
	kick := math.Exp(-beat * 6)
	if n := int(t / introTempo); n != d.beatN {
		d.beatN = n
		// Each beat shivers a ribbon of its own, round and round.
		v.pulse[n%voiceCount] = 1
		v.hueTo += 0.07
	}
	base := 0.3 + 0.25*smooth(0, 1.5, t)
	for i := range voiceCount {
		fi := float64(i)
		sway := 0.12 * math.Sin(t*(1.3+fi*0.4)+fi*1.7)
		push := 0.25 * kick
		if i != 0 {
			push *= 0.5 // the bass ribbon jumps the most
		}
		v.voice[i] = clamp01(base + 0.4*splash + push + sway + 0.5*leave)
		v.pulse[i] = math.Max(v.pulse[i]*math.Exp(-dt*7), splash)
	}
	v.hue += (v.hueTo - v.hue) * min(1, dt*6)
	// The clock lurches on every beat, and rushes in the splash and as the
	// interface opens.
	v.t += dt * (0.6 + 1.4*kick + 3*splash + 4*leave)
}

// renderIntroDance draws the start screen over the whole body: the ribbons,
// the logo and the status line on top, and, while the interface opens, the
// interface (main, rendered as it will be shown) growing out of the middle.
// "" while there is nothing to draw yet.
func (m *Model) renderIntroDance(logo, status string, main func() string) string {
	d := m.intro
	if d == nil || m.width <= 0 || m.height <= 0 {
		return ""
	}
	v := d.v
	w, h := m.width, m.height
	v.resize(w, h)
	asp := m.artCellAsp
	if asp <= 0 {
		asp = 2
	}
	v.ya = asp / 2
	v.sc = float64(v.h2) * v.ya / 2
	v.cx, v.cy = float64(w)/2, float64(v.h2)/2
	v.drawRibbons()
	v.post()
	v.fb, v.prev = v.prev, v.fb

	now := time.Now()
	t := now.Sub(d.start).Seconds()
	leave := d.leaving(now)
	alpha := clamp01(t/introFadeIn) * introDim * (1 - 0.5*leave)

	fgc, bgc, _ := styles.ScreenColors()
	fg := [3]uint8{fgc.R, fgc.G, fgc.B}
	bg := [3]uint8{bgc.R, bgc.G, bgc.B}

	var ui [][]uiCell
	if leave > 0 && main != nil {
		ui = parseCells(main(), w, h, fg, bg)
	}
	logoRow := max(0, (h-3)/2)
	// How far the interface has opened at a cell: a circle grows from the
	// logo, with a soft edge. Eased, so it starts gently and settles.
	open := func(x, y int) float64 {
		if ui == nil {
			return 0
		}
		e := leave * leave * (3 - 2*leave) * (1 + introSoft)
		dx := (float64(x) + 0.5 - float64(w)/2) / (float64(w) / 2)
		dy := (float64(y) + 0.5 - float64(logoRow)) * 2 / (float64(w) / 2)
		r := math.Hypot(dx, dy) / math.Hypot(1, float64(h)/(float64(w)/2))
		return clamp01((e - r) / introSoft)
	}

	// The logo shimmers: a wave of the ribbons' colours runs along it.
	runes := []rune(logo)
	logoFg := make([][3]uint8, len(runes))
	for i := range runes {
		c := v.palAt(float64(i)*0.12 - t*0.6)
		logoFg[i] = blend3(toByte3(c), fg, 0.5)
	}
	texts := []vizText{{row: logoRow, text: logo, fg: logoFg}}
	if status != "" && logoRow+2 < h {
		texts = append(texts, vizText{row: logoRow + 2, text: status, fg: [][3]uint8{blend3(fg, bg, 0.45)}})
	}
	return encodeIntro(v, alpha, fg, bg, texts, ui, open)
}

// encodeIntro writes the start screen. The ribbons are laid on the
// terminal's background: a pixel is the background, coloured by the ribbon
// as much as the ribbon is bright there, and less so towards the screen's
// edges; a cell with no ribbon in it is left to the terminal (its default
// background), so the picture has no edge of its own. Where the interface
// has opened (open 1) its cells are shown as they are; on the opening's
// edge its characters come in with their colours mixed from the ribbons'.
func encodeIntro(v *visualizer, alpha float64, fg, bg [3]uint8, texts []vizText, ui [][]uiCell, open func(x, y int) float64) string {
	w, h, h2 := v.w, v.h, v.h2
	bgv := vcol{float64(bg[0]) / 255, float64(bg[1]) / 255, float64(bg[2]) / 255}
	// pixel is the colour of a pixel and how much of the ribbons is in it.
	pixel := func(x, y int) ([3]uint8, float64) {
		c := v.out[y*w+x].mul(alpha)
		tm := toByte3(c)
		lum := float64(max(tm[0], tm[1], tm[2])) / 255
		ex := smooth(0, 8, float64(min(x, w-1-x)))
		ey := smooth(0, 6, float64(min(y, h2-1-y)))
		// The faint light the scene lays under its ribbons is left out:
		// away from them the background is the terminal's own.
		cov := smooth(0.08, 0.7, lum) * ex * ey * introMaxCov
		if lum <= 0 {
			return bg, 0
		}
		full := vcol{float64(tm[0]) / 255 / lum, float64(tm[1]) / 255 / lum, float64(tm[2]) / 255 / lum}
		return toByte3Lin(bgv.mix(full, cov)), cov
	}
	var sb strings.Builder
	sb.Grow(w * h * 30)
	last := ""
	emit := func(sgr, ch string) {
		if sgr != last {
			sb.WriteString("\x1b[")
			sb.WriteString(sgr)
			sb.WriteByte('m')
			last = sgr
		}
		sb.WriteString(ch)
	}
	for row := range h {
		var tr []rune
		tx := -1
		var tfg [][3]uint8
		for _, t := range texts {
			if t.row == row {
				tr = []rune(t.text)
				if len(tr) > w {
					tr = tr[:w]
				}
				tx, tfg = (w-len(tr))/2, t.fg
			}
		}
		skip := false // the right half of a wide character just written
		// uiChar is the interface's character at col, a space for the
		// right half of a wide one whose left half wasn't written.
		uiChar := func(col int) string {
			c := ui[row][col]
			if c.ch == "" {
				return " "
			}
			skip = col+1 < w && ui[row][col+1].ch == ""
			return c.ch
		}
		for col := 0; col < w; col++ {
			if skip {
				skip = false
				continue
			}
			k := open(col, row)
			if k >= 1 {
				c := ui[row][col]
				emit("0;"+c.sgr, uiChar(col))
				continue
			}
			top, ct := pixel(col, row*2)
			bot, cb := pixel(col, row*2+1)
			avg := blend3(top, bot, 0.5)
			if k > 0 {
				c := ui[row][col]
				f := blend3(avg, c.fg, k)
				b := blend3(avg, c.bg, k)
				emit("0;"+rgbSGR(38, f)+";"+rgbSGR(48, b), uiChar(col))
				continue
			}
			if tx >= 0 && col >= tx && col < tx+len(tr) {
				f := fg
				if len(tfg) > 0 {
					f = tfg[min(col-tx, len(tfg)-1)]
				}
				// The letters stand on the background, the ribbons
				// pushed back behind them.
				b := blend3(avg, bg, 0.65)
				if ct < 0.02 && cb < 0.02 {
					emit("0;"+rgbSGR(38, f), string(tr[col-tx]))
				} else {
					emit("0;"+rgbSGR(38, f)+";"+rgbSGR(48, b), string(tr[col-tx]))
				}
				continue
			}
			if ct < 0.02 && cb < 0.02 {
				emit("0", " ")
				continue
			}
			emit("0;"+rgbSGR(38, top)+";"+rgbSGR(48, bot), "▀")
		}
		sb.WriteString("\x1b[0m")
		last = "0"
		if row < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// toByte3Lin turns a colour in 0..1 into bytes as it is (no tone mapping).
func toByte3Lin(c vcol) [3]uint8 {
	f := func(x float64) uint8 { return uint8(math.Round(clamp01(x) * 255)) }
	return [3]uint8{f(c.r), f(c.g), f(c.b)}
}

func rgbSGR(kind int, c [3]uint8) string {
	return strconv.Itoa(kind) + ";2;" + strconv.Itoa(int(c[0])) + ";" + strconv.Itoa(int(c[1])) + ";" + strconv.Itoa(int(c[2]))
}

// introLogo is the logo as far as it has typed itself out.
func (m *Model) introLogo() string {
	if m.introStep < 0 || m.introStep >= len(introFrames) {
		return ""
	}
	return strings.TrimRight(introFrames[m.introStep], " ")
}
