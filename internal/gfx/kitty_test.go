package gfx

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func env(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }

func TestDetect(t *testing.T) {
	shmDir = t.TempDir()
	for _, c := range []struct {
		name    string
		env     map[string]string
		setting string
		want    Mode
	}{
		{"kitty", map[string]string{"KITTY_WINDOW_ID": "1"}, "", Shm},
		{"kitty turned off", map[string]string{"KITTY_WINDOW_ID": "1"}, "off", Off},
		{"kitty over ssh", map[string]string{"TERM": "xterm-kitty", "SSH_TTY": "/dev/pts/1"}, "", Direct},
		{"kitty in tmux", map[string]string{"KITTY_WINDOW_ID": "1", "TMUX": "x"}, "on", Off},
		{"ghostty", map[string]string{"TERM": "xterm-ghostty"}, "", Shm},
		{"unknown", map[string]string{"TERM": "xterm-256color"}, "", Off},
		{"unknown, forced", map[string]string{"TERM": "xterm-256color"}, "on", Shm},
	} {
		if got := Detect(env(c.env), c.setting); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFrameShm(t *testing.T) {
	shmDir = t.TempDir()
	im := &Image{ID: 7, Z: -1}
	rgb := make([]byte, 4*2*3)
	seq, err := im.Frame(Shm, rgb, 4, 2, 0, 1, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\x1b[2;1H", "a=T,f=24,s=4,v=2,i=7,p=1,c=80,r=24,z=-1,C=1,q=2,t=s;", "\x1b7", "\x1b8"} {
		if !strings.Contains(seq, want) {
			t.Errorf("missing %q in %q", want, seq)
		}
	}
	at := strings.Index(seq, "t=s;") + 4
	name, _ := base64.StdEncoding.DecodeString(seq[at : at+strings.Index(seq[at:], "\x1b\\")])
	if !strings.HasPrefix(string(name), "/") {
		t.Errorf("kitty wants the name with a leading slash: %q", name)
	}
	first := filepath.Join(shmDir, string(name))
	if st, err := os.Stat(first); err != nil || st.Size() != int64(len(rgb)) {
		t.Fatalf("the frame should be in shared memory: %v", err)
	}
	// Frames the terminal never read don't pile up.
	for range keepFrames + 3 {
		if _, err := im.Frame(Shm, rgb, 4, 2, 0, 0, 80, 24); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(first); err == nil {
		t.Error("an old frame should be removed")
	}
	if left, _ := os.ReadDir(shmDir); len(left) != keepFrames {
		t.Errorf("%d frames kept, want %d", len(left), keepFrames)
	}
	if del := im.Delete(); !strings.Contains(del, "a=d,d=I,i=7") {
		t.Errorf("delete = %q", del)
	}
	if left, _ := os.ReadDir(shmDir); len(left) != 0 {
		t.Errorf("%d files left in shared memory", len(left))
	}
}

func TestFrameDirectChunks(t *testing.T) {
	im := &Image{ID: 1}
	rgb := make([]byte, 100*40*3) // 16000 base64 characters: four chunks
	seq, err := im.Frame(Direct, rgb, 100, 40, 0, 0, 10, 4)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(seq, "\x1b_G"); n != 4 {
		t.Errorf("%d chunks, want 4", n)
	}
	if !strings.Contains(seq, "t=d,m=1;") || !strings.Contains(seq, "\x1b_Gm=0;") {
		t.Error("the first chunk says more follows, the last one ends it")
	}
	if seq, _ := im.Frame(Off, rgb, 100, 40, 0, 0, 10, 4); seq != "" {
		t.Error("nothing is sent when pictures are off")
	}
}

func TestCleanStale(t *testing.T) {
	shmDir = t.TempDir()
	// A process that is running (this one's parent) keeps its frames; this
	// one's own, at its start, are from before it restarted in place.
	mine := filepath.Join(shmDir, "utuimp-"+strconv.Itoa(os.Getppid())+"-71-3")
	own := filepath.Join(shmDir, "utuimp-"+strconv.Itoa(os.Getpid())+"-71-4")
	dead := filepath.Join(shmDir, "utuimp-2147483646-71-9")
	other := filepath.Join(shmDir, "something-else")
	for _, f := range []string{mine, own, dead, other} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	CleanStale()
	if _, err := os.Stat(dead); err == nil {
		t.Error("a dead process's frame should go")
	}
	if _, err := os.Stat(own); err == nil {
		t.Error("this process's frame from before its restart should go")
	}
	for _, f := range []string{mine, other} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s should stay", filepath.Base(f))
		}
	}
}
