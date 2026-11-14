package gfx

import (
	"strings"
	"testing"
)

// The letters a font lacks come from its own family first, upright and of
// its weight, whatever order fontconfig gives.
func TestFontStandIns(t *testing.T) {
	real := fcMatch
	t.Cleanup(func() { fcMatch = real })
	fonts, err := real("sans-serif:bold")
	if err != nil || len(fonts) == 0 {
		t.Skip("no fontconfig here")
	}
	f, err := LoadFont("sans-serif:bold", 24)
	if err != nil {
		t.Skip("no font here")
	}
	if f.family == "" || f.Height < 20 || f.Width("abc") < 20 {
		t.Fatalf("the font: family %q, height %d", f.family, f.Height)
	}
	var asked []string
	fcMatch = func(pattern string) ([]fcFont, error) {
		asked = append(asked, pattern)
		return []fcFont{
			{file: "/other-italic.ttf", slant: 100, weight: 200, family: "Other"},
			{file: "/other.ttf", weight: 200, family: "Other Sans"},
			{file: "/kin-light.ttf", weight: 80, family: f.family + " CJK JP"},
			{file: "/kin-bold.ttf", weight: 200, family: f.family + " CJK JP"},
		}, nil
	}
	// None of those files exist, so nothing opens: what matters is that
	// the family is asked for by the letter, once a block.
	f.standIns(0x4e2d)
	f.standIns(0x4e2e)
	if len(asked) != 1 || !strings.Contains(asked[0], "charset=4e2d") {
		t.Errorf("fontconfig asked %v, want once for the letter", asked)
	}
	if familyStem("Noto Sans CJK JP,Noto Sans CJK JP Bold") != "Noto Sans" {
		t.Error("a family is known by its first two words")
	}
	g := &Glyph{W: 2, H: 1, Advance: 3, Mask: []byte{255, 0}}
	g.thicken(1)
	if g.W != 3 || g.Advance != 4 || g.Mask[1] != 255 {
		t.Errorf("a thickened letter: %+v", g)
	}
}

// fontconfig is asked once a pattern and a font file read once, whatever
// the sizes: both take longer than a frame.
func TestFontsKept(t *testing.T) {
	real := fcMatch
	t.Cleanup(func() { fcMatch = real })
	if fonts, err := real("sans-serif"); err != nil || len(fonts) == 0 {
		t.Skip("no fontconfig here")
	}
	asked := 0
	fcMatch = func(pattern string) ([]fcFont, error) {
		asked++
		return real(pattern)
	}
	const pattern = "sans-serif:kept-test"
	a, err := LoadFont(pattern, 20)
	if err != nil {
		t.Skip("no font here")
	}
	b, err := LoadFont(pattern, 31)
	if err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Errorf("fontconfig asked %d times, want once", asked)
	}
	if a.first.font != b.first.font {
		t.Error("the font file was parsed again for another size")
	}
	if b.Height <= a.Height {
		t.Errorf("heights %d, %d: the sizes were mixed up", a.Height, b.Height)
	}
}
