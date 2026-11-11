package gst

import (
	"encoding/binary"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// fakeLibrespot writes a slow ramp (each sample says where in the stream it
// is) as fast as it is read, and can be made to fall silent, the way
// librespot does on its pipe when it is paused.
type fakeLibrespot struct {
	w      *io.PipeWriter
	frames atomic.Int64
	silent atomic.Bool
}

func (f *fakeLibrespot) run() {
	buf := make([]byte, 4096)
	for {
		for f.silent.Load() {
			time.Sleep(10 * time.Millisecond)
		}
		n := f.frames.Load()
		for i := 0; i < len(buf); i += 4 {
			v := uint16((n / 64) % 30000) //nolint:gosec // below 30000
			binary.LittleEndian.PutUint16(buf[i:], v)
			binary.LittleEndian.PutUint16(buf[i+2:], v)
			n++
		}
		if _, err := f.w.Write(buf); err != nil {
			return
		}
		f.frames.Store(n)
	}
}

// The raw player must take the stream at the speed it plays it, stop taking
// it while paused, carry on after the source was silent (without throwing
// the rest away as late), and hand the visualizer the sound ahead of time.
func TestRawPlayerPacesItsSource(t *testing.T) {
	if os.Getenv("UTUIMP_TEST_REAL_SINK") == "" { // set it (with PULSE_SINK naming a null sink) to try the real sink
		t.Setenv("UTUIMP_NULL_AUDIO", "1")
	}
	r, w := io.Pipe()
	src := &fakeLibrespot{w: w}
	go src.run()
	p, err := NewRaw(r)
	if err != nil {
		t.Skipf("no raw pipeline here: %v", err)
	}
	defer func() {
		_ = w.Close()
		_ = r.Close()
		p.Close()
	}()

	taken := func(d time.Duration) float64 {
		a := src.frames.Load()
		time.Sleep(d)
		return float64(src.frames.Load()-a) / 44100 / d.Seconds()
	}
	time.Sleep(time.Second) // the buffers fill up first
	if speed := taken(2 * time.Second); speed < 0.9 || speed > 1.1 {
		t.Errorf("the stream is taken at %.2f of real time, want 1", speed)
	}

	buf := make([]float32, 64)
	rate, ahead, _ := readTap(buf)
	if rate != 44100 || ahead < 0.15 {
		t.Errorf("the tap gives rate %d, %.0f ms ahead; want 44100 and about 180", rate, ahead*1000)
	}
	// What the tap shows is 180 ms ahead of what plays, which is at most
	// about a second behind what has been written.
	heard := float64(buf[len(buf)-1]) * 32768 * 64 / 44100
	if lag := float64(src.frames.Load())/44100 - heard; lag < 0 || lag > 1.2 {
		t.Errorf("the tap is %.2f s behind the source, want well under a second", lag)
	}

	p.Pause()
	time.Sleep(300 * time.Millisecond)
	if speed := taken(time.Second); speed > 0.05 {
		t.Errorf("paused, the stream is still taken at %.2f of real time", speed)
	}
	p.Play()
	time.Sleep(300 * time.Millisecond)
	if speed := taken(time.Second); speed < 0.9 || speed > 1.1 {
		t.Errorf("after the pause the stream is taken at %.2f of real time, want 1", speed)
	}

	// The source falls silent for two seconds (paused from a phone).
	src.silent.Store(true)
	time.Sleep(2 * time.Second)
	src.silent.Store(false)
	time.Sleep(500 * time.Millisecond)
	if speed := taken(2 * time.Second); speed < 0.9 || speed > 1.1 {
		t.Errorf("after the source was silent the stream is taken at %.2f of real time, want 1", speed)
	}

	// A long silence puts the player to rest; sound wakes it at once.
	src.silent.Store(true)
	time.Sleep(7 * time.Second)
	src.silent.Store(false)
	time.Sleep(1500 * time.Millisecond)
	if speed := taken(2 * time.Second); speed < 0.9 || speed > 1.1 {
		t.Errorf("after a long silence the stream is taken at %.2f of real time, want 1", speed)
	}
	if rate, ahead, _ := readTap(buf); rate != 44100 || ahead < 0.15 {
		t.Errorf("after a long silence the tap gives rate %d, %.0f ms ahead", rate, ahead*1000)
	}

	// A skip: what was in hand is dropped, and the stream goes on.
	before := src.frames.Load()
	p.Flush()
	time.Sleep(300 * time.Millisecond)
	if jump := float64(src.frames.Load()-before) / 44100; jump < 0.3 {
		t.Errorf("a flush should make room for new sound at once, only %.2f s was taken", jump)
	}
	if speed := taken(2 * time.Second); speed < 0.9 || speed > 1.1 {
		t.Errorf("after a flush the stream is taken at %.2f of real time, want 1", speed)
	}
	if rate, _, _ := readTap(buf); rate != 44100 {
		t.Errorf("after a flush the tap gives rate %d", rate)
	}

	// A skip on its way to the source: the old place is dropped at the
	// speed it would play (the source isn't raced through it, nor stopped).
	p.Hold()
	time.Sleep(200 * time.Millisecond)
	if speed := taken(time.Second); speed < 0.8 || speed > 1.2 {
		t.Errorf("held, the stream is taken at %.2f of real time, want 1", speed)
	}
	p.Play() // does nothing while held
	if !p.Holding() {
		t.Error("Play must not end a hold")
	}
	p.Release(true, 4096)
	time.Sleep(500 * time.Millisecond)
	if speed := taken(2 * time.Second); speed < 0.9 || speed > 1.1 {
		t.Errorf("after a release the stream is taken at %.2f of real time, want 1", speed)
	}
	if rate, _, _ := readTap(buf); rate != 44100 {
		t.Errorf("after a release the tap gives rate %d", rate)
	}

	// Released paused (a skip while paused): nothing is taken until Play.
	p.Hold()
	time.Sleep(100 * time.Millisecond)
	p.Release(false, 0)
	time.Sleep(400 * time.Millisecond)
	if speed := taken(time.Second); speed > 0.05 {
		t.Errorf("released paused, the stream is still taken at %.2f of real time", speed)
	}
	p.Play()
	time.Sleep(400 * time.Millisecond)
	if speed := taken(2 * time.Second); speed < 0.9 || speed > 1.1 {
		t.Errorf("played after a paused release, the stream is taken at %.2f of real time, want 1", speed)
	}
}
