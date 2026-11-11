package art

import (
	"image"
	"image/color"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderConsole(t *testing.T) {
	var pal [16]color.RGBA
	for i := range pal {
		v := uint8(i * 16) //nolint:gosec // test palette
		pal[i] = color.RGBA{v, v, v, 0xff}
	}
	SetConsolePalette(pal)
	defer func() { consolePalette = nil }()
	if !SupportsColor() {
		t.Fatal("console mode should show covers")
	}

	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = 0xff // white: the top pixels may use 15, the bottom ones only 0–7
	}
	lines := RenderConsole(img, Size{Width: 8, Height: 4})
	if len(lines) != 4 {
		t.Fatalf("got %d lines", len(lines))
	}
	sgr := regexp.MustCompile(`\x1b\[(\d+);(\d+)m`)
	for _, l := range lines {
		if w := ansi.StringWidth(l); w != 8 {
			t.Errorf("line width %d, want 8", w)
		}
		if !strings.HasSuffix(l, "\x1b[m") {
			t.Errorf("line doesn't reset: %q", l)
		}
		for _, m := range sgr.FindAllStringSubmatch(l, -1) {
			if m[1] != "97" {
				t.Errorf("white top pixel drawn with %s, want 97", m[1])
			}
			if m[2] < "40" || m[2] > "47" {
				t.Errorf("background %s outside 40–47", m[2])
			}
		}
	}
}
