package styles

import (
	"image/color"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestConsoleText_Attributes(t *testing.T) {
	consolePal = ConsolePalette(DefaultTheme())
	for _, tc := range []struct{ in, want string }{
		{"\x1b[1;3;4;35mx\x1b[m", "\x1b[35mx\x1b[m"}, // bold/italic/underline dropped
		{"\x1b[3mx\x1b[0m", "x\x1b[0m"},              // nothing left: sequence dropped
		{"\x1b[95mx", "\x1b[1;35mx"},                 // bright → bold + base
		{"\x1b[38;5;13mx", "\x1b[1;35mx"},            // 256 palette index < 16
		{"\x1b[48;5;9mx", "\x1b[41mx"},               // bright bg → normal bg
		{"\x1b[103mx", "\x1b[43mx"},                  // bright bg → normal bg
		{"\x1b[38;2;26;27;38mx", "\x1b[1;30mx"},      // the bg colour as text → muted, never invisible
		{"\x1b[48;2;26;27;38mx", "\x1b[40mx"},        // truecolor bg → nearest of 0–7
		{"\x1b[38:2::198:120:221mx", "\x1b[35mx"},    // colon form → primary slot
		{"\x1b[2Ja", "\x1b[2Ja"},                     // other CSI untouched
	} {
		if got := ConsoleText(tc.in); got != tc.want {
			t.Errorf("ConsoleText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestConsoleText_GlyphsKeepWidth(t *testing.T) {
	for r, rep := range consoleGlyphs {
		if w, rw := ansi.StringWidth(string(r)), ansi.StringWidth(rep); w != rw {
			t.Errorf("%q (width %d) → %q (width %d)", r, w, rep, rw)
		}
	}
	if got := ConsoleText("╭──╮ ⠋ ♪"); got != "┌──┐ | ♪" {
		t.Errorf("got %q", got)
	}
}

func TestConsolePalette(t *testing.T) {
	pal := ConsolePalette(DefaultTheme())
	if pal[0] != (color.RGBA{0x1a, 0x1b, 0x26, 0xff}) || pal[5] != (color.RGBA{0xc6, 0x78, 0xdd, 0xff}) {
		t.Errorf("palette slots don't follow the theme: %v", pal)
	}
	// The system theme has no colours of its own: classic's are used.
	if ConsolePalette(SystemTheme()) != pal {
		t.Error("system theme should get the classic palette")
	}
	seq := consolePaletteSeq(pal)
	if want := "\x1b]P01a1b26"; seq[:len(want)] != want {
		t.Errorf("palette sequence starts %q", seq[:len(want)])
	}
}
