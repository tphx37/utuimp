package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/tphx37/utuimp/internal/assets"
)

func TestPickTerminalPrefersCurrentAndSetsAppID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix terminals")
	}
	bin := t.TempDir()
	for _, name := range []string{"kitty", "foot"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o700); err != nil { //nolint:gosec // a fake program: it must run
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("UTUIMP_TERMINAL", "")
	t.Setenv("KITTY_PID", "")
	t.Setenv("TERM", "foot")

	l, _, err := pickTerminal()
	if err != nil || l.bin != "foot" {
		t.Fatalf("in foot: got %q, %v", l.bin, err)
	}
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("KITTY_PID", "1")
	l, _, _ = pickTerminal()
	if l.bin != "kitty" {
		t.Fatalf("in kitty: got %q", l.bin)
	}
	args := l.args([]string{"/bin/utuimp"})
	if !slices.Contains(args, assets.AppID) || args[len(args)-1] != "/bin/utuimp" {
		t.Errorf("kitty args = %v, want --class %s … /bin/utuimp", args, assets.AppID)
	}
	t.Setenv("UTUIMP_TERMINAL", "foot")
	if l, _, _ = pickTerminal(); l.bin != "foot" {
		t.Errorf("UTUIMP_TERMINAL should win, got %q", l.bin)
	}
}
