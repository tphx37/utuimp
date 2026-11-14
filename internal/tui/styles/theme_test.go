package styles_test

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/tphx37/utuimp/internal/tui/styles"
)

func TestLoadTheme_BuiltIn(t *testing.T) {
	for _, name := range styles.BuiltinThemeNames() {
		t.Run(name, func(t *testing.T) {
			theme, err := styles.LoadTheme(name, "/tmp")
			if err != nil {
				t.Fatalf("unexpected error for built-in %q: %v", name, err)
			}
			if theme.Primary == "" {
				t.Errorf("expected non-empty Primary for theme %q", name)
			}
		})
	}
}

func TestLoadTheme_Default(t *testing.T) {
	d := styles.SystemTheme()
	for _, name := range []string{"", "default", "system"} {
		theme, err := styles.LoadTheme(name, "/tmp")
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", name, err)
		}
		if theme.Primary != d.Primary {
			t.Errorf("expected default primary %q, got %q", d.Primary, theme.Primary)
		}
	}
}

func TestLoadTheme_ClassicIsOldDefault(t *testing.T) {
	theme, err := styles.LoadTheme("classic", "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if theme.Primary != styles.DefaultTheme().Primary {
		t.Errorf("classic primary = %q, want %q", theme.Primary, styles.DefaultTheme().Primary)
	}
}

func TestSystemThemeUsesTerminalPalette(t *testing.T) {
	th := styles.SystemTheme()
	if !th.ANSI {
		t.Fatal("system theme must be marked ANSI")
	}
	for _, c := range []string{th.Primary, th.Muted, th.Accent, th.Progress} {
		if strings.HasPrefix(c, "#") {
			t.Errorf("system theme colour %q is RGB, want an ANSI index", c)
		}
	}
}

func TestLoadTheme_BuiltInsHaveDistinctColors(t *testing.T) {
	def := styles.DefaultTheme()
	for _, name := range []string{"dracula", "gruvbox", "nord"} {
		theme, _ := styles.LoadTheme(name, "/tmp")
		if theme.Primary == def.Primary {
			t.Errorf("theme %q has same Primary as default; expected distinct color", name)
		}
	}
}

func TestLoadTheme_Custom(t *testing.T) {
	dir := t.TempDir()
	themesDir := filepath.Join(dir, "themes")
	if err := os.MkdirAll(themesDir, 0o700); err != nil {
		t.Fatal(err)
	}

	custom := styles.DefaultTheme()
	custom.Primary = "#ff0000"
	custom.Bear = "#00ff00"

	data, _ := json.Marshal(custom)
	if err := os.WriteFile(filepath.Join(themesDir, "mycolor.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	theme, err := styles.LoadTheme("mycolor", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if theme.Primary != "#ff0000" {
		t.Errorf("expected Primary #ff0000, got %q", theme.Primary)
	}
	if theme.Bear != "#00ff00" {
		t.Errorf("expected Bear #00ff00, got %q", theme.Bear)
	}
}

func TestLoadTheme_InvalidColor_FallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	themesDir := filepath.Join(dir, "themes")
	_ = os.MkdirAll(themesDir, 0o700)

	raw := `{"primary": "notacolor", "secondary": "#50fa7b"}`
	_ = os.WriteFile(filepath.Join(themesDir, "bad.json"), []byte(raw), 0o600)

	theme, err := styles.LoadTheme("bad", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	def := styles.DefaultTheme()
	if theme.Primary != def.Primary {
		t.Errorf("expected invalid Primary to fall back to %q, got %q", def.Primary, theme.Primary)
	}
	if theme.Secondary != "#50fa7b" {
		t.Errorf("expected valid Secondary to be kept, got %q", theme.Secondary)
	}
}

func TestLoadTheme_UnknownName_FallsBackToDefault(t *testing.T) {
	theme, err := styles.LoadTheme("nonexistent-theme", "/tmp")
	if err == nil {
		t.Error("expected error for unknown theme, got nil")
	}
	def := styles.DefaultTheme()
	if theme.Primary != def.Primary {
		t.Errorf("expected default Primary %q, got %q", def.Primary, theme.Primary)
	}
}

func TestLoadTheme_CustomGlowPalette_PartialDefault(t *testing.T) {
	dir := t.TempDir()
	themesDir := filepath.Join(dir, "themes")
	_ = os.MkdirAll(themesDir, 0o700)

	// Only first glow entry valid, rest invalid/empty.
	raw := `{"glow_palette": ["#000000","","","","","","",""]}`
	_ = os.WriteFile(filepath.Join(themesDir, "glow.json"), []byte(raw), 0o600)

	theme, _ := styles.LoadTheme("glow", dir)
	if theme.GlowPalette[0] != "#000000" {
		t.Errorf("expected GlowPalette[0] to be #000000, got %q", theme.GlowPalette[0])
	}
	def := styles.DefaultTheme()
	for i := 1; i < 8; i++ {
		if theme.GlowPalette[i] != def.GlowPalette[i] {
			t.Errorf("expected GlowPalette[%d] to fall back to default %q, got %q",
				i, def.GlowPalette[i], theme.GlowPalette[i])
		}
	}
}

func TestApply_NoPanic(t *testing.T) {
	for _, name := range styles.BuiltinThemeNames() {
		t.Run(name, func(t *testing.T) {
			theme, _ := styles.LoadTheme(name, "/tmp")
			styles.Apply(theme) // must not panic
		})
	}
	// Restore default.
	styles.Apply(styles.DefaultTheme())
}

func TestApply_ChangesColors(t *testing.T) {
	orig := styles.ColorPrimary

	styles.Apply(styles.DraculaTheme())
	if styles.ColorPrimary == orig {
		t.Error("expected ColorPrimary to change after Apply(DraculaTheme)")
	}
	if !strings.EqualFold(styles.ColorHex(styles.ColorPrimary), styles.DraculaTheme().Primary) {
		t.Errorf("expected ColorPrimary %q, got %q", styles.DraculaTheme().Primary, styles.ColorHex(styles.ColorPrimary))
	}

	// Restore.
	styles.Apply(styles.DefaultTheme())
	if styles.ColorPrimary != orig {
		t.Errorf("expected ColorPrimary restored to %q, got %q", styles.ColorHex(orig), styles.ColorHex(styles.ColorPrimary))
	}
}

func TestApply_UpdatesProgressGradStops(t *testing.T) {
	styles.Apply(styles.DraculaTheme())
	d := styles.DraculaTheme()
	if !strings.EqualFold(styles.ColorHex(styles.ProgressGradStops[0]), d.Progress) {
		t.Errorf("ProgressGradStops[0] = %q, want %q", styles.ColorHex(styles.ProgressGradStops[0]), d.Progress)
	}
	styles.Apply(styles.DefaultTheme())
}

func TestApply_UpdatesGlowPalette(t *testing.T) {
	styles.Apply(styles.NordTheme())
	n := styles.NordTheme()
	if !strings.EqualFold(styles.ColorHex(styles.GlowPalette[0]), n.GlowPalette[0]) {
		t.Errorf("GlowPalette[0] = %q, want %q", styles.ColorHex(styles.GlowPalette[0]), n.GlowPalette[0])
	}
	styles.Apply(styles.DefaultTheme())
}

func TestApplyArtworkPalette_AndClear(t *testing.T) {
	styles.Apply(styles.DefaultTheme())
	defStop0 := styles.ColorHex(styles.ProgressGradStops[0])

	styles.ApplyArtworkPalette("#090d0e", "#cde0f2", "#acc1e1", "#a5b5c4", "#8b9db7")
	if got := styles.ColorHex(styles.ProgressGradStops[0]); got != "#cde0f2" {
		t.Errorf("ProgressGradStops[0] after ApplyArtworkPalette = %q, want #cde0f2", got)
	}
	if got := styles.ColorHex(styles.ProgressGradStops[1]); got != "#acc1e1" {
		t.Errorf("ProgressGradStops[1] after ApplyArtworkPalette = %q, want #acc1e1", got)
	}

	styles.ClearArtworkPalette()
	if got := styles.ColorHex(styles.ProgressGradStops[0]); got != defStop0 {
		t.Errorf("ProgressGradStops[0] after ClearArtworkPalette = %q, want %q", got, defStop0)
	}
}

func TestApplyArtworkPalette_SkipsLowContrastAndLeavesSharedArtistUntouched(t *testing.T) {
	styles.Apply(styles.DefaultTheme())
	defPrimary := styles.ColorHex(styles.NowPlayingArtist.GetForeground())
	defStop0 := styles.ColorHex(styles.ProgressGradStops[0])

	// Pale cover palette: near-black text colors have poor contrast on dark ColorBg.
	styles.ApplyArtworkPalette("#f5f5f5", "#111214", "#18191c", "#202225", "#25272a")
	if got := styles.ColorHex(styles.ProgressGradStops[0]); got != defStop0 {
		t.Errorf("ProgressGradStops[0] with low-contrast palette = %q, want theme default %q", got, defStop0)
	}

	// High-contrast palette tints NowPlayingTrackArtist & NowPlayingTime without mutating NowPlayingArtist.
	styles.ApplyArtworkPalette("#090d0e", "#cde0f2", "#acc1e1", "", "")
	if got := styles.ColorHex(styles.NowPlayingArtist.GetForeground()); got != defPrimary {
		t.Errorf("NowPlayingArtist foreground = %q, want unchanged %q", got, defPrimary)
	}
	if got := styles.ColorHex(styles.NowPlayingTrackArtist.GetForeground()); got != "#cde0f2" {
		t.Errorf("NowPlayingTrackArtist foreground = %q, want #cde0f2", got)
	}
	if got := styles.ColorHex(styles.NowPlayingTime.GetForeground()); got != "#acc1e1" {
		t.Errorf("NowPlayingTime foreground = %q, want #acc1e1", got)
	}
	styles.ClearArtworkPalette()
}

// The black and white theme's own colours are greys; the song's colours
// stay what they are.
func TestMonoTheme(t *testing.T) {
	th, err := styles.LoadTheme("mono", t.TempDir())
	if err != nil {
		t.Fatalf("mono: %v", err)
	}
	isGrey := func(c color.Color) bool {
		r, g, b, _ := c.RGBA()
		return r == g && g == b
	}
	fields := map[string]string{"primary": th.Primary, "secondary": th.Secondary, "muted": th.Muted, "error": th.Error,
		"fg": th.Fg, "subtle": th.Subtle, "bg": th.Bg, "love": th.Love, "active": th.Active, "progress": th.Progress,
		"surface": th.Surface, "accent": th.Accent, "accent_warm": th.AccentWarm, "bear": th.Bear, "mode_normal_bg": th.ModeNormalBg,
		"mode_search_bg": th.ModeSearchBg, "mode_command_bg": th.ModeCommandBg, "mode_chip_fg": th.ModeChipFg,
		"border": th.Border, "border_focus": th.BorderFocus}
	for i, hex := range th.GlowPalette {
		fields["glow "+strconv.Itoa(i)] = hex
	}
	for name, hex := range fields {
		if hex == "" || !isGrey(lipgloss.Color(hex)) {
			t.Errorf("%s is %q, not a grey", name, hex)
		}
	}
	// The song's colours stay colours: it is a theme like the others.
	styles.Apply(th)
	t.Cleanup(func() { styles.Apply(styles.SystemTheme()) })
	styles.ApplyCoverColours([]string{"#ff3366", "#33aaff"})
	coloured := false
	for _, c := range styles.ProgressGradStops {
		coloured = coloured || !isGrey(c)
	}
	if !coloured {
		t.Error("the progress line keeps the cover's colours under the black and white theme")
	}
}
