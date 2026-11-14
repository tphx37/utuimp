package tui

import (
	"math"
	"os"
	"testing"
)

// haloShown is what the terminal shows of rgba (w×h) stretched over sw×sh pixels
// with linear filtering, over a grey background: one byte a pixel, the
// brightest channel.
func haloShown(rgba []byte, w, h, sw, sh int) []float64 {
	out := make([]float64, sw*sh)
	at := func(x, y, c int) float64 {
		x, y = min(max(x, 0), w-1), min(max(y, 0), h-1)
		return float64(rgba[(y*w+x)*4+c])
	}
	for Y := range sh {
		fy := (float64(Y)+0.5)*float64(h)/float64(sh) - 0.5
		y0 := int(math.Floor(fy))
		ky := fy - float64(y0)
		for X := range sw {
			fx := (float64(X)+0.5)*float64(w)/float64(sw) - 0.5
			x0 := int(math.Floor(fx))
			kx := fx - float64(x0)
			var c [4]float64
			for ch := range 4 {
				top := at(x0, y0, ch)*(1-kx) + at(x0+1, y0, ch)*kx
				bot := at(x0, y0+1, ch)*(1-kx) + at(x0+1, y0+1, ch)*kx
				c[ch] = top*(1-ky) + bot*ky
			}
			a := c[3] / 255
			out[Y*sw+X] = 60*(1-a) + max(c[0], c[1], c[2])*a
		}
	}
	return out
}

// TestHaloScale: the light behind the cover drawn at a third of the
// screen's pixels (linesHaloScale), as the terminal shows it, stretched,
// is the light drawn at full size, to a few of 255 where it can be seen
// (HALO_SCALE=1 reports a half and a quarter too).
func TestHaloScale(t *testing.T) {
	const W, H, mx, my, rad = 640, 540, 70, 63, 9.0
	pal := [3]vcol{{1, 0.4, 0.6}, {1, 0.7, 0.3}, {0.4, 0.5, 1}}
	draw := func(k int) []float64 {
		var s haloShape
		w, h := (W+k-1)/k, (H+k-1)/k
		buf := make([]byte, w*h*4)
		s.draw(buf, w, h, mx/k, my/k, rad/float64(k), 3, 0.6, 1, pal)
		return haloShown(buf, w, h, W, H)
	}
	full := draw(1)
	cmp := func(name string, a []float64) float64 {
		most, sum, n := 0.0, 0.0, 0
		for i := range a {
			x, y := i%W, i/W
			if x >= mx-2 && x < W-mx+2 && y >= my-2 && y < H-my+2 {
				continue // under the cover: never seen
			}
			d := math.Abs(a[i] - full[i])
			most, sum, n = math.Max(most, d), sum+d, n+1
		}
		t.Logf("%s against full: max %.1f, mean %.3f (of 255)", name, most, sum/float64(n))
		return most
	}
	if most := cmp("drawn smaller", draw(linesHaloScale)); most > 4 {
		t.Errorf("the light drawn smaller differs by up to %.1f of 255 where it shows", most)
	}
	if os.Getenv("HALO_SCALE") != "" {
		cmp("half", draw(2))
		cmp("quarter", draw(4))
	}
}
