package tui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The app's icon is a frame of the ribbons scene, drawn by the visualizer
// itself into a tiny terminal of iconPx×iconPx pixels (half-block cells, as
// on screen, through the same glow and colours), with a note in the middle
// of the ribbons. To draw it again:
//
//	UTUIMP_ICON=1 go test ./internal/tui -run TestRenderIcon
//
// writes every copy of it: the embedded SVG, the README's SVG and PNG, the
// 512×512 PNG the install script and the Flatpak take, a square copy
// (assets/utuimp_logo_square.*) and the sign-in pages' inline logo.
const (
	iconPx     = 48 // pixels across: as coarse as a terminal shows it
	iconCorner = 9  // radius of the rounded corners, in pixels
)

// iconPalette paints the icon's ribbons in black and white: three greys
// instead of a cover's hues, so the icon wears no colour of its own.
var iconPalette = [3]vcol{{0.5, 0.5, 0.5}, {0.22, 0.22, 0.22}, {0.35, 0.35, 0.35}}

// iconNote is the note, one character per pixel: '#' the note, '.' its
// dark outline, space nothing.
var iconNote = []string{
	"      ..           ",
	"     .##.          ",
	"     .###.         ",
	"     .####.        ",
	"     .##.##.       ",
	"     .##..##.      ",
	"     .##. .##.     ",
	"     .##.  .##.    ",
	"     .##.   .#.    ",
	"     .##.   .#.    ",
	"     .##.  .#.     ",
	"     .##.  ..      ",
	"     .##.          ",
	"     .##.          ",
	"     .##.          ",
	"  .....##.         ",
	" .#######.         ",
	".########.         ",
	".########.         ",
	".#######.          ",
	" .#####.           ",
	"  .....            ",
}

// iconPixels draws the icon: rgb per pixel, and whether the pixel is inside
// the rounded square.
func iconPixels() ([][3]uint8, []bool) { return iconPixelsOf(iconPx, iconCorner, iconNote, 28) }

// iconPixelsOf draws the icon w pixels across, with corners of the given
// radius, the note drawn by note and the given number of sparks.
func iconPixelsOf(w, corner int, note []string, sparks int) ([][3]uint8, []bool) {
	h := w / 2
	v := newVisualizer()
	v.pal = iconPalette
	v.w, v.h, v.h2 = w, h, h*2
	n := w * h * 2
	v.fb, v.prev, v.out = make([]vcol, n), make([]vcol, n), make([]vcol, n)
	g := ((w + 1) / 2) * ((v.h2 + 1) / 2)
	v.glowA, v.glowB = make([]vcol, g), make([]vcol, g)
	v.ya = 1 // square pixels: a cell twice as tall as it is wide
	v.sc = float64(v.h2) * v.ya / 2 * 1.6
	v.cx, v.cy = float64(w)/2, float64(v.h2)/2
	// A lively moment of a song: every voice busy, the bass the most.
	v.voice = [voiceCount]float64{0.9, 0.75, 0.6, 0.7, 0.55}
	v.t = 5                    // the ribbons crossing through the middle
	v.stars = v.stars[:sparks] // a few: more would be noise at this size
	v.drawRibbons()
	v.post()

	px := make([][3]uint8, n)
	for i, c := range v.out {
		// Grey through and through: the glow mixes no tint back in.
		b := toByte3(c)
		l := uint8((299*int(b[0]) + 587*int(b[1]) + 114*int(b[2])) / 1000) //nolint:gosec // a weighted mean of bytes: 0..255
		px[i] = [3]uint8{l, l, l}
	}
	// The note, its stem in the middle: the stem is the run of '#' a third
	// of the way down.
	nh := len(note)
	mid := note[nh/3]
	stem := strings.IndexByte(mid, '#')
	stemW := len(mid[stem:]) - len(strings.TrimLeft(mid[stem:], "#"))
	// A stem of odd width on an even square has no middle pixel: round it
	// right, since the note's head hangs off to the left of it.
	ox, oy := (w-stemW+1)/2-stem, (v.h2-nh)/2
	for y, row := range note {
		for x, ch := range row {
			i := (oy+y)*w + ox + x
			switch ch {
			case '#':
				px[i] = [3]uint8{250, 250, 250}
			case '.':
				px[i] = scale3(px[i], 0.2)
			}
		}
	}
	in := make([]bool, n)
	for y := range v.h2 {
		for x := range w {
			// Distance from the nearest corner's centre, in whole pixels:
			// a stepped curve, as a terminal would draw it.
			dx := max(0, max(corner-x, x-(w-1-corner)))
			dy := max(0, max(corner-y, y-(v.h2-1-corner)))
			in[y*w+x] = dx*dx+dy*dy <= corner*corner
		}
	}
	return px, in
}

// iconRadius is the radius of the icon files' corners, in icon pixels: the
// same round as the terminal's stepped one, but a smooth curve, since a
// launcher shrinks the icon and steps there only look ragged.
const iconRadius = iconCorner + 0.5

func TestRenderIcon(t *testing.T) {
	if os.Getenv("UTUIMP_ICON") == "" {
		t.Skip("set UTUIMP_ICON=1 to draw the icon")
	}
	// Drawn square: the corners are cut by the files themselves, smoothly.
	px, _ := iconPixelsOf(iconPx, 0, iconNote, 28)
	root := filepath.Join("..", "..")
	svg := iconSVG(px, iconRadius)
	for _, p := range []string{"internal/assets/utuimp.svg", "assets/utuimp_logo.svg"} {
		if err := os.WriteFile(filepath.Join(root, p), svg, 0o644); err != nil { //nolint:gosec // an icon
			t.Fatal(err)
		}
	}
	for p, size := range map[string]int{
		"assets/utuimp_logo.png": 1024,
		"assets/logo.png":        512,
		"flatpak/logo.png":       512,
	} {
		if err := writeIconPNG(filepath.Join(root, p), px, iconRadius, size); err != nil {
			t.Fatal(err)
		}
	}
	// A square copy, without the rounded corners, for places that round
	// or mask the icon themselves.
	if err := os.WriteFile(filepath.Join(root, "assets/utuimp_logo_square.svg"), iconSVG(px, 0), 0o644); err != nil { //nolint:gosec // an icon
		t.Fatal(err)
	}
	if err := writeIconPNG(filepath.Join(root, "assets/utuimp_logo_square.png"), px, 0, 1024); err != nil {
		t.Fatal(err)
	}
	// The sign-in pages show it too, inline and square, one PNG pixel per
	// icon pixel: the page scales it up (image-rendering: pixelated) and
	// rounds its corners (border-radius), both crisply at any size.
	var buf bytes.Buffer
	if err := encodeIconPNG(&buf, px, 0, iconPx); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(root, "internal", "signin", "page.html")
	html, err := os.ReadFile(page) //nolint:gosec // our own path
	if err != nil {
		t.Fatal(err)
	}
	logo := regexp.MustCompile(`src="data:image/png;base64,[^"]*"`)
	if !logo.Match(html) {
		t.Fatal("no inline logo in internal/signin/page.html")
	}
	html = logo.ReplaceAllLiteral(html, []byte(`src="data:image/png;base64,`+base64.StdEncoding.EncodeToString(buf.Bytes())+`"`))
	if err := os.WriteFile(page, html, 0o644); err != nil { //nolint:gosec // a page
		t.Fatal(err)
	}
}

// iconSVG draws the pixels as rects, one per run of one colour along a row,
// clipped to a square with corners of radius r (none if 0). The pixels are
// drawn crisp; the clip's curve is left smooth.
func iconSVG(px [][3]uint8, r float64) []byte {
	const w = iconPx
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="1024" height="1024">`+"\n", w, w)
	if r > 0 {
		fmt.Fprintf(&sb, `<defs><clipPath id="corners"><rect width="%d" height="%d" rx="%g"/></clipPath></defs>`+"\n", w, w, r)
		sb.WriteString(`<g clip-path="url(#corners)" shape-rendering="crispEdges">` + "\n")
	} else {
		sb.WriteString(`<g shape-rendering="crispEdges">` + "\n")
	}
	for y := range w {
		for x := 0; x < w; {
			i := y*w + x
			e := x + 1
			for e < w && px[y*w+e] == px[i] {
				e++
			}
			c := px[i]
			fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="%d" height="1" fill="#%02x%02x%02x"/>`+"\n", x, y, e-x, c[0], c[1], c[2])
			x = e
		}
	}
	sb.WriteString("</g>\n</svg>\n")
	return []byte(sb.String())
}

// writeIconPNG draws each pixel as a square, the corners cut round with
// radius r (in icon pixels; none if 0).
func writeIconPNG(path string, px [][3]uint8, r float64, size int) error {
	f, err := os.Create(path) //nolint:gosec // our own paths
	if err != nil {
		return err
	}
	if err := encodeIconPNG(f, px, r, size); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// encodeIconPNG writes the icon as a PNG size pixels across, its corners cut
// round with radius r, their edge antialiased.
func encodeIconPNG(out io.Writer, px [][3]uint8, r float64, size int) error {
	const w = iconPx
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	unit := float64(w) / float64(size) // icon pixels per output pixel
	const ss = 4                       // samples per side
	for y := range size {
		for x := range size {
			hit := 0
			for sy := range ss {
				for sx := range ss {
					u := (float64(x) + (float64(sx)+0.5)/ss) * unit
					v := (float64(y) + (float64(sy)+0.5)/ss) * unit
					dx := math.Max(0, math.Max(r-u, u-(w-r)))
					dy := math.Max(0, math.Max(r-v, v-(w-r)))
					if dx*dx+dy*dy <= r*r {
						hit++
					}
				}
			}
			if hit == 0 {
				continue
			}
			c := px[(y*w/size)*w+x*w/size]
			img.SetNRGBA(x, y, color.NRGBA{c[0], c[1], c[2], uint8(255 * hit / (ss * ss))})
		}
	}
	return png.Encode(out, img)
}

// The icon must show the note on top of the ribbons, not lose it in them.
func TestIconHasTheNote(t *testing.T) {
	px, in := iconPixels()
	white, lit := 0, 0
	for i, c := range px {
		if !in[i] {
			continue
		}
		if c == [3]uint8{250, 250, 250} {
			white++
		} else if max(c[0], c[1], c[2]) > 120 {
			lit++
		}
	}
	if white < 60 {
		t.Errorf("only %d pixels of the note", white)
	}
	if lit < 150 {
		t.Errorf("only %d lit pixels of ribbons around the note", lit)
	}
}

// aboutLogoPx is the size of the icon in the About panel: half of it, a
// square of 24×24 pixels (12 rows of half-block cells).
const aboutLogoPx = iconPx / 2

// aboutNote is the note at the About logo's size: a stem one pixel wide.
var aboutNote = []string{
	"   .    ",
	"  .#.   ",
	"  .##.  ",
	"  .#.#. ",
	"  .#..#.",
	"  .#. . ",
	"  .#.   ",
	" ..#.   ",
	".###.   ",
	".###.   ",
	" ...    ",
}

// TestRenderAboutLogo writes the About panel's small copy of the icon
// (internal/tui/views/about_logo.go), drawn at its own size with a note of
// its own (one shrunk from the big icon loses the note):
//
//	UTUIMP_ICON=1 go test ./internal/tui -run TestRenderAboutLogo
func TestRenderAboutLogo(t *testing.T) {
	if os.Getenv("UTUIMP_ICON") == "" {
		t.Skip("set UTUIMP_ICON=1 to draw the icon")
	}
	px, in := iconPixelsOf(aboutLogoPx, 4, aboutNote, 10)
	var sb strings.Builder
	sb.WriteString("// Code generated by TestRenderAboutLogo (internal/tui/icon_test.go); DO NOT EDIT.\n\n")
	sb.WriteString("package views\n\n")
	sb.WriteString("// aboutLogo is the app's icon, small: 0xRRGGBB per pixel, row by row, or\n")
	sb.WriteString("// -1 outside its rounded corners.\n")
	fmt.Fprintf(&sb, "var aboutLogo = [%d][%d]int32{\n", aboutLogoPx, aboutLogoPx)
	for y := range aboutLogoPx {
		sb.WriteString("\t{")
		for x := range aboutLogoPx {
			i := y*aboutLogoPx + x
			if x > 0 {
				sb.WriteString(", ")
			}
			if !in[i] {
				sb.WriteString("-1")
				continue
			}
			c := px[i]
			fmt.Fprintf(&sb, "0x%02x%02x%02x", c[0], c[1], c[2])
		}
		sb.WriteString("},\n")
	}
	sb.WriteString("}\n")
	if err := os.WriteFile(filepath.Join("views", "about_logo.go"), []byte(sb.String()), 0o644); err != nil { //nolint:gosec // generated source
		t.Fatal(err)
	}
}
