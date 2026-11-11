package spotifyconnect

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/player/tap"
)

// librespot plays the sound itself and doesn't share it. For the visualizer
// utuimp listens in on it: PulseAudio and PipeWire can record what one
// program plays (and nothing else) with "parec --monitor-stream". The
// recorder runs only while somebody reads the samples, that is, while the
// visualizer is open on a playing Spotify tab. Where the tools are missing
// (macOS, Windows, bare ALSA) nothing is captured and the visualizer keeps
// its own beat.

const (
	captureRate = 44100
	captureLen  = 1 << 16
	// captureIdle is how long after the last read the recorder stops.
	captureIdle = 5 * time.Second
	// captureStale is how old the newest sample may be to count as "now".
	captureStale = 400 * time.Millisecond
)

type capture struct {
	mu      sync.Mutex
	ring    [captureLen]float32
	written uint64
	last    time.Time // of the newest sample
	wanted  time.Time // of the last read
	running bool
	cmd     *exec.Cmd
	off     bool // the tools are missing: don't try again
}

var (
	theCapture  capture
	captureOnce sync.Once
)

// enableCapture offers librespot's sound to the visualizer.
func enableCapture() {
	captureOnce.Do(func() { tap.Add(theCapture.read) })
}

// stopCapture ends the recorder, if it runs (librespot is going away).
func stopCapture() {
	c := &theCapture
	c.mu.Lock()
	c.wanted = time.Time{}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	c.mu.Unlock()
}

// captureAhead is how far ahead of the speakers the recording runs: it is
// taken where the sound server mixes the stream, one hardware period before
// it is heard.
const captureAhead = 0.02

// read fills dst with the newest samples and returns their rate, or 0 when
// there are none. Asking is what keeps the recorder running.
func (c *capture) read(dst []float32) (int, float64, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.off {
		return 0, 0, 0
	}
	c.wanted = time.Now()
	if !c.running {
		c.running = true
		go c.run()
	}
	n := uint64(len(dst))
	if c.written < n || n > captureLen/2 || time.Since(c.last) > captureStale {
		return 0, 0, 0
	}
	for i := range dst {
		dst[i] = c.ring[(c.written-n+uint64(i))%captureLen] //nolint:gosec // i >= 0
	}
	return captureRate, captureAhead, int64(c.written) //nolint:gosec // far below the limit
}

func (c *capture) idle() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Since(c.wanted) > captureIdle
}

// run records librespot's stream for as long as the samples are read.
func (c *capture) run() {
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()
	parec, err1 := exec.LookPath("parec")
	pactl, err2 := exec.LookPath("pactl")
	if err1 != nil || err2 != nil {
		c.mu.Lock()
		c.off = true
		c.mu.Unlock()
		return
	}
	for !c.idle() {
		idx := librespotStream(pactl)
		if idx == "" {
			// No stream yet: librespot opens it when it starts to play.
			time.Sleep(time.Second)
			continue
		}
		c.record(parec, idx)
		time.Sleep(200 * time.Millisecond)
	}
}

// record runs parec on stream idx until it ends or nobody reads any more.
func (c *capture) record(parec, idx string) {
	cmd := exec.Command(parec, "--monitor-stream="+idx, "--raw", "--format=float32le", //nolint:gosec // our own arguments
		"--rate="+strconv.Itoa(captureRate), "--channels=1", "--latency-msec=10",
		"--client-name=utuimp visualizer")
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return
	}
	c.mu.Lock()
	c.cmd = cmd
	c.mu.Unlock()
	stop := make(chan struct{})
	go func() {
		// The stream is silent while paused, so the reads below block: the
		// idle check needs a clock of its own.
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if c.idle() {
					_ = cmd.Process.Kill()
					return
				}
			}
		}
	}()
	buf := make([]byte, 4096)
	have := 0
	for {
		n, err := out.Read(buf[have:])
		have += n
		whole := have &^ 3
		if whole > 0 {
			c.mu.Lock()
			for i := 0; i < whole; i += 4 {
				c.ring[c.written%captureLen] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i:]))
				c.written++
			}
			c.last = time.Now()
			c.mu.Unlock()
			have = copy(buf, buf[whole:have])
		}
		if err != nil {
			break
		}
	}
	close(stop)
	_, _ = io.Copy(io.Discard, out)
	_ = cmd.Wait()
	c.mu.Lock()
	c.cmd = nil
	c.mu.Unlock()
}

// librespotStream is the index of the playback stream librespot has open on
// the sound server, or "" when it has none.
func librespotStream(pactl string) string {
	out, err := exec.Command(pactl, "-f", "json", "list", "sink-inputs").Output() //nolint:gosec // pactl from PATH
	if err != nil {
		return ""
	}
	return findLibrespotStream(out)
}

// findLibrespotStream picks librespot's stream out of pactl's list. The
// stream doesn't always carry the process id (through ALSA it doesn't), but
// its names do say "librespot".
func findLibrespotStream(list []byte) string {
	var inputs []struct {
		Index      int               `json:"index"`
		Properties map[string]string `json:"properties"`
	}
	if json.Unmarshal(list, &inputs) != nil {
		return ""
	}
	for _, in := range inputs {
		for _, key := range []string{"application.process.binary", "application.name", "node.name"} {
			if strings.Contains(strings.ToLower(in.Properties[key]), "librespot") {
				return strconv.Itoa(in.Index)
			}
		}
	}
	return ""
}
