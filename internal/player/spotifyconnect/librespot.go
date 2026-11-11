package spotifyconnect

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	out  audioOut // plays librespot's sound; nil when librespot plays it itself
	pipe *os.File // our end of the pipe the sound comes through

	events chan Event // librespot's playback events; nil without them
}

// Event is a playback event librespot reported ("playing", "paused",
// "seeked", "loading", …). Pending is how many bytes of sound were waiting
// in the pipe when it came: those were written before it.
type Event struct {
	Name    string
	Pending int
}

// eventTag starts the lines the event script writes to librespot's stderr.
const eventTag = "utuimp-event "

// eventScript is run by librespot (--onevent) on every playback event; it
// writes the event to stderr, which utuimp reads. stdout is the sound.
const eventScript = "#!/bin/sh\nprintf '" + eventTag + "%s\\n' \"$PLAYER_EVENT\" >&2\n"

// writeEventScript puts the event script in dir and returns its path, or ""
// when it can't be used (librespot splits the command at spaces).
func writeEventScript(dir string) string {
	path := filepath.Join(dir, "onevent.sh")
	if strings.ContainsAny(path, " \t\n") {
		return ""
	}
	if err := os.WriteFile(path, []byte(eventScript), 0o700); err != nil { //nolint:gosec // our script, run by librespot
		return ""
	}
	_ = os.Chmod(path, 0o700) //nolint:gosec // see above
	return path
}

// Events delivers librespot's playback events while it runs (nil when they
// are not being followed: librespot plays the sound itself).
func (l *Librespot) Events() <-chan Event {
	if l == nil {
		return nil
	}
	return l.events
}

// readLog copies librespot's stderr to the log and passes the events on.
func (l *Librespot) readLog(r io.ReadCloser, log io.WriteCloser) {
	defer func() {
		_ = r.Close()
		if log != nil {
			_ = log.Close()
		}
		if l.events != nil {
			close(l.events)
		}
	}()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if name, ok := strings.CutPrefix(line, eventTag); ok {
			if l.events != nil {
				ev := Event{Name: strings.TrimSpace(name), Pending: pipePending(l.pipe)}
				select {
				case l.events <- ev:
				default: // nobody listening fast enough: drop, never block librespot
				}
			}
			continue
		}
		if log != nil {
			_, _ = io.WriteString(log, line+"\n")
		}
	}
}

// audioOut plays the sound librespot writes to its pipe. utuimp plays it
// itself where it can (Linux, through GStreamer): the sound then goes the
// same way as every other service's, the visualizer hears it ahead of time
// like theirs, and pause is instant. Play and Pause are for the commands
// given in utuimp; Flush drops what has been read but not played yet, after
// a skip or a seek.
type audioOut interface {
	Output
	Close()
}

// Output plays librespot's sound in utuimp. Hold and Release cover a skip
// or a seek: Hold silences it and drops what librespot still sends of the
// old place, Release goes on (playing or paused) with what comes next,
// dropping skip bytes more first.
type Output interface {
	Play()
	Pause()
	Flush()
	Hold()
	Release(play bool, skip int)
}

// Output is what plays librespot's sound, or nil when librespot does.
func (l *Librespot) Output() Output {
	if l == nil || l.out == nil {
		return nil
	}
	return l.out
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
		// The same volume scale as every other service (GStreamer's volume is
		// linear in amplitude), and no loudness normalisation, which the
		// others don't do either: 50% sounds the same on every tab.
		"--volume-ctrl", "linear",
	}
	if token != "" {
		args = append(args, "--access-token", token)
	}
	// The sound: through a pipe into utuimp's own player when there is one
	// (UTUIMP_SPOTIFY_OUTPUT=librespot keeps it in librespot).
	var out audioOut
	var pr, pw *os.File
	if os.Getenv("UTUIMP_SPOTIFY_OUTPUT") != "librespot" {
		var err error
		if pr, pw, err = os.Pipe(); err == nil {
			shrinkPipe(pw)
			if out, err = newOutput(pr); err != nil {
				_, _ = pr.Close(), pw.Close()
				out, pr, pw = nil, nil, nil
			}
		}
	}
	var events chan Event
	if out != nil {
		args = append(args, "--backend", "pipe", "--format", "S16")
		// Its events tell when a skip or a seek has reached the sound (see
		// Player.followEvents).
		if script := writeEventScript(cacheDir); script != "" {
			args = append(args, "--onevent", script)
			events = make(chan Event, 32)
		}
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // the user's librespot
	var logFile *os.File
	if logPath != "" {
		_ = os.MkdirAll(filepath.Dir(logPath), 0o700)
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil { //nolint:gosec // our log
			logFile = f
			cmd.Stdout, cmd.Stderr = f, f
		}
	}
	if out != nil {
		cmd.Stdout = pw // the sound; the log is stderr alone
	}
	// With events, stderr comes through utuimp: the event lines are taken
	// out and the rest goes to the log.
	var er, ew *os.File
	if events != nil {
		var err error
		if er, ew, err = os.Pipe(); err == nil {
			cmd.Stderr = ew
		} else {
			events = nil
		}
	}
	err := cmd.Start()
	if pw != nil {
		_ = pw.Close() // librespot holds its own copy
	}
	if ew != nil {
		_ = ew.Close()
	}
	if err != nil {
		if out != nil {
			_ = pr.Close()
			out.Close()
		}
		if er != nil {
			_ = er.Close()
		}
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, fmt.Errorf("starting librespot: %w", err)
	}
	l := &Librespot{cmd: cmd, done: make(chan struct{}), out: out, pipe: pr, events: events}
	if er != nil {
		var lw io.WriteCloser
		if logFile != nil {
			lw = logFile
		}
		go l.readLog(er, lw)
	} else if logFile != nil {
		_ = logFile.Close() // librespot holds its own copy
	}
	if out == nil {
		enableCapture() // the visualizer listens in instead, see capture.go
	}
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
	stopCapture()
	_ = l.cmd.Process.Signal(os.Interrupt)
	select {
	case <-l.done:
	case <-time.After(2 * time.Second):
		_ = l.cmd.Process.Kill()
		<-l.done
	}
	if l.out != nil {
		_ = l.pipe.Close()
		l.out.Close()
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
