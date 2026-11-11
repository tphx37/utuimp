package tui

import (
	"context"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
)

// listeningProvider records what the radio tells its station.
type listeningProvider struct {
	mockProvider
	events []string
}

func (p *listeningProvider) RadioTrackStarted(_ context.Context, seed, id string) error {
	p.events = append(p.events, "start "+id+" @"+seed)
	return nil
}

func (p *listeningProvider) RadioTrackEnded(_ context.Context, seed, id string, played time.Duration, finished bool) error {
	how := "skip"
	if finished {
		how = "finish"
	}
	p.events = append(p.events, how+" "+id+" "+played.String()+" @"+seed)
	return nil
}

func TestRadioFeedback(t *testing.T) {
	lp := &listeningProvider{}
	m := newModel(newMockPlayer())
	m.provider = lp
	m.radio.enabled = true
	m.radio.seed = &provider.Track{ID: "wave:user", CatalogID: "wave:user"}

	a := &provider.Track{ID: "a", Duration: 200 * time.Second}
	b := &provider.Track{ID: "b", Duration: 180 * time.Second}
	c := &provider.Track{ID: "c", Duration: 150 * time.Second}
	run := func(s player.State) {
		if cmd := m.radioFeedback(s); cmd != nil {
			cmd()
		}
	}
	run(player.State{Track: a, Loading: true}) // not playing yet
	run(player.State{Track: a, Playing: true}) // starts
	run(player.State{Track: a, Playing: true, Position: 193 * time.Second})
	run(player.State{Track: b, Playing: true}) // a played to the end
	run(player.State{Track: b, Playing: true, Position: 31 * time.Second})
	run(player.State{Track: b, Playing: false, Position: 31 * time.Second}) // a pause is not an end
	run(player.State{Track: c, Playing: true})                              // b skipped

	want := []string{
		"start a @wave:user",
		"finish a 3m13s @wave:user",
		"start b @wave:user",
		"skip b 31s @wave:user",
		"start c @wave:user",
	}
	if len(lp.events) != len(want) {
		t.Fatalf("events = %q, want %q", lp.events, want)
	}
	for i := range want {
		if lp.events[i] != want[i] {
			t.Errorf("event %d = %q, want %q", i, lp.events[i], want[i])
		}
	}

	// Off the radio, the station hears nothing.
	m.radio.enabled = false
	lp.events = nil
	run(player.State{Track: a, Playing: true})
	if len(lp.events) != 0 {
		t.Errorf("radio off: events = %q", lp.events)
	}
}
