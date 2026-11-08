package spotifyconnect

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tphx37/utuimp/internal/provider/spotify"
)

// DeviceName is the Spotify Connect device librespot shows up as.
const DeviceName = "utuimp"

// ErrNoLibrespot means librespot isn't installed.
var ErrNoLibrespot = errors.New("librespot is not installed")

// FindLibrespot returns the librespot executable: path if set, else from PATH.
func FindLibrespot(path string) (string, error) {
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("librespot at %s: %w", path, err)
		}
		return path, nil
	}
	p, err := exec.LookPath("librespot")
	if err != nil {
		return "", ErrNoLibrespot
	}
	return p, nil
}

// Librespot is a running librespot process: the audio engine.
type Librespot struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

// HasCredentials reports whether librespot has signed in before (it keeps
// reusable credentials in cacheDir), so it can start without a token.
func HasCredentials(cacheDir string) bool {
	_, err := os.Stat(filepath.Join(cacheDir, "credentials.json"))
	return err == nil
}

// StartLibrespot runs librespot as the "utuimp" Connect device, signed in
// with token (which must carry the "streaming" scope; "" = use the
// credentials cached in cacheDir). Its log goes to
// logPath. cacheDir keeps its credentials and volume.
func StartLibrespot(bin, token, cacheDir, logPath string, bitrate int) (*Librespot, error) {
	if bitrate != 96 && bitrate != 160 && bitrate != 320 {
		bitrate = 320
	}
	_ = os.MkdirAll(cacheDir, 0o700)
	args := []string{
		"--name", DeviceName,
		"--device-type", "computer",
		"--bitrate", fmt.Sprint(bitrate),
		"--disable-discovery",
		"--system-cache", cacheDir,
		"--disable-audio-cache",
		"--initial-volume", "100",
		"--enable-volume-normalisation",
	}
	if token != "" {
		args = append(args, "--access-token", token)
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // the user's librespot
	if logPath != "" {
		_ = os.MkdirAll(filepath.Dir(logPath), 0o700)
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil { //nolint:gosec // our log
			cmd.Stdout, cmd.Stderr = f, f
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting librespot: %w", err)
	}
	l := &Librespot{cmd: cmd, done: make(chan struct{})}
	go func() {
		l.err = cmd.Wait()
		close(l.done)
	}()
	return l, nil
}

// Exited reports whether librespot has stopped.
func (l *Librespot) Exited() bool {
	select {
	case <-l.done:
		return true
	default:
		return false
	}
}

// Stop ends librespot.
func (l *Librespot) Stop() {
	if l == nil || l.Exited() {
		return
	}
	_ = l.cmd.Process.Signal(os.Interrupt)
	select {
	case <-l.done:
	case <-time.After(2 * time.Second):
		_ = l.cmd.Process.Kill()
		<-l.done
	}
}

// WaitForDevice waits until the "utuimp" device appears in the account's
// device list and returns its ID.
func WaitForDevice(ctx context.Context, c *spotify.Client, l *Librespot) (string, error) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		var res struct {
			Devices []device `json:"devices"`
		}
		if err := c.Do(ctx, http.MethodGet, "/me/player/devices", nil, nil, &res); err == nil {
			for _, d := range res.Devices {
				if d.Name == DeviceName && d.ID != "" {
					return d.ID, nil
				}
			}
		}
		if l != nil && l.Exited() {
			return "", errors.New("librespot exited (see its log)")
		}
		select {
		case <-ctx.Done():
			return "", errors.New("the utuimp Spotify device did not appear")
		case <-tick.C:
		}
	}
}
