package art

import (
	"image"
	"image/color"
	"strconv"
	"strings"
)

// consolePalette is the Linux console's palette while console mode is on
// (see SetConsolePalette); nil otherwise.
var consolePalette []color.RGBA

// SetConsolePalette turns on cover art for the Linux console, drawn with the
// given 16 colours (the ones console mode programs into the console).
func SetConsolePalette(pal [16]color.RGBA) {
	consolePalette = append([]color.RGBA{}, pal[:]...)
}

// ConsoleMode reports whether covers are drawn for the Linux console.
func ConsoleMode() bool { return len(consolePalette) == 16 }

// RenderConsole draws img for the Linux console: half-blocks, Floyd–
// Steinberg dithered to the console palette. The console can give a
// foreground any of its 16 colours but a background only the first 8, so
// each cell is drawn as "▀" or "▄", whichever lets the half that needs a
// bright colour be the foreground. Bright colours go out as SGR 90–97,
// which console mode turns into the bold form the console understands.
func RenderConsole(img image.Image, size Size) []string {
	if img == nil || size.Width <= 0 || size.Height <= 0 || !ConsoleMode() {
		return nil
	}
	pxW, pxH := size.Width, size.Height*2
	grid := boxSample(img, pxW, pxH)
	if grid == nil {
		return nil
	}
	type cell struct {
		fg, bg int
		lower  bool // "▄": the foreground is the bottom half
		full   bool // "█": both halves are the foreground
	}
	cells := make([]cell, size.Width*size.Height)
	spread := func(x, y int, e [3]float64, w float64) {
		if x < 0 || x >= pxW || y >= pxH {
			return
		}
		g := &grid[y*pxW+x]
		for i := range 3 {
			g[i] += e[i] * w
		}
	}
	quant := func(p [3]float64, limit int) (int, [3]float64, float64) {
		i := nearestPal(color.RGBA{clamp8(p[0]), clamp8(p[1]), clamp8(p[2]), 0xff}, limit)
		q := consolePalette[i]
		e := [3]float64{p[0] - float64(q.R), p[1] - float64(q.G), p[2] - float64(q.B)}
		return i, e, errSq(e)
	}
	for row := range size.Height {
		yt, yb := row*2, row*2+1
		// Serpentine: alternate the direction each row so the error
		// doesn't streak to one side.
		dir, x0, x1 := 1, 0, pxW
		if row%2 == 1 {
			dir, x0, x1 = -1, pxW-1, -1
		}
		for x := x0; x != x1; x += dir {
			top, bot := grid[yt*pxW+x], grid[yb*pxW+x]
			// The bottom pixel takes its share of the top one's error first
			// (Floyd–Steinberg's 5/16 straight down).
			below := func(et [3]float64) [3]float64 {
				b := bot
				for i := range 3 {
					b[i] += et[i] * 5 / 16
				}
				return b
			}
			// Three ways to draw the cell; keep the closest:
			// "▀" top from 16 colours, bottom from 8; "▄" the other way
			// round; "█" both halves one of the 16.
			ti, et, dt := quant(top, 16)
			bi, eb, db := quant(below(et), 8)
			c, d := cell{fg: ti, bg: bi}, dt+db
			ti2, et2, dt2 := quant(top, 8)
			bi2, eb2, db2 := quant(below(et2), 16)
			if dt2+db2 < d {
				c, et, eb, d = cell{fg: bi2, bg: ti2, lower: true}, et2, eb2, dt2+db2
			}
			mid := [3]float64{(top[0] + bot[0]) / 2, (top[1] + bot[1]) / 2, (top[2] + bot[2]) / 2}
			fi, _, _ := quant(mid, 16)
			q := consolePalette[fi]
			qv := [3]float64{float64(q.R), float64(q.G), float64(q.B)}
			etF := [3]float64{top[0] - qv[0], top[1] - qv[1], top[2] - qv[2]}
			bF := below(etF)
			ebF := [3]float64{bF[0] - qv[0], bF[1] - qv[1], bF[2] - qv[2]}
			if dF := errSq(etF) + errSq(ebF); dF < d {
				c, et, eb = cell{fg: fi, bg: c.bg, full: true}, etF, ebF
			}
			cells[row*size.Width+x] = c
			// The top pixel's error: right, and into the row below the
			// cell (its share straight down went to the bottom pixel).
			spread(x+dir, yt, et, 7.0/16)
			spread(x-dir, yb, et, 3.0/16)
			spread(x+dir, yb, et, 1.0/16)
			spread(x+dir, yb, eb, 7.0/16)
			spread(x-dir, yb+1, eb, 3.0/16)
			spread(x, yb+1, eb, 5.0/16)
			spread(x+dir, yb+1, eb, 1.0/16)
		}
	}
	lines := make([]string, size.Height)
	for row := range size.Height {
		var sb strings.Builder
		lastFg, lastBg := -1, -1
		for col := range size.Width {
			c := cells[row*size.Width+col]
			if c.fg != lastFg || c.bg != lastBg {
				sb.WriteString("\x1b[")
				if c.fg >= 8 {
					sb.WriteString(strconv.Itoa(90 + c.fg - 8))
				} else {
					sb.WriteString(strconv.Itoa(30 + c.fg))
				}
				sb.WriteString(";" + strconv.Itoa(40+c.bg) + "m")
				lastFg, lastBg = c.fg, c.bg
			}
			switch {
			case c.full:
				sb.WriteString(fullBlock)
			case c.lower:
				sb.WriteString(lowerHalfBlock)
			default:
				sb.WriteString(upperHalfBlock)
			}
		}
		sb.WriteString("\x1b[m")
		lines[row] = sb.String()
	}
	return lines
}

// lowerHalfBlock and fullBlock fill the bottom half and all of a cell.
const (
	lowerHalfBlock = "▄"
	fullBlock      = "█"
)

// errSq weighs a colour error the way the eye roughly does (green most).
func errSq(e [3]float64) float64 { return e[0]*e[0]*2 + e[1]*e[1]*4 + e[2]*e[2]*3 }

// boxSample averages img down to a pxW × pxH grid of RGB values (0–255).
func boxSample(img image.Image, pxW, pxH int) [][3]float64 {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil
	}
	out := make([][3]float64, pxW*pxH)
	for py := range pxH {
		y0 := b.Min.Y + py*srcH/pxH
		y1 := max(b.Min.Y+(py+1)*srcH/pxH, y0+1)
		for px := range pxW {
			x0 := b.Min.X + px*srcW/pxW
			x1 := max(b.Min.X+(px+1)*srcW/pxW, x0+1)
			var rs, gs, bs, n float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, bl, _ := img.At(x, y).RGBA()
					rs += float64(r >> 8)
					gs += float64(g >> 8)
					bs += float64(bl >> 8)
					n++
				}
			}
			out[py*pxW+px] = [3]float64{rs / n, gs / n, bs / n}
		}
	}
	return out
}

func nearestPal(c color.RGBA, limit int) int {
	best, bestD := 0, -1
	for i := range limit {
		q := consolePalette[i]
		rm := (int(c.R) + int(q.R)) / 2
		dr, dg, db := int(c.R)-int(q.R), int(c.G)-int(q.G), int(c.B)-int(q.B)
		d := ((512+rm)*dr*dr)>>8 + 4*dg*dg + ((767-rm)*db*db)>>8
		if bestD < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

func clamp8(v float64) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return uint8(v + 0.5)
}

// Palette entries 9–14 belong to the cover: console mode's UI never draws
// with them (see styles.ConsoleTheme), so each cover reprograms them with
// colours of its own, the ones the fixed entries cover worst.
const coverSlotLo, coverSlotHi = 9, 15

// ConsoleCoverPalette fits the cover's palette entries to img and returns
// the sequence that programs them into the console (ESC ] P), or "" outside
// console mode. Covers rendered afterwards dither to the new entries.
func ConsoleCoverPalette(img image.Image) string {
	if img == nil || !ConsoleMode() || img.Bounds().Empty() {
		return ""
	}
	px := sampleGrid(img, 48, 48)
	toRGB := func(c color.RGBA) rgb { return rgb{float64(c.R), float64(c.G), float64(c.B)} }
	// The fixed entries are centroids that never move; the free ones are
	// seeded maximin (the pixel farthest from every colour so far) and then
	// refined with a few k-means rounds.
	var cent []rgb
	var fixed int
	for i, c := range consolePalette {
		if i < coverSlotLo || i >= coverSlotHi {
			cent = append(cent, toRGB(c))
		}
	}
	fixed = len(cent)
	minD := make([]float64, len(px))
	for i, p := range px {
		minD[i] = dist2(p, cent[nearest(p, cent)])
	}
	for len(cent) < fixed+coverSlotHi-coverSlotLo {
		far := 0
		for i := range px {
			if minD[i] > minD[far] {
				far = i
			}
		}
		cent = append(cent, px[far])
		for i, p := range px {
			minD[i] = min(minD[i], dist2(p, px[far]))
		}
	}
	sums := make([]rgb, len(cent))
	cnt := make([]float64, len(cent))
	for range 8 {
		clear(sums)
		clear(cnt)
		for _, p := range px {
			j := nearest(p, cent)
			for c := range 3 {
				sums[j][c] += p[c]
			}
			cnt[j]++
		}
		for j := fixed; j < len(cent); j++ {
			if cnt[j] > 0 {
				cent[j] = rgb{sums[j][0] / cnt[j], sums[j][1] / cnt[j], sums[j][2] / cnt[j]}
			}
		}
	}
	var sb strings.Builder
	for k, c := range cent[fixed:] {
		slot := coverSlotLo + k
		consolePalette[slot] = color.RGBA{clamp8(c[0]), clamp8(c[1]), clamp8(c[2]), 0xff}
		q := consolePalette[slot]
		sb.WriteString("\x1b]P" + strconv.FormatInt(int64(slot), 16) + hex2(q.R) + hex2(q.G) + hex2(q.B))
	}
	return sb.String()
}

// ConsolePaletteSeq is the sequence that programs the whole palette, the
// cover's entries included, into the console, or "" outside console mode.
// The visualizer sends it when it closes: it draws with a palette of its own.
func ConsolePaletteSeq() string {
	if !ConsoleMode() {
		return ""
	}
	var sb strings.Builder
	for i, q := range consolePalette {
		sb.WriteString("\x1b]P" + strconv.FormatInt(int64(i), 16) + hex2(q.R) + hex2(q.G) + hex2(q.B))
	}
	return sb.String()
}

func hex2(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&0xf]})
}
