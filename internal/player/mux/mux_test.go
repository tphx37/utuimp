package mux

import (
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/player"
)

type fakePlayer struct {
	player.Player
	ch     chan player.State
	paused chan struct{}
}

func newFake() *fakePlayer {
	return &fakePlayer{ch: make(chan player.State, 4), paused: make(chan struct{}, 4)}
}
func (f *fakePlayer) Subscribe() <-chan player.State { return f.ch }
func (f *fakePlayer) Pause() error                   { f.paused <- struct{}{}; return nil }

func TestStartingOneServicePausesTheOther(t *testing.T) {
	m := New(0)
	a, b := newFake(), newFake()
	m.Set(0, a)
	m.Set(1, b)
	out := m.Subscribe()

	a.ch <- player.State{Playing: true}
	<-out
	b.ch <- player.State{Playing: true}
	select {
	case <-a.paused:
	case <-time.After(time.Second):
		t.Fatal("service 0 should be paused when service 1 starts")
	}
	select {
	case st := <-out:
		if !st.Playing {
			t.Error("integrations should now follow service 1")
		}
	case <-time.After(time.Second):
		t.Fatal("no state forwarded from the new service")
	}
}
