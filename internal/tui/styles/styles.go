package styles

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Color variables — all mutable so Apply(Theme) can swap the palette at startup.
// Initial values match DefaultTheme (Tokyo Night / Catppuccin Mocha blend).
var (
	ColorPrimary   = lipgloss.Color("#C678DD")
	ColorSecondary = lipgloss.Color("#98C379")
	ColorMuted     = lipgloss.Color("#5C6370")
	ColorError     = lipgloss.Color("#E06C75")
	ColorFg        = lipgloss.Color("#ABB2BF")
	ColorSubtle    = lipgloss.Color("#61AFEF")

	ColorBg = lipgloss.Color("#1a1b26")

	ColorLove       = lipgloss.Color("#f38ba8")
	ColorActive     = lipgloss.Color("#a6e3a1")
	ColorProgress   = lipgloss.Color("#89b4fa")
	ColorSurface    = lipgloss.Color("#2a2b3d")
	ColorAccent     = lipgloss.Color("#7aa2f7")
	ColorAccentWarm = lipgloss.Color("#E5C07B") // amber — labels, search, note animations
	ColorBear       = lipgloss.Color("#C4A265") // bear mascot body

	ColorGlow0 = lipgloss.Color("#1e1e2e")
	ColorGlow1 = lipgloss.Color("#2d2b55")
	ColorGlow2 = lipgloss.Color("#4a3f8a")
	ColorGlow3 = lipgloss.Color("#6e57c4")
	ColorGlow4 = lipgloss.Color("#9d7fea")
	ColorGlow5 = lipgloss.Color("#bb9af7")
	ColorGlow6 = lipgloss.Color("#cba6f7")
	ColorGlow7 = lipgloss.Color("#e0d4ff")

	// ColorBorder / ColorBorderFocus colour the rounded box frames.
	ColorBorder      color.Color = lipgloss.NoColor{}
	ColorBorderFocus             = lipgloss.Color("#C678DD")

	// ANSIMode is true when the active theme uses the terminal's own palette;
	// see Theme.ANSI.
	ANSIMode bool

	// ArtworkActive is true while ProgressGradStops hold the current
	// cover's colours (set by ApplyArtworkPalette).
	ArtworkActive bool
)

var (
	TitleBar = lipgloss.NewStyle().
			Italic(true).
			Foreground(ColorPrimary).
			PaddingLeft(1).
			PaddingRight(1)

	ViewName = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(2)

	StatusBar = lipgloss.NewStyle().
			Foreground(ColorFg).
			PaddingLeft(1).
			PaddingRight(1)

	NowPlayingLabel = lipgloss.NewStyle().
			Italic(true).
			Bold(true).
			Foreground(ColorAccentWarm)

	NowPlayingTitle = lipgloss.NewStyle().
			Italic(true).
			Foreground(ColorFg)

	NowPlayingTitlePlaying = lipgloss.NewStyle().
				Italic(true).
				Bold(true).
				Foreground(ColorGlow7)

	NowPlayingArtist = lipgloss.NewStyle().
				Foreground(ColorPrimary)

	NowPlayingTrackArtist = lipgloss.NewStyle().
				Foreground(ColorPrimary)
	NowPlayingAlbum = lipgloss.NewStyle().
			Foreground(ColorMuted)

	ControlActive = lipgloss.NewStyle().Foreground(ColorActive)

	Playing = lipgloss.NewStyle().
		Foreground(ColorGlow5)

	Paused = lipgloss.NewStyle().
		Foreground(ColorMuted)

	VibingStatus = lipgloss.NewStyle().
			Foreground(ColorMuted)

	Selected = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Italic(true)

	QueueItem = lipgloss.NewStyle().
			Foreground(ColorFg)

	QueueItemMuted = lipgloss.NewStyle().
			Foreground(ColorMuted)

	Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorMuted)

	KeyHint = lipgloss.NewStyle().
		Foreground(ColorMuted).
		PaddingLeft(1)

	KeyName = lipgloss.NewStyle().
		Foreground(ColorAccent)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Italic(true).
			PaddingLeft(1)

	ArtworkFrame = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2).
			Align(lipgloss.Center)

	TabActive = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Underline(true).
			PaddingLeft(1).
			PaddingRight(1)

	TabInactive = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(1).
			PaddingRight(1)

	Spinner = lipgloss.NewStyle().
		Foreground(ColorGlow4)

	SidebarActive = lipgloss.NewStyle().
			Foreground(ColorGlow5).
			Italic(true)

	SidebarInactive = lipgloss.NewStyle().
			Foreground(ColorMuted)

	Separator = lipgloss.NewStyle().
			Foreground(ColorMuted)

	Header = lipgloss.NewStyle().
		Foreground(ColorAccent)

	ModeNormal  = lipgloss.NewStyle().Background(ColorSecondary).Foreground(ColorBg).Bold(true).Padding(0, 1)
	ModeSearch  = lipgloss.NewStyle().Background(ColorSubtle).Foreground(ColorBg).Bold(true).Padding(0, 1)
	ModeCommand = lipgloss.NewStyle().Background(ColorAccentWarm).Foreground(ColorBg).Bold(true).Padding(0, 1)

	ProgressBar = lipgloss.NewStyle().Foreground(ColorProgress)
	ProgressBg  = lipgloss.NewStyle().Foreground(ColorSurface)

	TimeStyle      = lipgloss.NewStyle().Foreground(ColorMuted)
	NowPlayingTime = lipgloss.NewStyle().Foreground(ColorProgress)

	FavoriteActive = lipgloss.NewStyle().Foreground(ColorLove)

	// Bear mascot styles — updated by Apply().
	BearStyle  = lipgloss.NewStyle().Foreground(ColorBear)
	NoteStyle  = lipgloss.NewStyle().Foreground(ColorAccentWarm)
	SleepStyle = lipgloss.NewStyle().Foreground(ColorAccent).Faint(true)
)

// GlowPalette drives the "now playing" breathing animation, from dark to bright.
var GlowPalette = []color.Color{
	ColorGlow0, // 0 — darkest
	ColorGlow1,
	ColorGlow2,
	ColorGlow3,
	ColorGlow4,
	ColorGlow5,
	ColorGlow6,
	ColorGlow7, // 7 — peak brightness
}

// ProgressGradStops are the colour stops for the progress-bar filled gradient
// (left → right: blue → lavender → rose pink). Updated by Apply().
var ProgressGradStops = []color.Color{
	ColorProgress, // blue
	ColorGlow6,    // lavender
	ColorLove,     // rose pink
}

// themeColor parses a theme colour: "#rrggbb", an ANSI index ("0"–"255"), or
// "" for the terminal's default colour.
func themeColor(s string) color.Color {
	if s == "" || s == "default" {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(s)
}

// Apply replaces every style and color variable with values derived from t.
// It must be called once at program startup, before any TUI model is created.
func Apply(t Theme) {
	// Update color vars.
	ColorPrimary = themeColor(t.Primary)
	ColorSecondary = themeColor(t.Secondary)
	ColorMuted = themeColor(t.Muted)
	ColorError = themeColor(t.Error)
	ColorFg = themeColor(t.Fg)
	ColorSubtle = themeColor(t.Subtle)
	ColorBg = themeColor(t.Bg)
	ColorLove = themeColor(t.Love)
	ColorActive = themeColor(t.Active)
	ColorProgress = themeColor(t.Progress)
	ColorSurface = themeColor(t.Surface)
	ColorAccent = themeColor(t.Accent)
	ColorAccentWarm = themeColor(t.AccentWarm)
	ColorBear = themeColor(t.Bear)
	ColorGlow0 = themeColor(t.GlowPalette[0])
	ColorGlow1 = themeColor(t.GlowPalette[1])
	ColorGlow2 = themeColor(t.GlowPalette[2])
	ColorGlow3 = themeColor(t.GlowPalette[3])
	ColorGlow4 = themeColor(t.GlowPalette[4])
	ColorGlow5 = themeColor(t.GlowPalette[5])
	ColorGlow6 = themeColor(t.GlowPalette[6])
	ColorGlow7 = themeColor(t.GlowPalette[7])
	ColorBorder = themeColor(t.Border)
	ColorBorderFocus = themeColor(t.BorderFocus)
	if t.BorderFocus == "" && !t.ANSI {
		ColorBorderFocus = ColorPrimary
	}
	ANSIMode = t.ANSI
	ArtworkActive = false

	// Update glow slices.
	GlowPalette = []color.Color{
		ColorGlow0, ColorGlow1, ColorGlow2, ColorGlow3,
		ColorGlow4, ColorGlow5, ColorGlow6, ColorGlow7,
	}
	ProgressGradStops = []color.Color{ColorProgress, ColorGlow6, ColorLove}

	// Recreate style vars.
	TitleBar = lipgloss.NewStyle().Italic(true).Foreground(ColorPrimary).PaddingLeft(1).PaddingRight(1)
	ViewName = lipgloss.NewStyle().Foreground(ColorMuted).PaddingLeft(2)
	StatusBar = lipgloss.NewStyle().Foreground(ColorFg).PaddingLeft(1).PaddingRight(1)
	NowPlayingLabel = lipgloss.NewStyle().Italic(true).Bold(true).Foreground(ColorAccentWarm)
	NowPlayingTitle = lipgloss.NewStyle().Italic(true).Foreground(ColorFg)
	NowPlayingTitlePlaying = lipgloss.NewStyle().Italic(true).Bold(true).Foreground(ColorGlow7)
	NowPlayingArtist = lipgloss.NewStyle().Foreground(ColorPrimary)
	NowPlayingTrackArtist = lipgloss.NewStyle().Foreground(ColorPrimary)
	NowPlayingAlbum = lipgloss.NewStyle().Foreground(ColorMuted)
	ControlActive = lipgloss.NewStyle().Foreground(ColorActive)
	Playing = lipgloss.NewStyle().Foreground(ColorGlow5)
	Paused = lipgloss.NewStyle().Foreground(ColorMuted)
	VibingStatus = lipgloss.NewStyle().Foreground(ColorMuted)
	Selected = lipgloss.NewStyle().Foreground(ColorPrimary).Italic(true)
	QueueItem = lipgloss.NewStyle().Foreground(ColorFg)
	QueueItemMuted = lipgloss.NewStyle().Foreground(ColorMuted)
	Border = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorMuted)
	KeyHint = lipgloss.NewStyle().Foreground(ColorMuted).PaddingLeft(1)
	KeyName = lipgloss.NewStyle().Foreground(ColorAccent)
	ErrorStyle = lipgloss.NewStyle().Foreground(ColorError).Italic(true).PaddingLeft(1)
	ArtworkFrame = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorPrimary).Padding(1, 2).Align(lipgloss.Center)
	TabActive = lipgloss.NewStyle().Foreground(ColorAccent).Underline(true).PaddingLeft(1).PaddingRight(1)
	TabInactive = lipgloss.NewStyle().Foreground(ColorMuted).PaddingLeft(1).PaddingRight(1)
	Spinner = lipgloss.NewStyle().Foreground(ColorGlow4)
	SidebarActive = lipgloss.NewStyle().Foreground(ColorGlow5).Italic(true)
	SidebarInactive = lipgloss.NewStyle().Foreground(ColorMuted)
	Separator = lipgloss.NewStyle().Foreground(ColorMuted)
	Header = lipgloss.NewStyle().Foreground(ColorAccent)
	ModeNormal = lipgloss.NewStyle().Background(ColorSecondary).Foreground(ColorBg).Bold(true).Padding(0, 1)
	ModeSearch = lipgloss.NewStyle().Background(ColorSubtle).Foreground(ColorBg).Bold(true).Padding(0, 1)
	ModeCommand = lipgloss.NewStyle().Background(ColorAccentWarm).Foreground(ColorBg).Bold(true).Padding(0, 1)
	ProgressBar = lipgloss.NewStyle().Foreground(ColorProgress)
	ProgressBg = lipgloss.NewStyle().Foreground(ColorSurface)
	TimeStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	NowPlayingTime = lipgloss.NewStyle().Foreground(ColorProgress)
	FavoriteActive = lipgloss.NewStyle().Foreground(ColorLove)
	BearStyle = lipgloss.NewStyle().Foreground(ColorBear)
	NoteStyle = lipgloss.NewStyle().Foreground(ColorAccentWarm)
	SleepStyle = lipgloss.NewStyle().Foreground(ColorAccent).Faint(true)
}

// ApplyArtworkPalette dynamically tints the Now Playing accent styles and
// progress bar gradient using Apple Music's per-track artwork color palette.
// Only colors that have legible contrast (WCAG >= 3.0) against the active theme
// background ColorBg are accepted; if none qualify (e.g. dark text colors from
// a pale cover on a dark theme), the active theme defaults are restored.
func ApplyArtworkPalette(_, text1, text2, text3, text4 string) {
	var accents []color.Color
	for _, hex := range []string{text1, text2, text3, text4} {
		if validColor(hex) {
			c := lipgloss.Color(hex)
			if contrastRatio(c, ColorBg) >= 3.0 {
				accents = append(accents, c)
			}
		}
	}
	applyAccents(accents)
}

// ApplyCoverColours tints the same styles as ApplyArtworkPalette with
// colours picked from the cover image itself (for services that send no
// palette), most of the cover first. A colour too close to the theme's
// background is lightened or darkened until it reads, rather than dropped:
// the text keeps the cover's hue.
func ApplyCoverColours(hexes []string) {
	var accents []color.Color
	for _, hex := range hexes {
		if validColor(hex) {
			accents = append(accents, legibleOn(lipgloss.Color(hex), ColorBg))
		}
	}
	applyAccents(accents)
}

// legibleOn moves c towards white (on a dark background) or black (on a
// light one) until it has a contrast of 3 against bg.
func legibleOn(c, bg color.Color) color.Color {
	to := lipgloss.Color("#ffffff")
	if relativeLuminance(bg) > 0.4 {
		to = lipgloss.Color("#000000")
	}
	for k := 0.0; k <= 1; k += 0.05 {
		if m := LerpColor(c, to, k); contrastRatio(m, bg) >= 3.0 {
			return m
		}
	}
	return to
}

func applyAccents(accents []color.Color) {
	if len(accents) == 0 {
		ClearArtworkPalette()
		return
	}
	c1 := accents[0]
	c2 := LerpColor(c1, ColorGlow6, 0.5)
	if len(accents) >= 2 {
		c2 = accents[1]
	}
	c3 := LerpColor(c2, ColorLove, 0.5)
	if len(accents) >= 3 {
		c3 = accents[2]
	}

	ProgressGradStops = []color.Color{c1, c2, c3}
	ArtworkActive = true
	if ANSIMode {
		// Keep the text in the terminal's palette; only the progress line
		// takes the cover's colours.
		return
	}
	NowPlayingTrackArtist = lipgloss.NewStyle().Foreground(c1)
	NowPlayingTitlePlaying = lipgloss.NewStyle().
		Italic(true).
		Bold(true).
		Foreground(LerpColor(c1, lipgloss.Color("#ffffff"), 0.35))
	NowPlayingTime = lipgloss.NewStyle().Foreground(c2)
}

// ClearArtworkPalette restores the Now Playing styles and progress gradient
// stops to the active theme's base colors.
func ClearArtworkPalette() {
	NowPlayingTrackArtist = lipgloss.NewStyle().Foreground(ColorPrimary)
	NowPlayingTitlePlaying = lipgloss.NewStyle().Italic(true).Bold(true).Foreground(ColorGlow7)
	NowPlayingTime = lipgloss.NewStyle().Foreground(ColorProgress)
	ProgressGradStops = []color.Color{ColorProgress, ColorGlow6, ColorLove}
	ArtworkActive = false
}

func relativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	chanLum := func(v uint32) float64 {
		s := float64(v>>8) / 255.0
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*chanLum(r) + 0.7152*chanLum(g) + 0.0722*chanLum(b)
}

func contrastRatio(a, b color.Color) float64 {
	la := relativeLuminance(a)
	lb := relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// LerpColor linearly interpolates between two colours by factor t ∈ [0,1].
// Used to compute gradient colours for dynamic indicators like the similarity bar.
func LerpColor(a, b color.Color, t float64) color.Color {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	if !isRGB(a) || !isRGB(b) {
		// Palette indices have no RGB value we can trust (the terminal
		// decides), so snap to the nearer endpoint instead of blending.
		if t < 0.5 {
			return a
		}
		return b
	}
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	r := uint8(float64(ar>>8) + t*(float64(br>>8)-float64(ar>>8)))
	g := uint8(float64(ag>>8) + t*(float64(bg>>8)-float64(ag>>8)))
	bl := uint8(float64(ab>>8) + t*(float64(bb>>8)-float64(ab>>8)))
	return lipgloss.Color("#" + hex2(r) + hex2(g) + hex2(bl))
}

// isRGB reports whether c is a concrete RGB colour rather than a terminal
// palette index or the default colour.
func isRGB(c color.Color) bool {
	switch c.(type) {
	case ansi.BasicColor, ansi.IndexedColor, lipgloss.NoColor, nil:
		return false
	}
	return true
}

// ColorHex returns the "#rrggbb" lowercase hex representation of a color.Color.
// Useful for comparing color values against theme hex strings.
func ColorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	// RGBA() returns 16-bit alpha-premultiplied values (0-65535); >>8 maps to [0,255]. //nolint:gosec
	return "#" + hex2(uint8(r>>8)) + hex2(uint8(g>>8)) + hex2(uint8(b>>8)) //nolint:gosec
}

func hex2(v uint8) string {
	const h = "0123456789abcdef"
	return string([]byte{h[v>>4], h[v&0xf]})
}

// TermFg / TermBg are the terminal's actual default colours as reported by
// the terminal (OSC 10/11). nil until known. ANSI themes need them to blend
// colours, since palette indices carry no trustworthy RGB value.
var (
	TermFg color.Color
	TermBg color.Color
)

// Palette is the real colours of the terminal's 16 palette entries, once
// PaletteKnown (see QueryPalette).
var (
	Palette      [16][3]uint8
	PaletteKnown bool
)

// SetTerminalColors records colours reported by the terminal; nil leaves a
// value unchanged.
func SetTerminalColors(fg, bg color.Color) {
	if fg != nil && isRGB(fg) {
		TermFg = fg
	}
	if bg != nil && isRGB(bg) {
		TermBg = bg
	}
}

// LyricColors returns the RGB colours karaoke highlighting blends between:
// sung text (the cover's colour when known, else the foreground) and the
// still-to-be-sung text (the foreground faded toward the background).
// ok is false when no RGB values are known (an ANSI theme in a terminal that
// didn't report its colours); callers then fall back to palette styles.
func LyricColors() (sung, dim color.Color, ok bool) {
	fg, bg := ColorFg, ColorBg
	if ANSIMode {
		fg, bg = TermFg, TermBg
	}
	if fg == nil || bg == nil || !isRGB(fg) || !isRGB(bg) {
		return nil, nil, false
	}
	sung = fg
	switch {
	case ArtworkActive && len(ProgressGradStops) > 0 && isRGB(ProgressGradStops[0]):
		sung = LerpColor(ProgressGradStops[0], lipgloss.Color("#ffffff"), 0.25)
	case !ANSIMode && isRGB(ColorGlow7):
		sung = ColorGlow7
	}
	return sung, LerpColor(fg, bg, 0.55), true
}

// LyricFadeColors returns the lyrics' resting grey and the background they
// fade into (the cover view's lines above the current one); ok is false
// without RGB colours to blend.
func LyricFadeColors() (dim, bg color.Color, ok bool) {
	_, dim, ok = LyricColors()
	if !ok {
		return nil, nil, false
	}
	bg = ColorBg
	if ANSIMode {
		bg = TermBg
	}
	return dim, bg, true
}

// ScreenColors returns the real colours of the screen's default text and
// background as RGB: what the terminal reported, else the theme's own (an
// ANSI theme has none: then dark grey on black is assumed). ok is false
// when the background is such a guess.
func ScreenColors() (fg, bg color.RGBA, ok bool) {
	rgb := func(c color.Color) color.RGBA {
		r, g, b, _ := c.RGBA()
		return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255} //nolint:gosec // 16-bit to 8-bit
	}
	fg, bg = color.RGBA{200, 200, 200, 255}, color.RGBA{0, 0, 0, 255}
	switch {
	case TermFg != nil:
		fg = rgb(TermFg)
	case !ANSIMode && isRGB(ColorFg):
		fg = rgb(ColorFg)
	}
	switch {
	case TermBg != nil:
		bg, ok = rgb(TermBg), true
	case !ANSIMode && isRGB(ColorBg):
		bg, ok = rgb(ColorBg), true
	}
	return fg, bg, ok
}
