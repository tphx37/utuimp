//go:build linux || darwin

package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCaptureStderrSendsNativeWritesToLog(t *testing.T) {
	cache := t.TempDir()
	if runtime.GOOS == "darwin" {
		t.Setenv("HOME", cache)
		cache = filepath.Join(cache, "Library", "Caches")
	} else {
		t.Setenv("XDG_CACHE_HOME", cache)
	}
	restore := captureStderr()
	// What a C library does: write straight to descriptor 2.
	_, _ = unix.Write(2, []byte("'impl->recurse > 0' failed\n"))
	restore()

	data, err := os.ReadFile(filepath.Join(cache, "utuimp", "stderr.log")) //nolint:gosec // under the test's own cache directory
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "impl->recurse") {
		t.Errorf("log = %q, want the native write", data)
	}
}
