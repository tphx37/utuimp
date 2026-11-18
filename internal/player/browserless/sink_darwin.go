//go:build darwin && cgo

package browserless

/*
#cgo darwin CXXFLAGS: -std=c++17
#cgo darwin LDFLAGS: -framework AudioToolbox -framework CoreFoundation -lc++
#include "sink_darwin.h"
#include <stdlib.h>

extern void utuimpDarwinOnEOS(uintptr_t userData);
extern void utuimpDarwinOnError(uintptr_t userData, char* errMsg);
extern void utuimpDarwinOnStreamStart(uintptr_t userData, char* uri);

static inline utuimp_darwinsink_t utuimp_create_darwin_sink(uintptr_t userData) {
    return utuimp_darwinsink_create(userData, utuimpDarwinOnEOS, utuimpDarwinOnError, utuimpDarwinOnStreamStart);
}
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"

	"github.com/tphx37/utuimp/internal/player/tap"
)

type darwinAudioSink struct {
	mu            sync.Mutex
	handle        C.utuimp_darwinsink_t
	cgoH          cgo.Handle
	onEOS         func()
	onError       func(error)
	onStreamStart func(string)
}

func newAudioSink() (audioSink, error) {
	s := &darwinAudioSink{}
	h := cgo.NewHandle(s)
	s.cgoH = h

	playerHandle := C.utuimp_create_darwin_sink(C.uintptr_t(h))
	if playerHandle == nil {
		h.Delete()
		return nil, errors.New("failed to create Darwin audio sink")
	}
	s.handle = playerHandle
	tap.Add(s.readTap)
	return s, nil
}

func sinkFromHandle(userData C.uintptr_t) (s *darwinAudioSink) {
	defer func() { _ = recover() }()
	s, _ = cgo.Handle(userData).Value().(*darwinAudioSink)
	return s
}

//export utuimpDarwinOnEOS
func utuimpDarwinOnEOS(userData C.uintptr_t) {
	s := sinkFromHandle(userData)
	if s == nil {
		return
	}
	s.mu.Lock()
	cb := s.onEOS
	s.mu.Unlock()
	if cb != nil {
		go cb()
	}
}

//export utuimpDarwinOnError
func utuimpDarwinOnError(userData C.uintptr_t, cErrMsg *C.char) {
	msg := C.GoString(cErrMsg)
	s := sinkFromHandle(userData)
	if s == nil {
		return
	}
	s.mu.Lock()
	cb := s.onError
	s.mu.Unlock()
	if cb != nil {
		go cb(errors.New(msg))
	}
}

//export utuimpDarwinOnStreamStart
func utuimpDarwinOnStreamStart(userData C.uintptr_t, cURI *C.char) {
	uri := C.GoString(cURI)
	s := sinkFromHandle(userData)
	if s == nil {
		return
	}
	s.mu.Lock()
	cb := s.onStreamStart
	s.mu.Unlock()
	if cb != nil {
		go cb(uri)
	}
}

func (s *darwinAudioSink) PlayURI(uri string) {
	cs := C.CString(uri)
	defer C.free(unsafe.Pointer(cs))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.utuimp_darwinsink_play_uri(s.handle, cs)
	}
}

func (s *darwinAudioSink) SetNextURI(uri string) {
	var cs *C.char
	if uri != "" {
		cs = C.CString(uri)
		defer C.free(unsafe.Pointer(cs))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.utuimp_darwinsink_set_next_uri(s.handle, cs)
	}
}

func (s *darwinAudioSink) OnStreamStart(fn func(string)) {
	s.mu.Lock()
	s.onStreamStart = fn
	s.mu.Unlock()
}
func (s *darwinAudioSink) Play() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.utuimp_darwinsink_play(s.handle)
	}
}

func (s *darwinAudioSink) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.utuimp_darwinsink_pause(s.handle)
	}
}

func (s *darwinAudioSink) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.utuimp_darwinsink_stop(s.handle)
	}
}

func (s *darwinAudioSink) Seek(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.utuimp_darwinsink_seek(s.handle, C.double(d.Seconds()))
	}
}

func (s *darwinAudioSink) SetVolume(v float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.utuimp_darwinsink_set_volume(s.handle, C.float(v))
	}
}

func (s *darwinAudioSink) SetEqualizer(gains []float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == nil {
		return
	}
	if len(gains) == 0 {
		C.utuimp_darwinsink_set_eq(s.handle, nil, 0)
		return
	}
	cg := make([]C.double, len(gains))
	for i, g := range gains {
		cg[i] = C.double(g)
	}
	C.utuimp_darwinsink_set_eq(s.handle, &cg[0], C.int(len(cg)))
}

func (s *darwinAudioSink) Position() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		sec := C.utuimp_darwinsink_position(s.handle)
		return time.Duration(float64(sec) * float64(time.Second))
	}
	return 0
}

func (s *darwinAudioSink) OnEOS(fn func()) {
	s.mu.Lock()
	s.onEOS = fn
	s.mu.Unlock()
}

func (s *darwinAudioSink) OnError(fn func(error)) {
	s.mu.Lock()
	s.onError = fn
	s.mu.Unlock()
}

func (s *darwinAudioSink) Destroy() {
	s.mu.Lock()
	handle := s.handle
	s.handle = nil
	s.mu.Unlock()
	if handle != nil {
		C.utuimp_darwinsink_destroy(handle)
		s.cgoH.Delete()
	}
}

func (s *darwinAudioSink) readTap(dst []float32) (int, float64, int64) {
	if len(dst) == 0 {
		return 0, 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == nil {
		return 0, 0, 0
	}
	var ahead C.double
	var end C.int64_t
	rate := C.utuimp_darwinsink_read_tap(s.handle, (*C.float)(unsafe.Pointer(&dst[0])), C.int(len(dst)), &ahead, &end)
	return int(rate), float64(ahead), int64(end)
}
