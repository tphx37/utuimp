package gfx

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Text drawn by the program itself, for words that are part of a picture
// (see the cover view's lyrics): the system's own font, found through
// fontconfig, at any size, with the fonts fontconfig would fall back on for
// the letters the first one lacks.

// Font is a font at one size, with the fonts that stand in for the letters
// it lacks. Those are chosen to look like it: of the same family where the
// system has one for the script (Noto Sans, then Noto Sans CJK, Noto Sans
// Arabic and so on), upright, and as heavy, so that a title reads the same
// in any language.
type Font struct {
	Size    float64 // in pixels
	Ascent  int     // from the top of a line to the baseline
	Height  int     // of a line
	pattern string
	bold    bool
	family  string // the family the first font is of, by its first words
	first   *fontFace
	// The fonts found for the letters the first one lacks, by block of
	// 256 letters.
	more   map[rune][]*fontFace
	opened map[string]*fontFace
	glyphs map[rune]*Glyph
}

type fontFace struct {
	font *sfnt.Font
	face font.Face
	buf  sfnt.Buffer
	thin bool // lighter than was asked for: its letters are thickened
}

// Glyph is one letter, drawn: how solid each of its pixels is.
type Glyph struct {
	W, H    int
	X, Y    int // its corner, from the pen's place on the baseline
	Advance int // how far the pen moves on
	Mask    []byte
}

// fcFont is a font as fontconfig describes it.
type fcFont struct {
	file   string
	index  int
	weight int // fontconfig's: 80 regular, 200 bold
	slant  int // 0 upright
	family string
}

// fcMatch asks fontconfig for the fonts that fit pattern, best first
// (tests replace it).
var fcMatch = func(pattern string) ([]fcFont, error) { //nolint:gochecknoglobals // swapped in tests
	out, err := exec.Command("fc-match", "-s", "-f", "%{file}\t%{index}\t%{weight}\t%{slant}\t%{family}\n", pattern).Output() //nolint:gosec // a font pattern of the program's own
	if err != nil {
		return nil, err
	}
	var fonts []fcFont
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(out), "\n") {
		f := strings.SplitN(line, "\t", 5)
		if len(f) != 5 {
			continue
		}
		low := strings.ToLower(f[0])
		if seen[f[0]+f[1]] || !strings.HasSuffix(low, ".ttf") && !strings.HasSuffix(low, ".otf") && !strings.HasSuffix(low, ".ttc") {
			continue
		}
		seen[f[0]+f[1]] = true
		idx, _ := strconv.Atoi(f[1])
		weight, _ := strconv.Atoi(f[2])
		slant, _ := strconv.Atoi(f[3])
		fonts = append(fonts, fcFont{file: f[0], index: idx, weight: weight, slant: slant, family: f[4]})
	}
	return fonts, nil
}

// The fonts fontconfig gave for each pattern, and each font file read and
// parsed, kept for the program's life: asking fontconfig starts a process
// and a font file is a few megabytes to read, a frame's time or more, and
// the screen must not wait for either (see Warm).
var (
	fontsMu    sync.Mutex
	matched    = map[string][]fcFont{}
	parsed     = map[string]*sfnt.Font{}
	parsedFail = map[string]bool{}
)

// match is fcMatch, once per pattern.
func match(pattern string) ([]fcFont, error) {
	fontsMu.Lock()
	fonts, ok := matched[pattern]
	fontsMu.Unlock()
	if ok {
		return fonts, nil
	}
	fonts, err := fcMatch(pattern)
	if err != nil {
		return nil, err
	}
	fontsMu.Lock()
	matched[pattern] = fonts
	fontsMu.Unlock()
	return fonts, nil
}

// parse reads and parses a font file, once; nil when it can't be used.
func parse(fc fcFont) *sfnt.Font {
	key := fc.file + "#" + strconv.Itoa(fc.index)
	fontsMu.Lock()
	sf, ok := parsed[key]
	failed := parsedFail[key]
	fontsMu.Unlock()
	if ok || failed {
		return sf
	}
	data, err := os.ReadFile(fc.file)
	if err == nil {
		if strings.HasSuffix(strings.ToLower(fc.file), ".ttc") {
			if col, err := opentype.ParseCollection(data); err == nil && col.NumFonts() > 0 {
				sf, _ = col.Font(min(max(fc.index, 0), col.NumFonts()-1))
			}
		} else {
			sf, _ = opentype.Parse(data)
		}
	}
	fontsMu.Lock()
	if sf != nil {
		parsed[key] = sf
	} else {
		parsedFail[key] = true
	}
	fontsMu.Unlock()
	return sf
}

// Warm finds and reads the fonts for patterns ahead of time, on its own
// goroutine, so that the first words drawn in them come at once.
func Warm(patterns ...string) {
	go func() {
		for _, p := range patterns {
			fonts, err := match(p)
			if err != nil {
				continue
			}
			for _, fc := range fonts {
				if fc.slant == 0 && parse(fc) != nil {
					break
				}
			}
		}
	}()
}

// familyStem is the first two words of a family's name: what the fonts
// made to go together share ("Noto Sans" of "Noto Sans CJK JP").
func familyStem(family string) string {
	family, _, _ = strings.Cut(family, ",")
	words := strings.Fields(family)
	if len(words) > 2 {
		words = words[:2]
	}
	return strings.Join(words, " ")
}

// LoadFont finds the font fontconfig gives for pattern ("sans-serif:bold")
// and makes it ready at size pixels.
func LoadFont(pattern string, size float64) (*Font, error) {
	fonts, err := match(pattern)
	if err != nil {
		return nil, err
	}
	f := &Font{Size: size, pattern: pattern, bold: strings.Contains(pattern, "bold"),
		more: map[rune][]*fontFace{}, opened: map[string]*fontFace{}, glyphs: map[rune]*Glyph{}}
	for _, fc := range fonts {
		if fc.slant != 0 {
			continue
		}
		if f.first = f.open(fc); f.first != nil {
			f.family = familyStem(fc.family)
			break
		}
	}
	if f.first == nil {
		return nil, errors.New("no font found for " + pattern)
	}
	m := f.first.face.Metrics()
	f.Ascent = m.Ascent.Ceil()
	f.Height = (m.Ascent + m.Descent).Ceil()
	return f, nil
}

// open reads a font file, once; nil when it can't be used.
func (f *Font) open(fc fcFont) *fontFace {
	key := fc.file + "#" + strconv.Itoa(fc.index)
	if ff, ok := f.opened[key]; ok {
		return ff
	}
	f.opened[key] = nil
	sf := parse(fc)
	if sf == nil {
		return nil
	}
	face, err := opentype.NewFace(sf, &opentype.FaceOptions{Size: f.Size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil
	}
	ff := &fontFace{font: sf, face: face, thin: f.bold && fc.weight < 150}
	f.opened[key] = ff
	return ff
}

// standIns are the fonts to try for the letter r, which the first font
// lacks: those that have it, the first font's own family before the rest,
// upright and of the weight asked for before the others.
func (f *Font) standIns(r rune) []*fontFace {
	block := r >> 8
	if faces, ok := f.more[block]; ok {
		return faces
	}
	fonts, _ := match(fmt.Sprintf("%s:charset=%x", f.pattern, r))
	rank := func(fc fcFont) int {
		n := 0
		if familyStem(fc.family) != f.family {
			n += 4
		}
		if fc.slant != 0 {
			n += 2
		}
		if f.bold != (fc.weight >= 150) {
			n++
		}
		return n
	}
	sort.SliceStable(fonts, func(a, b int) bool { return rank(fonts[a]) < rank(fonts[b]) })
	var faces []*fontFace
	for _, fc := range fonts {
		if len(faces) >= 6 {
			break
		}
		if ff := f.open(fc); ff != nil && ff != f.first {
			faces = append(faces, ff)
		}
	}
	f.more[block] = faces
	return faces
}

// Glyph returns the letter r drawn, from the first font or, when that
// lacks it, from one that stands in for it. A letter with no shape (a
// space), and one no font has, is only its Advance.
func (f *Font) Glyph(r rune) *Glyph {
	if g, ok := f.glyphs[r]; ok {
		return g
	}
	g := &Glyph{}
	f.glyphs[r] = g
	draw := func(ff *fontFace) bool {
		if idx, err := ff.font.GlyphIndex(&ff.buf, r); err != nil || idx == 0 {
			return false
		}
		dr, mask, maskp, adv, ok := ff.face.Glyph(fixed.P(0, 0), r)
		if !ok {
			return false
		}
		g.Advance = adv.Round()
		g.W, g.H, g.X, g.Y = dr.Dx(), dr.Dy(), dr.Min.X, dr.Min.Y
		g.Mask = make([]byte, g.W*g.H)
		for y := range g.H {
			for x := range g.W {
				_, _, _, a := mask.At(maskp.X+x, maskp.Y+y).RGBA()
				g.Mask[y*g.W+x] = byte(a >> 8) //nolint:gosec // a is 16 bits: its top byte
			}
		}
		if ff.thin {
			g.thicken(max(1, int(f.Size/20+0.5)))
		}
		return true
	}
	if draw(f.first) {
		return g
	}
	if slices.ContainsFunc(f.standIns(r), draw) { // the first of them that has it
		return g
	}
	g.Advance = int(f.Size / 2) // a letter no font has: a gap
	return g
}

// thicken makes a letter n pixels heavier sideways: a lighter font standing
// in for a bold one.
func (g *Glyph) thicken(n int) {
	if g.W == 0 || g.H == 0 {
		return
	}
	w := g.W + n
	out := make([]byte, w*g.H)
	for y := range g.H {
		for x := range g.W {
			a := g.Mask[y*g.W+x]
			for d := 0; d <= n; d++ {
				if a > out[y*w+x+d] {
					out[y*w+x+d] = a
				}
			}
		}
	}
	g.Mask, g.W = out, w
	g.Advance += n
}

// Width is how wide s is, in pixels.
func (f *Font) Width(s string) int {
	w := 0
	for _, r := range s {
		w += f.Glyph(r).Advance
	}
	return w
}

// Draw writes s into mask (w×h, a byte a pixel: how solid), its pen
// starting at x on the baseline at y. It returns where the pen ended up
// after each letter.
func (f *Font) Draw(mask []byte, w, h, x, y int, s string) (ends []int) {
	for _, r := range s {
		g := f.Glyph(r)
		for gy := range g.H {
			py := y + g.Y + gy
			if py < 0 || py >= h {
				continue
			}
			for gx := range g.W {
				px := x + g.X + gx
				if px < 0 || px >= w {
					continue
				}
				if a := g.Mask[gy*g.W+gx]; a > mask[py*w+px] {
					mask[py*w+px] = a
				}
			}
		}
		x += g.Advance
		ends = append(ends, x)
	}
	return ends
}
