//go:build !windows

package spotifyconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/provider/spotify"
)

func TestStartLibrespotAndWaitForDevice(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "librespot")
	// A stand-in librespot: records its arguments and keeps running.
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\nexec /bin/sleep 30\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := FindLibrespot(bin); err != nil || got != bin {
		t.Fatalf("FindLibrespot = %q, %v", got, err)
	}

	ls, err := StartLibrespot(bin, "tok123", filepath.Join(dir, "cache"), filepath.Join(dir, "ls.log"), 320)
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Stop()

	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The device shows up on the second poll.
		if polls.Add(1) < 2 {
			_, _ = w.Write([]byte(`{"devices":[{"id":"phone","name":"Phone"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"devices":[{"id":"phone","name":"Phone"},{"id":"dev-u","name":"utuimp","type":"Computer"}]}`))
	}))
	defer srv.Close()
	c := spotify.NewClient("cid", spotify.Token{AccessToken: "at"}, nil)
	c.SetBaseURL(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, err := WaitForDevice(ctx, c, ls)
	if err != nil || id != "dev-u" {
		t.Fatalf("device = %q, %v", id, err)
	}

	raw, _ := os.ReadFile(argsFile) //nolint:gosec // test file
	args := string(raw)
	for _, want := range []string{"--name utuimp", "--access-token tok123", "--disable-discovery", "--bitrate 320", "--device-type computer"} {
		if !strings.Contains(args, want) {
			t.Errorf("librespot args %q missing %q", args, want)
		}
	}
	ls.Stop()
	if !ls.Exited() {
		t.Error("Stop should end librespot")
	}
}

func TestFindLibrespotMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := FindLibrespot(""); err != ErrNoLibrespot {
		t.Errorf("err = %v", err)
	}
}
