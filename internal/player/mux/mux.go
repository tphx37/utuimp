// Package mux joins the players of several music services into one view
// for system integrations (MPRIS media keys, Discord, Last.fm): they follow
// whichever service is playing, and starting one service pauses the others.
package mux

import (
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/player"
)

// Mux implements the playback-control subset integrations need.
type Mux struct {
	mu      sync.Mutex
	players map[int]player.Player
	playing map[int]bool
	current int
	subs    []chan player.State
}

// New returns an empty mux; current is the initially focused service.
func New(current int) *Mux {
	return &Mux{players: map[int]player.Player{}, playing: map[int]bool{}, current: current}
}

// Set registers the player of service idx and starts following it.
func (m *Mux) Set(idx int, p player.Player) {
	m.mu.Lock()
	m.players[idx] = p
	m.mu.Unlock()
	ch := p.Subscribe()
	go func() {
		for st := range ch {
			m.update(idx, st)
		}
	}()
}

// Focus makes service idx current (e.g. the user switched to its tab) when
// no other service is playing.
func (m *Mux) Focus(idx int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, on := range m.playing {
		if on && i != idx {
			return
		}
	}
	m.current = idx
}

func (m *Mux) update(idx int, st player.State) {
	m.mu.Lock()
	started := st.Playing && !m.playing[idx]
	m.playing[idx] = st.Playing
	var pause []player.Player
	if started {
		m.current = idx
		for i, on := range m.playing {
			if on && i != idx {
				pause = append(pause, m.players[i])
				m.playing[i] = false
			}
		}
	}
	forward := idx == m.current
	subs := append([]chan player.State(nil), m.subs...)
	m.mu.Unlock()

	// One service at a time: the one that just started wins.
	for _, p := range pause {
		go func() { _ = p.Pause() }()
	}
	if forward {
		for _, ch := range subs {
			select {
			case ch <- st:
			default:
			}
		}
	}
}

// Subscribe returns states of the current service.
func (m *Mux) Subscribe() <-chan player.State {
	ch := make(chan player.State, 8)
	m.mu.Lock()
	m.subs = append(m.subs, ch)
	m.mu.Unlock()
	return ch
}

func (m *Mux) cur() player.Player {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.players[m.current]
}

func (m *Mux) do(fn func(player.Player) error) error {
	if p := m.cur(); p != nil {
		return fn(p)
	}
	return nil
}

func (m *Mux) Play() error  { return m.do(func(p player.Player) error { return p.Play() }) }
func (m *Mux) Pause() error { return m.do(func(p player.Player) error { return p.Pause() }) }
func (m *Mux) Next() error  { return m.do(func(p player.Player) error { return p.Next() }) }
func (m *Mux) Previous() error {
	return m.do(func(p player.Player) error { return p.Previous() })
}
func (m *Mux) Seek(d time.Duration) error {
	return m.do(func(p player.Player) error { return p.Seek(d) })
}
func (m *Mux) SetRepeat(mode int) error {
	return m.do(func(p player.Player) error { return p.SetRepeat(mode) })
}
func (m *Mux) SetShuffle(on bool) error {
	return m.do(func(p player.Player) error { return p.SetShuffle(on) })
}
