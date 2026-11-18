package tap

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// Exercise the exact header used by Core Audio and waveOut, including the
// device clock, lookahead, seek origin, S16 conversion and overwritten PCM.
func TestNativePCMDeviceClock(t *testing.T) {
	cc, err := exec.LookPath("gcc")
	if err != nil {
		cc, err = exec.LookPath("cc")
	}
	if err != nil {
		t.Skip("no C compiler")
	}
	binary := filepath.Join(t.TempDir(), "pcm-test")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	args := []string{"-std=c11", "-Wall", "-Wextra", "-Werror", "-I.", "testdata/pcm.c", "-o", binary}
	if runtime.GOOS != "windows" {
		args = append(args, "-pthread", "-lm")
	}
	if output, err := exec.Command(cc, args...).CombinedOutput(); err != nil { // #nosec G204 -- compiler from PATH, fixed test source and arguments.
		t.Fatalf("compile PCM test: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary).CombinedOutput(); err != nil { // #nosec G204 -- executable just built from the fixed test source.
		t.Fatalf("PCM clock test: %v\n%s", err, output)
	}
}
