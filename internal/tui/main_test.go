package tui

import (
	"os"
	"testing"
)

// TestMain points HOME and the XDG directories at a throwaway directory, so
// nothing a test saves (the config, history, the library files) can reach
// the developer's real ~/.config/utuimp or ~/.cache/utuimp.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "utuimp-tui-test-home-*")
	if err != nil {
		panic("creating temp HOME: " + err.Error())
	}
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		if err := os.Setenv(key, tmp); err != nil {
			panic("setting temp " + key + ": " + err.Error())
		}
	}
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}
