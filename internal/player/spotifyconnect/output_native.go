//go:build (darwin || windows) && cgo

package spotifyconnect

/*
#cgo darwin LDFLAGS: -framework AudioToolbox -framework CoreFoundation
#cgo windows LDFLAGS: -lwinmm
#include "pcm_output.h"
*/
import "C"

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/tphx37/utuimp/internal/player/tap"
)

// nativeOutput gives librespot the same device-clock PCM tap as the other
// services, without recording system audio or starting another program.
type nativeOutput struct {
	mu         sync.Mutex
	handle     *C.utuimp_pcmout
	source     io.Reader
	done       chan struct{}
	closed     sync.Once
	wg         sync.WaitGroup
	held       bool
	skip       int
	generation uint64
}

func newOutput(r io.Reader) (audioOut, error) {
	handle := C.utuimp_pcmout_create()
	if handle == nil {
		return nil, errors.New("cannot open native Spotify audio output")
	}
	out := &nativeOutput{handle: handle, source: r, done: make(chan struct{})}
	tap.Add(out.readTap)
	out.wg.Add(1)
	go out.run()
	return out, nil
}

func (out *nativeOutput) run() {
	defer out.wg.Done()
	buffer := make([]byte, 8192)
	for {
		out.mu.Lock()
		generation := out.generation
		out.mu.Unlock()
		n, err := io.ReadFull(out.source, buffer)
		// Ignore an incomplete stereo frame, even at EOF.
		n -= n % 4
		if n > 0 {
			for {
				out.mu.Lock()
				if out.handle == nil {
					out.mu.Unlock()
					return
				}
				drop := out.held || generation != out.generation
				if !drop && out.skip > 0 {
					cut := min(n, out.skip)
					out.skip -= cut
					n -= cut
					copy(buffer, buffer[cut:cut+n])
				}
				status := C.int(1)
				if !drop && n > 0 {
					status = C.utuimp_pcmout_write(out.handle, (*C.char)(unsafe.Pointer(&buffer[0])), C.int(n))
				}
				out.mu.Unlock()
				if status < 0 {
					return
				}
				if status > 0 {
					break
				}
				select {
				case <-out.done:
					return
				case <-time.After(5 * time.Millisecond):
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (out *nativeOutput) setPaused(paused bool) {
	out.mu.Lock()
	defer out.mu.Unlock()
	if out.handle == nil {
		return
	}
	value := C.int(0)
	if paused {
		value = 1
	}
	C.utuimp_pcmout_pause(out.handle, value)
}
func (out *nativeOutput) Play()  { out.setPaused(false) }
func (out *nativeOutput) Pause() { out.setPaused(true) }
func (out *nativeOutput) Flush() {
	out.mu.Lock()
	defer out.mu.Unlock()
	out.generation++
	if out.handle != nil {
		C.utuimp_pcmout_flush(out.handle)
	}
}
func (out *nativeOutput) Hold() {
	out.mu.Lock()
	defer out.mu.Unlock()
	out.held = true
	out.generation++
	if out.handle != nil {
		C.utuimp_pcmout_pause(out.handle, 1)
		C.utuimp_pcmout_flush(out.handle)
	}
}
func (out *nativeOutput) Release(play bool, skip int) {
	out.mu.Lock()
	defer out.mu.Unlock()
	out.held = false
	out.skip = max(0, skip+3) / 4 * 4
	out.generation++
	if out.handle != nil {
		C.utuimp_pcmout_flush(out.handle)
		paused := C.int(1)
		if play {
			paused = 0
		}
		C.utuimp_pcmout_pause(out.handle, paused)
	}
}
func (out *nativeOutput) readTap(dst []float32) (int, float64, int64) {
	if len(dst) == 0 {
		return 0, 0, 0
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	if out.handle == nil || out.held {
		return 0, 0, 0
	}
	var ahead C.double
	var end C.int64_t
	rate := C.utuimp_pcmout_read(out.handle, (*C.float)(unsafe.Pointer(&dst[0])), C.int(len(dst)), &ahead, &end)
	return int(rate), float64(ahead), int64(end)
}
func (out *nativeOutput) Close() {
	out.closed.Do(func() {
		close(out.done)
		if source, ok := out.source.(io.Closer); ok {
			_ = source.Close()
		}
		out.wg.Wait()
		out.mu.Lock()
		if out.handle != nil {
			C.utuimp_pcmout_destroy(out.handle)
			out.handle = nil
		}
		out.mu.Unlock()
	})
}
func shrinkPipe(*os.File) {}
func pipePending(file *os.File) int {
	if file == nil {
		return 0
	}
	return int(C.utuimp_pcmout_pending(C.uintptr_t(file.Fd())))
}
