package styles

import (
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// The Linux virtual console (Ctrl+Alt+F2, no desktop) is a terminal of its
// own kind: 16 colours from a harsh VGA palette, italic and underline drawn
// as colour changes, bold as "bright", 256/truecolour squashed to the
// nearest of 8 colours, and a font with only the CP437 glyphs. Console mode
// works around all of it:
//
//   - the palette is reprogrammed (ESC ] P) with the theme's own colours,
//     and restored on exit (ESC ] R);
//   - the UI is drawn with palette indices (ConsoleTheme), and every other
//     colour is mapped to the nearest palette entry;
//   - bright colours (8–15) are sent as bold + base colour, the one form the
//     console tracks correctly; italic, underline, blink, faint and the UI's
//     own bold are dropped;
//   - glyphs the console font lacks are replaced by look-alikes of the same
//     width (ConsoleText).

// Console is true while utuimp runs on the Linux console (see EnterConsole).
var Console bool

// consolePal is the palette set on the console: 0–7 are the normal colours
// (the only ones a background can use), 8–15 the bright ones.
var consolePal [16]color.RGBA

// DetectConsole reports whether the program runs on the Linux virtual
// console. UTUIMP_CONSOLE=1 or 0 forces the answer.
func DetectConsole() bool {
	switch os.Getenv("UTUIMP_CONSOLE") {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	t := os.Getenv("TERM")
	return t == "linux" || strings.HasPrefix(t, "linux-")
}

// ConsolePalette is the 16-colour palette console mode programs for theme t:
// the theme's own colours in the slots ConsoleTheme refers to (9–14 only
// until the first cover takes them over). A theme made
// of palette indices (system) has no colours of its own, so it gets the
// classic theme's.
func ConsolePalette(t Theme) [16]color.RGBA {
	if t.ANSI {
		t = DefaultTheme()
	}
	t = mergeWithDefault(t)
	white := lipgloss.Color("#ffffff")
	hex := lipgloss.Color
	cs := [16]color.Color{
		hex(t.Bg), hex(t.Error), hex(t.Secondary), hex(t.AccentWarm),
		hex(t.Accent), hex(t.Primary), hex(t.Subtle), hex(t.Fg),
		hex(t.Muted), hex(t.Love), hex(t.Active), hex(t.Bear),
		hex(t.Progress), hex(t.GlowPalette[5]), LerpColor(hex(t.Subtle), white, 0.3), hex(t.GlowPalette[7]),
	}
	var out [16]color.RGBA
	for i, c := range cs {
		r, g, b, _ := c.RGBA()
		out[i] = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff} //nolint:gosec // 16-bit channels to 8
	}
	return out
}

// ConsoleTheme is the theme console mode draws with: palette indices that
// point at the slots ConsolePalette fills. It uses 0–8 and 15 only.
func ConsoleTheme() Theme {
	return Theme{
		Primary:    "5",
		Secondary:  "2",
		Muted:      "8",
		Error:      "1",
		Fg:         "",
		Subtle:     "6",
		Bg:         "0",
		Love:       "1",
		Active:     "2",
		Progress:   "4",
		Surface:    "8",
		Accent:     "4",
		AccentWarm: "3",
		Bear:       "3",
		// 9–14 are left to the cover (see art.ConsoleCoverPalette).
		GlowPalette: [8]string{
			"8", "8", "4", "4", "5", "5", "5", "15",
		},
		ModeNormalBg:  "2",
		ModeSearchBg:  "6",
		ModeCommandBg: "3",
		ModeChipFg:    "0",
		Border:        "8",
		BorderFocus:   "5",
		ANSI:          true,
	}
}

// EnterConsole turns console mode on: it programs the palette for theme t
// and returns the function that restores the console's own palette.
func EnterConsole(t Theme) (pal [16]color.RGBA, restore func()) {
	Console = true
	consolePal = ConsolePalette(t)
	_, _ = fmt.Fprint(os.Stdout, consolePaletteSeq(consolePal)) // nothing to do if the console can't take it
	return consolePal, func() { _, _ = fmt.Fprint(os.Stdout, "\x1b]R") }
}

// consolePaletteSeq sets the console's 16 colours (ESC ] P n rrggbb).
func consolePaletteSeq(pal [16]color.RGBA) string {
	var sb strings.Builder
	for i, c := range pal {
		fmt.Fprintf(&sb, "\x1b]P%x%02x%02x%02x", i, c.R, c.G, c.B)
	}
	return sb.String()
}

// nearestConsole returns the palette entry closest to c among indices
// [from, to), skipping 9–14: those follow the cover.
func nearestConsole(c color.RGBA, from, to int) int {
	best, bestD := from, -1
	for i := from; i < to; i++ {
		if i >= 9 && i < 15 {
			continue
		}
		if d := colorDist(c, consolePal[i]); bestD < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// colorDist is the "redmean" weighted RGB distance, close enough to how
// different two colours look for picking a palette entry.
func colorDist(a, b color.RGBA) int {
	rm := (int(a.R) + int(b.R)) / 2
	dr, dg, db := int(a.R)-int(b.R), int(a.G)-int(b.G), int(a.B)-int(b.B)
	return ((512+rm)*dr*dr)>>8 + 4*dg*dg + ((767-rm)*db*db)>>8
}

// consoleGlyphs replaces the UI's glyphs that the console font (CP437) does
// not have. Each replacement takes as many cells as the original, so the
// layout stays put.
var consoleGlyphs = map[rune]string{
	'…': ".", '—': "-", '–': "-", '“': "\"", '”': "\"", '’': "'",
	'▸': "►", '▶': "►", '◂': "◄", '◀': "◄", '⏎': "◄",
	'╭': "┌", '╮': "┐", '╰': "└", '╯': "┘",
	'━': "═", '╸': "═", '╺': "═", '┃': "║",
	'╲': "\\", '╱': "/",
	'⇄': "↔", '↺': "○", '↻': "○", '⏸': "║",
	'✓': "√", '✔': "√", '✗': "x", '✕': "x", '⚠': "!",
	'♡': "♥", '✦': "*", '✧': "*", '☆': "*", '★': "*", '●': "•",
	'ʕ': "(", 'ʔ': ")", 'ᴥ': "w", '˘': "-",
	'🔇': "♪x", '🎤': "♫ ", '🎵': "♪ ", '👩': "  ", '🅴': "E",
	// The spinners, frame by frame, as a turning bar.
	'⠋': "|", '⠙': "/", '⠹': "-", '⠸': "\\", '⠼': "|", '⠴': "/", '⠦': "-", '⠧': "\\", '⠇': "|", '⠏': "/",
	'⣾': "|", '⣽': "/", '⣻': "-", '⢿': "\\", '⡿': "|", '⣟': "/", '⣯': "-", '⣷': "\\",
}

// consoleSpinner stands in for any other braille glyph.
const consoleSpinner = `|/-\`

// ConsoleText rewrites a rendered frame for the Linux console: glyphs from
// consoleGlyphs, colours mapped onto the console palette and the
// attributes the console can't show dropped (see the package comment).
func ConsoleText(s string) string {
	var sb strings.Builder
	sb.Grow(len(s) + len(s)/8)
	for i := 0; i < len(s); {
		c := s[i]
		if c == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j >= len(s) {
				sb.WriteString(s[i:])
				break
			}
			if s[j] == 'm' {
				sb.WriteString(consoleSGR(s[i+2 : j]))
			} else {
				sb.WriteString(s[i : j+1])
			}
			i = j + 1
			continue
		}
		if c < utf8.RuneSelf {
			sb.WriteByte(c)
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch rep, ok := consoleGlyphs[r]; {
		case ok:
			sb.WriteString(rep)
		case r >= 0x2800 && r <= 0x28ff: // braille
			sb.WriteByte(consoleSpinner[int(r)%len(consoleSpinner)])
		default:
			sb.WriteString(s[i : i+size])
		}
		i += size
	}
	return sb.String()
}

// consoleSGR rewrites the parameters of one SGR sequence; it returns "" when
// nothing is left of it.
func consoleSGR(params string) string {
	if params == "" {
		return "\x1b[m"
	}
	toks := strings.Split(params, ";")
	var out []string
	fg := func(idx int) {
		if idx >= 8 {
			out = append(out, "1", strconv.Itoa(30+idx-8))
		} else {
			out = append(out, strconv.Itoa(30+idx))
		}
	}
	bg := func(idx int) { out = append(out, strconv.Itoa(40+idx)) }
	for k := 0; k < len(toks); k++ {
		t := toks[k]
		if base, rest, ok := strings.Cut(t, ":"); ok {
			// Colon forms: 38:5:n, 38:2::r:g:b, 4:3 (underline styles).
			if base == "38" || base == "48" {
				if c, ok := colonColor(rest); ok {
					if base == "38" {
						fg(nearestConsole(c, 1, 16))
					} else {
						bg(nearestConsole(c, 0, 8))
					}
				}
			}
			continue
		}
		n, err := strconv.Atoi(t)
		if t == "" {
			n, err = 0, nil
		}
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			out = append(out, "0")
		case n == 38 || n == 48:
			c, used, ok := extColor(toks[k+1:])
			k += used
			if !ok {
				continue
			}
			if n == 38 {
				if idx, basic := basicIndex(toks[k-used+1:]); basic {
					fg(idx)
				} else {
					fg(nearestConsole(c, 1, 16))
				}
			} else {
				if idx, basic := basicIndex(toks[k-used+1:]); basic && idx < 8 {
					bg(idx)
				} else {
					bg(nearestConsole(c, 0, 8))
				}
			}
		case n >= 30 && n <= 37, n == 39, n >= 40 && n <= 47, n == 49, n == 7, n == 27:
			out = append(out, t)
		case n >= 90 && n <= 97:
			fg(n - 90 + 8)
		case n >= 100 && n <= 107:
			bg(n - 100)
		case n == 22:
			out = append(out, "22")
		default:
			// Bold, faint, italic, underline, blink, strike and their
			// resets: the console shows them as colour changes.
		}
	}
	if len(out) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(out, ";") + "m"
}

// basicIndex reports whether toks (after 38/48) is "5;n" with n < 16, a
// palette colour to keep as it is.
func basicIndex(toks []string) (int, bool) {
	if len(toks) >= 2 && toks[0] == "5" {
		if n, err := strconv.Atoi(toks[1]); err == nil && n >= 0 && n < 16 {
			return n, true
		}
	}
	return 0, false
}

// extColor parses the tokens after 38/48 ("5;n" or "2;r;g;b"); used is how
// many it took.
func extColor(toks []string) (c color.RGBA, used int, ok bool) {
	if len(toks) == 0 {
		return c, 0, false
	}
	switch toks[0] {
	case "5":
		if len(toks) < 2 {
			return c, len(toks), false
		}
		n, err := strconv.Atoi(toks[1])
		if err != nil || n < 0 || n > 255 {
			return c, 2, false
		}
		return xterm256(n), 2, true
	case "2":
		if len(toks) < 4 {
			return c, len(toks), false
		}
		var v [3]uint8
		for i := range 3 {
			x, err := strconv.Atoi(toks[1+i])
			if err != nil || x < 0 || x > 255 {
				return c, 4, false
			}
			v[i] = uint8(x)
		}
		return color.RGBA{v[0], v[1], v[2], 0xff}, 4, true
	}
	return c, 1, false
}

// colonColor parses the part after "38:" ("5:n", "2::r:g:b" or "2:r:g:b").
func colonColor(s string) (color.RGBA, bool) {
	parts := strings.Split(s, ":")
	if len(parts) >= 4 && parts[0] == "2" {
		rgb := parts[len(parts)-3:]
		c, _, ok := extColor(append([]string{"2"}, rgb...))
		return c, ok
	}
	c, _, ok := extColor(parts)
	return c, ok
}

// xterm256 is the RGB value of a 256-colour palette index; 0–15 are taken
// from the console palette.
func xterm256(n int) color.RGBA {
	switch {
	case n < 16:
		return consolePal[n]
	case n < 232:
		n -= 16
		lv := func(v int) uint8 {
			if v == 0 {
				return 0
			}
			return uint8(55 + v*40) //nolint:gosec // 0–255
		}
		return color.RGBA{lv(n / 36), lv(n / 6 % 6), lv(n % 6), 0xff}
	default:
		g := uint8(8 + (n-232)*10) //nolint:gosec // 8–238
		return color.RGBA{g, g, g, 0xff}
	}
}
