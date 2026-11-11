// Package tap hands the audio being played to the visualizer. A backend that
// can see its own PCM (the GStreamer one) registers a reader; one that has
// to listen in on another program (Spotify's librespot) adds a second one.
// When neither has anything the visualizer animates on its own.
package tap

import (
	"sync"
	"sync/atomic"
)

// Reader fills dst with mono samples in [-1, 1] and returns their sample
// rate, or 0 when nothing is playing. ahead tells where dst ends: that many
// seconds after the sound coming out of the speakers right now. A backend
// that holds the sound before it is played can hand out a little of the
// future, which lets the visualizer show a hit at the very moment it sounds
// instead of after it has found it.
// end counts the samples of the stream up to the end of dst, so a caller
// can tell exactly how much sound passed between two reads.
type Reader func(dst []float32) (rate int, ahead float64, end int64)

var reader atomic.Pointer[Reader]

// Register installs the sample reader.
func Register(r Reader) { reader.Store(&r) }

var (
	extraMu sync.Mutex
	extra   []Reader
)

// Add installs one more reader, asked when the registered one has nothing.
func Add(r Reader) {
	extraMu.Lock()
	extra = append(extra, r)
	extraMu.Unlock()
}

// Read fills dst (see Reader) and returns the rate, or 0 when no backend
// provides samples or nothing is playing.
func Read(dst []float32) (rate int, ahead float64, end int64) {
	if r := reader.Load(); r != nil {
		if rate, ahead, end := (*r)(dst); rate > 0 {
			return rate, ahead, end
		}
	}
	extraMu.Lock()
	rs := extra
	extraMu.Unlock()
	for _, r := range rs {
		if rate, ahead, end := r(dst); rate > 0 {
			return rate, ahead, end
		}
	}
	return 0, 0, 0
}
