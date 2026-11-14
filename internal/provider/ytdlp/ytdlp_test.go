package ytdlp

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickFormat(t *testing.T) {
	fs := []Format{
		{FormatID: "18", Acodec: "mp4a.40.2", Vcodec: "avc1"},
		{FormatID: "140", Acodec: "mp4a.40.2", Vcodec: "none", Abr: 129},
		{FormatID: "251", Acodec: "opus", Vcodec: "none", Abr: 160},
	}
	if id, cp, _ := PickFormat(fs); id != "140" || !cp {
		t.Errorf("got %s copy=%v, want 140 copied", id, cp)
	}
	if id, cp, _ := PickFormat(fs[2:]); id != "251" || cp {
		t.Errorf("got %s copy=%v, want 251 re-encoded", id, cp)
	}
}

func TestBotCheckError(t *testing.T) {
	err := Error("ERROR: [youtube] x: Sign in to confirm you’re not a bot.", nil)
	if err != ErrBotCheck {
		t.Errorf("err = %v", err)
	}
}

// The yt-dlp | ffmpeg pipeline turns an M4A into ADTS AAC. yt-dlp is
// replaced by a script that prints a file, so no network is needed.
func TestStreamPipelineMakesADTS(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	m4a := filepath.Join(dir, "a.m4a")
	if out, err := exec.Command(ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "sine=d=1", //nolint:gosec // ffmpeg found on the path, a fixed command
		"-c:a", "aac", m4a).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg can't encode AAC: %v %s", err, out)
	}
	fake := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec cat "+m4a+"\n"), 0o700); err != nil { //nolint:gosec // a fake program: it must run
		t.Fatal(err)
	}
	s := Streamer{YtDlp: fake, FFmpeg: ffmpeg}
	rc, err := s.Open(context.Background(), []byte(`{}`), "140", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 100 || b[0] != 0xFF || b[1]&0xF0 != 0xF0 {
		t.Fatalf("not ADTS: %d bytes, % x", len(b), b[:min(4, len(b))])
	}
}

func TestStreamPipelineReportsFailure(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	fake := filepath.Join(t.TempDir(), "yt-dlp")
	_ = os.WriteFile(fake, []byte("#!/bin/sh\necho 'ERROR: [youtube] x: Video unavailable' >&2\nexit 1\n"), 0o700) //nolint:gosec // a fake program: it must run
	rc, err := Streamer{YtDlp: fake, FFmpeg: ffmpeg}.Open(context.Background(), []byte(`{}`), "140", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.ReadAll(rc); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("err = %v, want yt-dlp's message", err)
	}
}
