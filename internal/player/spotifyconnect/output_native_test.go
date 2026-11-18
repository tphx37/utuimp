//go:build (darwin || windows) && cgo

package spotifyconnect

import (
	"io"
	"testing"
	"time"
)

func TestNativeOutputTapAndControls(t *testing.T) {
	reader, writer := io.Pipe()
	output, err := newOutput(reader)
	if err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Skipf("native audio device unavailable: %v", err)
	}
	defer output.Close()
	defer func() { _ = writer.Close() }()
	go func() {
		silence := make([]byte, 8192)
		for {
			if _, err := writer.Write(silence); err != nil {
				return
			}
		}
	}()
	out := output.(*nativeOutput)
	samples := make([]float32, 8192)
	waitForPCM := func() {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if rate, ahead, end := out.readTap(samples); rate > 0 {
				if rate != 44100 || ahead < 0 || ahead > 0.181 || end < 8192 {
					t.Fatalf("invalid device-clock window: rate=%d ahead=%f end=%d", rate, ahead, end)
				}
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("native output did not provide PCM")
	}
	waitForPCM()
	out.Pause()
	if rate, _, _ := out.readTap(samples); rate != 0 {
		t.Fatal("paused output still supplies PCM")
	}
	out.Play()
	waitForPCM()
	out.Hold()
	if rate, _, _ := out.readTap(samples); rate != 0 {
		t.Fatal("held output still supplies PCM")
	}
	out.Release(true, 0)
	waitForPCM()
	closed := make(chan struct{})
	go func() { out.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("native output did not close its blocked reader")
	}
	if rate, _, _ := out.readTap(samples); rate != 0 {
		t.Fatal("closed output still supplies PCM")
	}
}
