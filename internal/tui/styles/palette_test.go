//go:build unix

package styles

import (
	"fmt"
	"strings"
	"testing"
)

func TestParsePalette(t *testing.T) {
	defer func(p [16][3]uint8, k bool) { Palette, PaletteKnown = p, k }(Palette, PaletteKnown)
	PaletteKnown = false
	var sb strings.Builder
	for i := range 15 {
		fmt.Fprintf(&sb, "\x1b]4;%d;rgb:%02x%02x/1010/2020\x07", i, i, i)
	}
	ParsePalette([]byte(sb.String()))
	if PaletteKnown {
		t.Fatal("15 of 16 answers is not a palette")
	}
	sb.WriteString("\x1b]4;15;rgb:ff/80/00\x1b\\\x1b[?62c")
	ParsePalette([]byte(sb.String()))
	if !PaletteKnown || Palette[3] != [3]uint8{3, 0x10, 0x20} || Palette[15] != [3]uint8{0xff, 0x80, 0} {
		t.Errorf("known=%v %v %v", PaletteKnown, Palette[3], Palette[15])
	}
}
