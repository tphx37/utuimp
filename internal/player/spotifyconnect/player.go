// Package spotifyconnect is a player.Player for Spotify. utuimp runs
// librespot as its own Spotify Connect device ("utuimp") that plays the
// audio, and drives it (or any other device of the account) through the
// Web API. Spotify requires Premium for this.
package spotifyconnect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/audioquality"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/spotify"
)

// maxURIs bounds how many queued tracks are handed to Spotify at once.
const maxURIs = 400

// Player implements player.Player over Spotify Connect.
type Player struct {
	c    *spotify.Client
	prov *spotify.Provider
	log  func(string)

	mu        sync.Mutex
	state     player.State
	subs      []chan player.State
	queue     []string
	idx       int
	deviceID  string
	ownDevice bool // SetDevice selected the local librespot device
	// out plays librespot's sound when utuimp does that itself (see
	// Librespot.Output); nil otherwise.
	out Output
	// A skip or a seek holds the output (see Output.Hold) until librespot
	// says the new place is playing: held is the hold's number (0: none),
	// heldPlay whether to play after it. cmdAt is when a command was last
	// given here: the next polls may still report the state before it.
	held     int
	heldGen  int
	heldPlay bool
	events   bool // librespot's events are followed
	cmdAt    time.Time

	kick chan struct{} // re-read the state soon (after a command)
	done chan struct{}

	// Volume goes to Spotify from one goroutine, latest value first: a
	// burst of +/- becomes one request instead of several that may land out
	// of order. volSetAt holds the polled device volume off for a while, as
	// Spotify applies changes late and would report the old level.
	volTarget int // percent waiting to be sent; -1 = none
	volSetAt  time.Time
	volKick   chan struct{}
	closeOnce sync.Once
}

// New starts polling the user's playback state.
func New(prov *spotify.Provider, log func(string)) *Player {
	if log == nil {
		log = func(string) {}
	}
	p := &Player{c: prov.Client(), prov: prov, log: log, done: make(chan struct{}), kick: make(chan struct{}, 1),
		volTarget: -1, volKick: make(chan struct{}, 1)}
	p.state.Volume = 1
	go p.poll()
	go p.sendVolume()
	return p
}

func (p *Player) broadcast() {
	p.mu.Lock()
	s := p.state
	subs := append([]chan player.State(nil), p.subs...)
	p.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- s:
		default:
		}
	}
}

// refresh asks the poller to read the state shortly, so a command's effect
// shows without waiting a full poll interval.
func (p *Player) refresh() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

func (p *Player) fail(err error) error {
	p.refresh()
	if err == nil {
		return nil
	}
	p.mu.Lock()
	p.state.Error = err.Error()
	p.state.Loading = false
	p.mu.Unlock()
	p.log("[spotify] " + err.Error())
	p.broadcast()
	return err
}

type device struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	IsActive     bool   `json:"is_active"`
	IsRestricted bool   `json:"is_restricted"`
}

// SetOutput tells the player what plays librespot's sound, so that pause,
// play, skips and seeks given here act on it at once instead of when
// librespot's stream gets round to it.
func (p *Player) SetOutput(out Output) {
	p.mu.Lock()
	p.out = out
	if local, ok := out.(interface{ SetVolume(float64) }); ok {
		local.SetVolume(p.state.Volume)
	}
	p.mu.Unlock()
}

// FollowEvents reads librespot's playback events: they end a skip's or a
// seek's hold the moment the new place plays, and show a pause or a resume
// given elsewhere (a phone) at once.
func (p *Player) FollowEvents(events <-chan Event) {
	if events == nil {
		return
	}
	go func() {
		for {
			select {
			case <-p.done:
				return
			case ev, ok := <-events:
				if !ok {
					p.mu.Lock()
					p.events = false
					play := p.heldPlay
					p.mu.Unlock()
					p.release(0, play, 0)
					return
				}
				// Holds wait for events only once they are known to come.
				p.mu.Lock()
				p.events = true
				p.mu.Unlock()
				p.event(ev)
			}
		}
	}()
}

// commandHold is how long after a command given here the polled state is
// not trusted where it disagrees: Spotify reports the change late.
const commandHold = 2500 * time.Millisecond

// holdFor is how long a hold lasts at most when librespot sends no event.
const holdFor = 2 * time.Second

// holdFallback is how long a hold lasts without librespot's events.
const holdFallback = 700 * time.Millisecond

// event handles one of librespot's playback events.
func (p *Player) event(ev Event) {
	p.mu.Lock()
	held, play, out := p.held, p.heldPlay, p.out
	p.mu.Unlock()
	if out == nil {
		return
	}
	switch ev.Name {
	case "playing":
		if held != 0 {
			p.setPlaying(true)
			p.release(held, true, ev.Pending)
			return
		}
		if p.setPlaying(true) {
			out.Play()
		}
	case "seeked":
		if held != 0 {
			p.release(held, play, ev.Pending)
		}
	case "paused":
		if held != 0 {
			p.release(held, false, ev.Pending)
			return
		}
		if p.setPlaying(false) {
			out.Pause()
		}
	case "stopped":
		if held != 0 {
			p.release(held, false, ev.Pending)
		}
	}
}

// setPlaying shows a play or a pause reported by librespot, unless the
// opposite was just asked for here (the event may be about an earlier
// state). It reports whether the report was taken.
func (p *Player) setPlaying(on bool) bool {
	p.mu.Lock()
	if time.Since(p.cmdAt) < commandHold && p.state.Playing != on {
		p.mu.Unlock()
		return false
	}
	changed := p.state.Playing != on || (on && p.state.Loading)
	p.state.Playing = on
	if on {
		p.state.Loading = false
	}
	p.mu.Unlock()
	if changed {
		p.broadcast()
	}
	return true
}

// hold starts a skip's or a seek's hold: the output is silent and drops
// the old place until librespot plays the new one (or the time is up).
// play is whether to play once it does. Without an output it does nothing.
func (p *Player) hold(play bool) {
	p.mu.Lock()
	p.cmdAt = time.Now()
	out := p.out
	if out == nil {
		p.mu.Unlock()
		return
	}
	p.heldGen++
	gen := p.heldGen
	p.held = gen
	p.heldPlay = play
	wait := holdFor
	if !p.events {
		wait = holdFallback
	}
	p.mu.Unlock()
	out.Hold()
	time.AfterFunc(wait, func() {
		p.mu.Lock()
		play := p.heldPlay
		p.mu.Unlock()
		p.release(gen, play, 0)
	})
}

// release ends hold number gen (0: whichever is on).
func (p *Player) release(gen int, play bool, skip int) {
	p.mu.Lock()
	if p.held == 0 || gen != 0 && gen != p.held {
		p.mu.Unlock()
		return
	}
	p.held = 0
	out := p.out
	p.mu.Unlock()
	if out != nil {
		out.Release(play, skip)
	}
}

// SetDevice makes id (the utuimp librespot device) the playback device.
func (p *Player) SetDevice(id string) {
	p.mu.Lock()
	p.deviceID = id
	p.ownDevice = id != ""
	p.mu.Unlock()
}

// ensureDevice picks the device to play on: utuimp's own, else the active
// one, else this computer, else the first available.
func (p *Player) ensureDevice(ctx context.Context) (string, error) {
	p.mu.Lock()
	id := p.deviceID
	ownDevice := p.ownDevice
	p.mu.Unlock()
	if id != "" {
		return id, nil
	}
	var res struct {
		Devices []device `json:"devices"`
	}
	if err := p.c.Do(ctx, http.MethodGet, "/me/player/devices", nil, nil, &res); err != nil {
		return "", err
	}
	var pick *device
	for i := range res.Devices {
		d := &res.Devices[i]
		if d.ID == "" || d.IsRestricted || (ownDevice && d.Name != DeviceName) {
			continue
		}
		switch {
		case d.Name == DeviceName:
			pick = d
		case pick != nil && pick.Name == DeviceName:
		case d.IsActive:
			pick = d
		case pick == nil, !pick.IsActive && d.Type == "Computer" && pick.Type != "Computer":
			pick = d
		}
	}
	if pick == nil {
		return "", spotify.ErrNoDevice
	}
	p.mu.Lock()
	p.deviceID = pick.ID
	p.mu.Unlock()
	p.log("[spotify] playing on " + pick.Name)
	return pick.ID, nil
}

func (p *Player) devQuery(ctx context.Context) (url.Values, error) {
	id, err := p.ensureDevice(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("device_id", id)
	return q, nil
}

func ctx10() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// playFrom starts the local queue at idx on the device.
func (p *Player) playFrom(idx int) error {
	p.mu.Lock()
	if idx < 0 || idx >= len(p.queue) {
		p.mu.Unlock()
		return nil
	}
	p.idx = idx
	ids := p.queue[idx:min(len(p.queue), idx+maxURIs)]
	uris := make([]string, len(ids))
	for i, id := range ids {
		uris[i] = "spotify:track:" + id
	}
	p.state.Loading = true
	p.state.Playing = true // it is asked to play
	p.state.Error = ""
	p.mu.Unlock()
	p.hold(true)
	p.broadcast()

	ctx, cancel := ctx10()
	defer cancel()
	q, err := p.devQuery(ctx)
	if err != nil {
		return p.fail(err)
	}
	err = p.c.Do(ctx, http.MethodPut, "/me/player/play", q, map[string]any{"uris": uris}, nil)
	if errors.Is(err, spotify.ErrNoDevice) {
		// The remembered device went away: pick again once.
		p.mu.Lock()
		p.deviceID = ""
		p.mu.Unlock()
		if q, err = p.devQuery(ctx); err == nil {
			err = p.c.Do(ctx, http.MethodPut, "/me/player/play", q, map[string]any{"uris": uris}, nil)
		}
	}
	if err != nil {
		p.mu.Lock()
		ownDevice := p.ownDevice
		p.mu.Unlock()
		if ownDevice && errors.Is(err, spotify.ErrPlaybackRestricted) {
			err = fmt.Errorf("%w; librespot may need an update (see docs/troubleshooting.md#spotify)", err)
		}
		err = fmt.Errorf("PUT /me/player/play: %w", err)
	}
	return p.fail(err)
}

// simple sends a body-less playback command.
func (p *Player) simple(method, path string, extra url.Values) error {
	ctx, cancel := ctx10()
	defer cancel()
	q, err := p.devQuery(ctx)
	if err != nil {
		return p.fail(err)
	}
	maps.Copy(q, extra)
	err = p.c.Do(ctx, method, path, q, nil, nil)
	if err != nil {
		err = fmt.Errorf("%s %s: %w", method, path, err)
	}
	return p.fail(err)
}

// ── player.Player ──────────────────────────────────────────────────────────

func (p *Player) Play() error {
	p.mu.Lock()
	idle := p.state.Track == nil && len(p.queue) > 0
	idx := p.idx
	p.mu.Unlock()
	if idle {
		return p.playFrom(idx)
	}
	p.mu.Lock()
	p.state.Playing = true
	p.cmdAt = time.Now()
	p.heldPlay = true // a skip under way plays once it gets there
	out := p.out
	p.mu.Unlock()
	if out != nil {
		out.Play()
	}
	p.broadcast()
	return p.simple(http.MethodPut, "/me/player/play", nil)
}

func (p *Player) Pause() error {
	p.mu.Lock()
	p.state.Playing = false
	p.cmdAt = time.Now()
	p.heldPlay = false // a skip under way stays paused once it gets there
	out := p.out
	p.mu.Unlock()
	if out != nil {
		out.Pause() // silence now, not when the request gets there
	}
	p.broadcast()
	return p.simple(http.MethodPut, "/me/player/pause", nil)
}

func (p *Player) Stop() error { return p.Pause() }

// Next, Previous and Seek hold the output until the new place plays (see
// hold): until Spotify gets the command to librespot, what it sends is
// still the old place.
func (p *Player) Next() error {
	p.mu.Lock()
	p.state.Loading = true
	playing := p.state.Playing
	p.mu.Unlock()
	p.hold(playing)
	return p.simple(http.MethodPost, "/me/player/next", nil)
}

func (p *Player) Previous() error {
	p.mu.Lock()
	playing := p.state.Playing
	p.mu.Unlock()
	p.hold(playing)
	return p.simple(http.MethodPost, "/me/player/previous", nil)
}

func (p *Player) Seek(pos time.Duration) error {
	p.mu.Lock()
	playing := p.state.Playing
	p.mu.Unlock()
	p.hold(playing)
	return p.simple(http.MethodPut, "/me/player/seek", url.Values{"position_ms": {strconv.FormatInt(pos.Milliseconds(), 10)}})
}

// volumeHold is how long after a local change the polled device volume is
// ignored (unless it already matches).
const volumeHold = 3 * time.Second

// volumeDebounce gathers a burst of volume changes into one request.
const volumeDebounce = 120 * time.Millisecond

// SetVolume shows the new volume at once and has sendVolume pass it on.
func (p *Player) SetVolume(v float64) error {
	pct := int(math.Round(min(max(v, 0), 1) * 100))
	p.mu.Lock()
	p.state.Volume = float64(pct) / 100
	if local, ok := p.out.(interface{ SetVolume(float64) }); ok {
		local.SetVolume(p.state.Volume)
		p.mu.Unlock()
		p.broadcast()
		return nil
	}
	p.volTarget = pct
	p.volSetAt = time.Now()
	p.mu.Unlock()
	p.broadcast()
	select {
	case p.volKick <- struct{}{}:
	default:
	}
	return nil
}

// sendVolume sends the latest requested volume, one request at a time.
func (p *Player) sendVolume() {
	for {
		select {
		case <-p.done:
			return
		case <-p.volKick:
		}
		select {
		case <-p.done:
			return
		case <-time.After(volumeDebounce):
		}
		p.mu.Lock()
		pct := p.volTarget
		p.volTarget = -1
		p.mu.Unlock()
		if pct < 0 {
			continue
		}
		_ = p.simple(http.MethodPut, "/me/player/volume", url.Values{"volume_percent": {strconv.Itoa(pct)}})
	}
}

func (p *Player) SetAudioBitrate(kbps int) error {
	if err := audioquality.Validate(kbps); err != nil {
		return err
	}
	return player.ErrAudioBitrateSavedPreferenceOnly
}

func (p *Player) SetQueue(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	p.mu.Lock()
	p.queue = append([]string(nil), ids...)
	p.mu.Unlock()
	return p.playFrom(0)
}

func (p *Player) SetPlaylist(_ string, startIdx int) error { return p.playFrom(startIdx) }

// PlayAt implements player.QueueJumper.
func (p *Player) PlayAt(idx int) error { return p.playFrom(idx) }

func (p *Player) AppendQueue(ids []string) error {
	p.mu.Lock()
	idle := len(p.queue) == 0 || p.state.Track == nil
	first := len(p.queue)
	p.queue = append(p.queue, ids...)
	p.mu.Unlock()
	if idle {
		return p.playFrom(first)
	}
	for _, id := range ids {
		if err := p.simple(http.MethodPost, "/me/player/queue", url.Values{"uri": {"spotify:track:" + id}}); err != nil {
			return err
		}
	}
	return nil
}

// PlayNext implements player.NextQueuer via Spotify's own queue, which plays
// before the rest of the current list.
func (p *Player) PlayNext(ids []string) error {
	p.mu.Lock()
	if len(p.queue) == 0 {
		p.mu.Unlock()
		return p.SetQueue(ids)
	}
	at := min(p.idx+1, len(p.queue))
	p.queue = append(p.queue[:at], append(append([]string(nil), ids...), p.queue[at:]...)...)
	p.mu.Unlock()
	for _, id := range ids {
		if err := p.simple(http.MethodPost, "/me/player/queue", url.Values{"uri": {"spotify:track:" + id}}); err != nil {
			return err
		}
	}
	return nil
}

func (p *Player) SetRepeat(mode int) error {
	st := "off"
	switch mode {
	case player.RepeatModeAll:
		st = "context"
	case player.RepeatModeOne:
		st = "track"
	}
	p.mu.Lock()
	p.state.RepeatMode = mode
	p.mu.Unlock()
	return p.simple(http.MethodPut, "/me/player/repeat", url.Values{"state": {st}})
}

func (p *Player) SetShuffle(on bool) error {
	p.mu.Lock()
	p.state.ShuffleMode = on
	p.mu.Unlock()
	return p.simple(http.MethodPut, "/me/player/shuffle", url.Values{"state": {strconv.FormatBool(on)}})
}

// SetEqualizer: the sound is made by the Spotify device, not here.
func (p *Player) SetEqualizer([]player.EQBand) error { return nil }

// RemoveFromQueue and MoveInQueue edit the local list; Spotify's own queue
// can't be edited through the API, so the change applies from the next
// time the list is (re)started.
func (p *Player) RemoveFromQueue(idx int) error {
	p.mu.Lock()
	if idx < 0 || idx >= len(p.queue) {
		p.mu.Unlock()
		return nil
	}
	p.queue = append(p.queue[:idx], p.queue[idx+1:]...)
	current := idx == p.idx
	if idx < p.idx {
		p.idx--
	}
	i := p.idx
	p.mu.Unlock()
	if current {
		return p.playFrom(i)
	}
	return nil
}

func (p *Player) MoveInQueue(from, to int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if from < 0 || from >= len(p.queue) || to < 0 || to >= len(p.queue) {
		return nil
	}
	id := p.queue[from]
	p.queue = append(p.queue[:from], p.queue[from+1:]...)
	p.queue = append(p.queue[:to], append([]string{id}, p.queue[to:]...)...)
	return nil
}

func (p *Player) ClearQueue() error {
	p.mu.Lock()
	p.queue = nil
	p.idx = 0
	p.state.Track = nil
	p.mu.Unlock()
	return p.Pause()
}

func (p *Player) GetState() (*player.State, error) {
	p.mu.Lock()
	s := p.state
	p.mu.Unlock()
	return &s, nil
}

func (p *Player) Subscribe() <-chan player.State {
	ch := make(chan player.State, 8)
	p.mu.Lock()
	p.subs = append(p.subs, ch)
	p.mu.Unlock()
	return ch
}

func (p *Player) Close() error {
	p.closeOnce.Do(func() { close(p.done) })
	return nil
}

type playbackState struct {
	IsPlaying    bool   `json:"is_playing"`
	ProgressMs   int64  `json:"progress_ms"`
	ShuffleState bool   `json:"shuffle_state"`
	RepeatState  string `json:"repeat_state"`
	Device       struct {
		ID            string `json:"id"`
		VolumePercent *int   `json:"volume_percent"`
	} `json:"device"`
	Item json.RawMessage `json:"item"`
}

// Poll intervals: Spotify rate-limits this client tightly, so the state is
// read every few seconds (the UI interpolates the position in between) and
// rarely while nothing plays.
const (
	pollPlaying = 3 * time.Second
	pollIdle    = 10 * time.Second
)

// poll mirrors the device's playback into the player state.
func (p *Player) poll() {
	var curID string
	wait := time.Second
	for {
		select {
		case <-p.done:
			return
		case <-time.After(wait):
		case <-p.kick:
			time.Sleep(400 * time.Millisecond) // let Spotify apply the command
		}
		ctx, cancel := ctx10()
		var st playbackState
		err := p.c.Do(ctx, http.MethodGet, "/me/player", nil, nil, &st)
		cancel()
		var rl spotify.ErrRateLimited
		switch {
		case errors.As(err, &rl):
			wait = rl.Wait
			continue
		case err != nil || len(st.Item) == 0 || string(st.Item) == "null":
			wait = pollIdle
			continue
		}
		var track *provider.Track
		if tr, perr := p.prov.ParseTrack(st.Item); perr == nil && tr.ID != curID {
			track = &tr
			curID = tr.ID
		}

		p.mu.Lock()
		if track != nil {
			p.state.Track = track
			for i, id := range p.queue {
				if id == curID {
					p.idx = i
					break
				}
			}
		}
		if st.Device.ID != "" && !p.ownDevice {
			p.deviceID = st.Device.ID
		}
		// Right after a command given here Spotify may still report the
		// state before it: keep ours until it agrees or the time is up.
		if time.Since(p.cmdAt) >= commandHold || st.IsPlaying == p.state.Playing {
			p.state.Playing = st.IsPlaying
			if st.IsPlaying && p.out != nil && p.held == 0 {
				// Started from somewhere else (a phone) while paused here.
				p.out.Play()
			}
		}
		p.state.Loading = false
		p.state.Position = time.Duration(st.ProgressMs) * time.Millisecond
		p.state.ShuffleMode = st.ShuffleState
		switch st.RepeatState {
		case "context":
			p.state.RepeatMode = player.RepeatModeAll
		case "track":
			p.state.RepeatMode = player.RepeatModeOne
		default:
			p.state.RepeatMode = player.RepeatModeOff
		}
		if v := st.Device.VolumePercent; v != nil && p.out == nil {
			// Right after a local change the device may still report the
			// old level: keep ours until it catches up or the hold ends.
			reported := float64(*v) / 100
			if time.Since(p.volSetAt) > volumeHold || math.Abs(reported-p.state.Volume) < 0.015 {
				p.state.Volume = reported
				p.volSetAt = time.Time{}
			}
		}
		p.mu.Unlock()
		p.broadcast()
		wait = pollPlaying
		if !st.IsPlaying {
			wait = pollIdle / 2
		}
	}
}

var (
	_ player.Player      = (*Player)(nil)
	_ player.QueueJumper = (*Player)(nil)
	_ player.NextQueuer  = (*Player)(nil)
)
