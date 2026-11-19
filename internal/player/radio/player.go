// Package radio plays continuous internet radio streams without buffering them in Go.
package radio

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/player/browserless"
	"github.com/tphx37/utuimp/internal/provider"
)

type lookup interface {
	Lookup(string) (provider.Track, bool)
	GetPlaylistTracks(context.Context, string) ([]provider.Track, error)
}
type Player struct {
	mu           sync.Mutex
	sink         browserless.AudioSink
	source       lookup
	state        player.State
	queue        []string
	idx          int
	generation   uint64
	streamArtist bool
	streamAlbum  bool
	cancel       context.CancelFunc
	broadcast    player.Broadcast
	once         sync.Once
}

func New(source lookup) (*Player, error) {
	sink, e := browserless.NewAudioSink()
	if e != nil {
		return nil, e
	}
	p := &Player{sink: sink, source: source, idx: -1, state: player.State{Volume: 1}}
	sink.OnError(func(err error) {
		p.mu.Lock()
		p.state.Error = err.Error()
		p.state.Playing = false
		p.state.Loading = false
		s := p.state
		p.mu.Unlock()
		p.broadcast.Send(s)
	})
	if tagged, ok := sink.(interface {
		OnTags(func(string, string, string, string))
	}); ok {
		tagged.OnTags(p.updateMetadata)
	}
	sink.OnEOS(func() { _ = p.Next() })
	return p, nil
}

func stationHost(id string) string {
	u, err := url.Parse(id)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// updateMetadata changes display fields while retaining the station's ID.
func (p *Player) updateMetadata(title, artist, album, station string) {
	title, artist, album, station = provider.Clean(title), provider.Clean(artist), provider.Clean(album), provider.Clean(station)
	if title != "" && artist == "" {
		if a, b, ok := strings.Cut(title, " - "); ok && strings.TrimSpace(a) != "" && strings.TrimSpace(b) != "" {
			artist, title = strings.TrimSpace(a), strings.TrimSpace(b)
		}
	}
	p.mu.Lock()
	if p.state.Track == nil || !p.state.Playing {
		p.mu.Unlock()
		return
	}
	t := *p.state.Track
	if title != "" && title != t.Title {
		if artist == "" && p.streamArtist {
			t.Artist = ""
			p.streamArtist = false
		}
		if album == "" && p.streamAlbum {
			t.Album = ""
			p.streamAlbum = false
		}
	}
	if title != "" {
		t.Title = title
	} else if station != "" && (t.Title == "" || t.Title == t.ID || stationHost(t.ID) == t.Title) {
		t.Title = station
	}
	if artist != "" {
		t.Artist = artist
		p.streamArtist = true
	} else if station != "" && (t.Artist == "" || t.Artist == "Internet radio") && title != "" {
		t.Artist = station
	}
	if album != "" {
		t.Album = album
		p.streamAlbum = true
	}
	if t.Title == p.state.Track.Title && t.Artist == p.state.Track.Artist && t.Album == p.state.Track.Album {
		p.mu.Unlock()
		return
	}
	p.state.Track = &t
	s := p.state
	p.mu.Unlock()
	p.broadcast.Send(s)
}

func (p *Player) playAt(i int) error {
	p.mu.Lock()
	if i < 0 || i >= len(p.queue) {
		p.mu.Unlock()
		return errors.New("station not in queue")
	}
	id := p.queue[i]
	t, ok := p.source.Lookup(id)
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("station not found")
	}
	if p.cancel != nil {
		p.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.generation++
	generation := p.generation
	p.idx = i
	p.streamArtist, p.streamAlbum = false, false
	p.state.Track = &t
	p.state.Playing = true
	p.state.Loading = true
	p.state.Error = ""
	s := p.state
	p.mu.Unlock()
	p.broadcast.Send(s)
	stream, err := resolvePlaylist(ctx, id)
	p.mu.Lock()
	if generation != p.generation {
		p.mu.Unlock()
		return nil
	}
	if err != nil {
		p.state.Playing = false
		p.state.Loading = false
		p.state.Error = err.Error()
		s = p.state
		p.mu.Unlock()
		p.broadcast.Send(s)
		return err
	}
	p.sink.PlayURI(stream)
	p.state.Loading = false
	s = p.state
	if !p.state.Playing {
		p.sink.Pause()
	}
	p.mu.Unlock()
	p.broadcast.Send(s)
	return nil
}
func (p *Player) PlayAt(i int) error { return p.playAt(i) }
func (p *Player) Play() error {
	p.mu.Lock()
	i := p.idx
	has := p.state.Track != nil
	p.state.Playing = true
	s := p.state
	p.mu.Unlock()
	if !has {
		return p.playAt(i)
	}
	p.sink.Play()
	p.broadcast.Send(s)
	return nil
}
func (p *Player) Pause() error {
	p.sink.Pause()
	p.mu.Lock()
	p.state.Playing = false
	s := p.state
	p.mu.Unlock()
	p.broadcast.Send(s)
	return nil
}
func (p *Player) Stop() error {
	p.mu.Lock()
	p.generation++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.sink.Stop()
	p.state.Playing = false
	p.state.Loading = false
	s := p.state
	p.mu.Unlock()
	p.broadcast.Send(s)
	return nil
}
func (p *Player) Next() error {
	p.mu.Lock()
	i := p.idx + 1
	end := i >= len(p.queue)
	p.mu.Unlock()
	if end {
		return p.Stop()
	}
	return p.playAt(i)
}
func (p *Player) Previous() error        { p.mu.Lock(); i := p.idx - 1; p.mu.Unlock(); return p.playAt(i) }
func (*Player) Seek(time.Duration) error { return errors.New("live radio cannot be seeked") }
func (p *Player) SetVolume(v float64) error {
	v = max(0, min(1, v))
	p.sink.SetVolume(v)
	p.mu.Lock()
	p.state.Volume = v
	s := p.state
	p.mu.Unlock()
	p.broadcast.Send(s)
	return nil
}
func (*Player) SetAudioBitrate(int) error { return player.ErrAudioBitrateSavedPreferenceOnly }
func (p *Player) SetQueue(ids []string) error {
	if len(ids) == 0 {
		return p.ClearQueue()
	}
	p.mu.Lock()
	p.queue = append([]string(nil), ids...)
	p.idx = 0
	p.mu.Unlock()
	return p.playAt(0)
}
func (p *Player) SetPlaylist(id string, index int) error {
	tracks, err := p.source.GetPlaylistTracks(context.Background(), id)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(tracks))
	for _, t := range tracks {
		ids = append(ids, t.ID)
	}
	if len(ids) == 0 {
		return errors.New("playlist is empty")
	}
	if index < 0 || index >= len(ids) {
		index = 0
	}
	p.mu.Lock()
	p.queue = ids
	p.idx = index
	p.mu.Unlock()
	return p.playAt(index)
}
func (p *Player) AppendQueue(ids []string) error {
	p.mu.Lock()
	empty := len(p.queue) == 0
	p.queue = append(p.queue, ids...)
	p.mu.Unlock()
	if empty && len(ids) > 0 {
		return p.playAt(0)
	}
	return nil
}
func (p *Player) SetRepeat(mode int) error {
	p.mu.Lock()
	p.state.RepeatMode = mode
	p.mu.Unlock()
	return nil
}
func (p *Player) SetShuffle(on bool) error {
	p.mu.Lock()
	p.state.ShuffleMode = on
	p.mu.Unlock()
	return nil
}
func (p *Player) SetEqualizer(bands []player.EQBand) error {
	g := make([]float64, len(bands))
	for i, b := range bands {
		g[i] = b.Gain
	}
	p.sink.SetEqualizer(g)
	return nil
}
func (p *Player) RemoveFromQueue(i int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i < 0 || i >= len(p.queue) {
		return errors.New("invalid queue index")
	}
	p.queue = append(p.queue[:i], p.queue[i+1:]...)
	if i < p.idx {
		p.idx--
	}
	return nil
}
func (p *Player) MoveInQueue(from, to int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if from < 0 || to < 0 || from >= len(p.queue) || to >= len(p.queue) {
		return errors.New("invalid queue index")
	}
	v := p.queue[from]
	p.queue = append(p.queue[:from], p.queue[from+1:]...)
	p.queue = append(p.queue[:to], append([]string{v}, p.queue[to:]...)...)
	switch {
	case p.idx == from:
		p.idx = to
	case from < p.idx && to >= p.idx:
		p.idx--
	case from > p.idx && to <= p.idx:
		p.idx++
	}
	return nil
}
func (p *Player) ClearQueue() error {
	_ = p.Stop()
	p.mu.Lock()
	p.queue = nil
	p.idx = -1
	p.state.Track = nil
	s := p.state
	p.mu.Unlock()
	p.broadcast.Send(s)
	return nil
}
func (p *Player) GetState() (*player.State, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.state
	return &s, nil
}
func (p *Player) Subscribe() <-chan player.State { return p.broadcast.Subscribe() }
func (p *Player) Close() error {
	p.once.Do(func() { _ = p.Stop(); p.sink.Destroy(); p.broadcast.Close() })
	return nil
}
