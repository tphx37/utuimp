package ystream

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/audioquality"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/player/browserless"
	"github.com/tphx37/utuimp/internal/provider"
)

// Source resolves what the player needs from the music service.
type Source interface {
	// Lookup returns track metadata for an id.
	Lookup(ctx context.Context, id string) (provider.Track, error)
	// StreamURL returns a fresh direct link to the audio and its codec
	// ("mp3" or "aac").
	StreamURL(ctx context.Context, id string) (link, codec string, err error)
}

// Opener is implemented by sources that produce the audio themselves (a
// helper program, an HLS playlist) instead of handing out a link. When it
// is implemented StreamURL is not called. The stream must be MP3 or ADTS
// AAC (codec "mp3" or "aac"). meta, when not nil, is fresher metadata for
// the track, found while opening it.
type Opener interface {
	OpenStream(ctx context.Context, id string) (rc io.ReadCloser, codec string, meta *provider.Track, err error)
}

// trackData is one track's audio, downloaded into memory while it plays.
type trackData struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []byte
	done   bool
	err    error
	codec  string
	cancel context.CancelFunc
}

func newTrackData(codec string, cancel context.CancelFunc) *trackData {
	d := &trackData{codec: codec, cancel: cancel}
	d.cond = sync.NewCond(&d.mu)
	return d
}

func (d *trackData) finish(err error) {
	d.mu.Lock()
	d.done, d.err = true, err
	d.cond.Broadcast()
	d.mu.Unlock()
}

func (d *trackData) download(ctx context.Context, link string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		d.finish(err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		d.finish(err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		d.finish(fmt.Errorf("audio download: HTTP %d", resp.StatusCode))
		return
	}
	d.read(resp.Body)
}

// read buffers r until it ends.
func (d *trackData) read(r io.Reader) {
	finish := d.finish
	chunk := make([]byte, 64<<10)
	for {
		n, rerr := r.Read(chunk)
		if n > 0 {
			d.mu.Lock()
			d.buf = append(d.buf, chunk[:n]...)
			d.cond.Broadcast()
			d.mu.Unlock()
		}
		if rerr == io.EOF {
			finish(nil)
			return
		}
		if rerr != nil {
			finish(rerr)
			return
		}
	}
}

// Player implements player.Player for Yandex Music, and for any other
// service whose Source hands out MP3 or AAC.
type Player struct {
	name string                // service name for the debug log
	gst  browserless.AudioSink // platform audio output
	src  Source
	ln   net.Listener
	tok  string

	mu         sync.RWMutex
	state      player.State
	subs       []chan player.State
	queue      []string
	meta       map[string]provider.Track
	idx        int
	ended      bool
	gen        int
	tracks     map[int]*trackData // by generation
	seekOffset time.Duration

	log       func(string)
	done      chan struct{}
	closeOnce sync.Once
}

// Option configures a Player.
type Option func(*Player)

// WithName sets the service name used in log lines (default "yandex").
func WithName(name string) Option { return func(p *Player) { p.name = name } }

// New starts the GStreamer pipeline and the loopback server.
func New(src Source, log func(string), opts ...Option) (*Player, error) {
	cfg := &Player{name: "yandex"}
	for _, o := range opts {
		o(cfg)
	}
	name := cfg.name
	g, err := browserless.NewAudioSink()
	if err != nil {
		return nil, fmt.Errorf("%s player: %w", name, err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		g.Destroy()
		return nil, err
	}
	b := make([]byte, 16)
	_, _ = crand.Read(b)
	if log == nil {
		log = func(string) {}
	}
	p := &Player{
		name: name,
		gst:  g, src: src, ln: ln, tok: hex.EncodeToString(b),
		meta: map[string]provider.Track{}, tracks: map[int]*trackData{},
		log: log, done: make(chan struct{}),
	}
	p.state.Volume = 1
	mux := http.NewServeMux()
	mux.HandleFunc("/t/", p.serve)
	go func() { _ = http.Serve(ln, mux) }() //nolint:gosec // loopback only
	g.OnEOS(func() { _ = p.advance() })
	g.OnError(func(e error) {
		p.mu.Lock()
		p.state.Error = e.Error()
		s := p.state
		p.mu.Unlock()
		p.log("[" + p.name + "] playback error: " + e.Error())
		p.broadcast(s)
	})
	go p.pollState()
	return p, nil
}

// SetTracks seeds metadata for ids the UI already knows (avoids lookups).
func (p *Player) SetTracks(ts []provider.Track) {
	p.mu.Lock()
	for _, t := range ts {
		p.meta[t.ID] = t
	}
	p.mu.Unlock()
}

func (p *Player) broadcast(s player.State) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, ch := range p.subs {
		select {
		case ch <- s:
		default:
		}
	}
}

// streamURL is /t/{gen}/{token}/{skipMs}.{mp3|aac}; the extension tells
// the macOS and Windows outputs which decoder to use.
func (p *Player) streamURL(gen int, skip time.Duration) string {
	ext := "mp3"
	p.mu.RLock()
	if d := p.tracks[gen]; d != nil && d.codec == "aac" {
		ext = "aac"
	}
	p.mu.RUnlock()
	return fmt.Sprintf("http://127.0.0.1:%d/t/%d/%s/%d.%s",
		p.ln.Addr().(*net.TCPAddr).Port, gen, p.tok, skip.Milliseconds(), ext)
}

// serve streams /t/{gen}/{token}[/{skipMs}] from the in-memory download.
func (p *Player) serve(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/t/"), "/")
	if len(parts) < 2 || parts[1] != p.tok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	gen, _ := strconv.Atoi(parts[0])
	var skip time.Duration
	if len(parts) > 2 {
		ms, _ := strconv.ParseInt(strings.SplitN(parts[2], ".", 2)[0], 10, 64)
		skip = time.Duration(ms) * time.Millisecond
	}
	p.mu.RLock()
	d := p.tracks[gen]
	p.mu.RUnlock()
	if d == nil {
		http.NotFound(w, r)
		return
	}
	ctype := "audio/mpeg"
	if d.codec == "aac" {
		ctype = "audio/aac"
	}
	w.Header().Set("Content-Type", ctype)
	flusher, _ := w.(http.Flusher)

	stop := context.AfterFunc(r.Context(), func() {
		d.mu.Lock()
		d.cond.Broadcast()
		d.mu.Unlock()
	})
	defer stop()

	d.mu.Lock()
	off := 0
	if skip > 0 {
		for {
			o, _, ok := frameOffset(d.buf, d.codec, skip, d.done)
			if ok {
				off = o
				break
			}
			if r.Context().Err() != nil {
				d.mu.Unlock()
				return
			}
			d.cond.Wait()
		}
	}
	for {
		for off >= len(d.buf) && !d.done && r.Context().Err() == nil {
			d.cond.Wait()
		}
		if r.Context().Err() != nil || off >= len(d.buf) {
			d.mu.Unlock()
			return
		}
		chunk := d.buf[off:]
		d.mu.Unlock()
		n, err := w.Write(chunk)
		if flusher != nil {
			flusher.Flush()
		}
		if err != nil {
			return
		}
		d.mu.Lock()
		off += n
	}
}

// start resolves and plays queue[idx] (generation gen).
func (p *Player) start(gen, idx int) {
	p.mu.RLock()
	id := p.queue[idx]
	t, known := p.meta[id]
	p.mu.RUnlock()

	ctx, cancel := context.WithCancel(context.Background())
	if !known {
		lctx, lcancel := context.WithTimeout(ctx, 10*time.Second)
		if mt, err := p.src.Lookup(lctx, id); err == nil {
			t = mt
		} else {
			t = provider.Track{ID: id}
		}
		lcancel()
	}
	p.mu.Lock()
	if gen != p.gen {
		p.mu.Unlock()
		cancel()
		return
	}
	p.meta[id] = t
	tc := t
	p.state.Track = &tc
	s := p.state
	p.mu.Unlock()
	p.broadcast(s)

	var (
		link, codec string
		body        io.ReadCloser
		err         error
	)
	if op, ok := p.src.(Opener); ok {
		// The stream lives as long as the track (ctx), not just the 15s
		// a link gets to resolve.
		var meta *provider.Track
		body, codec, meta, err = op.OpenStream(ctx, id)
		if err == nil && meta != nil {
			m := *meta
			m.ID = id
			p.mu.Lock()
			if gen == p.gen {
				p.meta[id] = m
				t = m
				tc := m
				p.state.Track = &tc
			}
			p.mu.Unlock()
		}
	} else {
		rctx, rcancel := context.WithTimeout(ctx, 15*time.Second)
		link, codec, err = p.src.StreamURL(rctx, id)
		rcancel()
	}
	if err != nil {
		cancel()
		p.log(fmt.Sprintf("[%s] %s: %v", p.name, t.Title, err))
		p.mu.Lock()
		stale := gen != p.gen
		if !stale {
			p.state.Error = "cannot play “" + t.Title + "”: " + err.Error()
			p.state.SkippedID = id
		}
		s := p.state
		p.mu.Unlock()
		if !stale {
			p.broadcast(s)
			_ = p.advance()
		}
		return
	}

	d := newTrackData(codec, cancel)
	p.mu.Lock()
	if gen != p.gen {
		p.mu.Unlock()
		cancel()
		if body != nil {
			_ = body.Close()
		}
		return
	}
	for g, old := range p.tracks {
		if g != gen {
			old.cancel()
			delete(p.tracks, g)
		}
	}
	p.tracks[gen] = d
	p.seekOffset = 0
	p.ended = false
	p.state.Loading = false
	p.state.Playing = true
	p.state.Position = 0
	p.state.Error = ""
	p.state.SkippedID = ""
	s = p.state
	p.mu.Unlock()

	if body != nil {
		go func() {
			defer func() { _ = body.Close() }()
			d.read(body)
		}()
	} else {
		go d.download(ctx, link)
	}
	p.gst.PlayURI(p.streamURL(gen, 0))
	p.broadcast(s)
}

// playIdx switches to queue position idx.
func (p *Player) playIdx(idx int) {
	p.mu.Lock()
	if idx < 0 || idx >= len(p.queue) {
		p.mu.Unlock()
		return
	}
	p.gen++
	gen := p.gen
	p.idx = idx
	p.state.Loading = true
	p.state.Playing = false
	p.state.Position = 0
	if t, ok := p.meta[p.queue[idx]]; ok {
		tc := t
		p.state.Track = &tc
	}
	s := p.state
	p.mu.Unlock()
	p.gst.Stop()
	p.broadcast(s)
	go p.start(gen, idx)
}

// advance moves on after a track ends (honouring repeat/shuffle).
func (p *Player) advance() error {
	p.mu.Lock()
	n := len(p.queue)
	if n == 0 {
		p.mu.Unlock()
		return nil
	}
	next := p.idx + 1
	switch {
	case p.state.RepeatMode == player.RepeatModeOne:
		next = p.idx
	case p.state.ShuffleMode && n > 1:
		next = rand.IntN(n) //nolint:gosec // shuffle
	case next >= n && p.state.RepeatMode == player.RepeatModeAll:
		next = 0
	case next >= n:
		p.ended = true
		p.state.Playing = false
		s := p.state
		p.mu.Unlock()
		p.broadcast(s)
		return nil
	}
	p.mu.Unlock()
	p.playIdx(next)
	return nil
}

// ── player.Player ──────────────────────────────────────────────────────────

func (p *Player) Play() error {
	p.mu.Lock()
	if (p.ended || p.state.Track == nil) && len(p.queue) > 0 {
		idx := p.idx
		if p.ended || idx < 0 || idx >= len(p.queue) {
			idx = 0
		}
		p.mu.Unlock()
		p.playIdx(idx)
		return nil
	}
	p.state.Playing = true
	s := p.state
	p.mu.Unlock()
	p.gst.Play()
	p.broadcast(s)
	return nil
}

func (p *Player) Pause() error {
	p.gst.Pause()
	p.mu.Lock()
	p.state.Playing = false
	s := p.state
	p.mu.Unlock()
	p.broadcast(s)
	return nil
}

func (p *Player) Stop() error {
	p.gst.Stop()
	p.mu.Lock()
	p.state.Playing = false
	s := p.state
	p.mu.Unlock()
	p.broadcast(s)
	return nil
}

func (p *Player) Next() error {
	p.mu.Lock()
	n := len(p.queue)
	if n == 0 {
		p.mu.Unlock()
		return nil
	}
	next := p.idx + 1
	if p.state.ShuffleMode && n > 1 {
		next = rand.IntN(n) //nolint:gosec // shuffle
	} else if next >= n {
		next = 0
	}
	p.mu.Unlock()
	p.playIdx(next)
	return nil
}

func (p *Player) Previous() error {
	if p.position() > 3*time.Second {
		return p.Seek(0)
	}
	p.mu.Lock()
	n := len(p.queue)
	if n == 0 {
		p.mu.Unlock()
		return nil
	}
	prev := p.idx - 1
	if prev < 0 {
		prev = n - 1
	}
	p.mu.Unlock()
	p.playIdx(prev)
	return nil
}

// PlayAt implements player.QueueJumper.
func (p *Player) PlayAt(idx int) error {
	p.playIdx(idx)
	return nil
}

func (p *Player) Seek(pos time.Duration) error {
	p.mu.Lock()
	gen := p.gen
	d := p.tracks[gen]
	playing := p.state.Playing
	if d == nil {
		p.mu.Unlock()
		return errors.New("nothing playing")
	}
	pos = max(pos, 0)
	if t := p.state.Track; t != nil && t.Duration > 0 {
		pos = min(pos, t.Duration)
	}
	p.seekOffset = pos
	p.state.Position = pos
	s := p.state
	p.mu.Unlock()
	p.gst.PlayURI(p.streamURL(gen, pos))
	if !playing {
		p.gst.Pause()
	}
	p.broadcast(s)
	return nil
}

func (p *Player) SetVolume(v float64) error {
	v = min(max(v, 0), 1)
	p.gst.SetVolume(v)
	p.mu.Lock()
	p.state.Volume = v
	s := p.state
	p.mu.Unlock()
	p.broadcast(s)
	return nil
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
	p.playIdx(0)
	return nil
}

func (p *Player) SetPlaylist(_ string, startIdx int) error {
	p.playIdx(startIdx)
	return nil
}

func (p *Player) AppendQueue(ids []string) error {
	p.mu.Lock()
	wasIdle := len(p.queue) == 0 || (p.ended && !p.state.Playing)
	first := len(p.queue)
	p.queue = append(p.queue, ids...)
	p.mu.Unlock()
	if wasIdle && len(ids) > 0 {
		p.playIdx(first)
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
	gains := make([]float64, len(bands))
	for i, b := range bands {
		gains[i] = b.Gain
	}
	p.gst.SetEqualizer(gains)
	return nil
}

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
	p.mu.Unlock()
	if current {
		p.mu.Lock()
		if p.idx >= len(p.queue) {
			p.mu.Unlock()
			return p.Stop()
		}
		i := p.idx
		p.mu.Unlock()
		p.playIdx(i)
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
	switch {
	case from == p.idx:
		p.idx = to
	case from < p.idx && to >= p.idx:
		p.idx--
	case from > p.idx && to <= p.idx:
		p.idx++
	}
	return nil
}

// PlayNext implements player.NextQueuer: insert after the current track.
func (p *Player) PlayNext(ids []string) error {
	p.mu.Lock()
	if len(p.queue) == 0 {
		p.mu.Unlock()
		return p.SetQueue(ids)
	}
	at := min(p.idx+1, len(p.queue))
	p.queue = append(p.queue[:at], append(append([]string(nil), ids...), p.queue[at:]...)...)
	p.mu.Unlock()
	return nil
}

func (p *Player) ClearQueue() error {
	p.gst.Stop()
	p.mu.Lock()
	p.gen++
	p.queue = nil
	p.idx = 0
	p.state.Track = nil
	p.state.Playing = false
	p.state.Loading = false
	s := p.state
	p.mu.Unlock()
	p.broadcast(s)
	return nil
}

func (p *Player) position() time.Duration {
	p.mu.RLock()
	off := p.seekOffset
	p.mu.RUnlock()
	return off + p.gst.Position()
}

func (p *Player) GetState() (*player.State, error) {
	p.mu.RLock()
	s := p.state
	p.mu.RUnlock()
	s.Position = p.position()
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
	p.closeOnce.Do(func() {
		close(p.done)
		p.mu.Lock()
		for _, d := range p.tracks {
			d.cancel()
		}
		p.mu.Unlock()
		_ = p.ln.Close()
		p.gst.Destroy()
	})
	return nil
}

func (p *Player) pollState() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.mu.RLock()
			playing := p.state.Playing
			p.mu.RUnlock()
			if !playing {
				continue
			}
			pos := p.position()
			p.mu.Lock()
			p.state.Position = pos
			s := p.state
			p.mu.Unlock()
			p.broadcast(s)
		case <-p.done:
			return
		}
	}
}

var (
	_ player.Player      = (*Player)(nil)
	_ player.QueueJumper = (*Player)(nil)
	_ player.NextQueuer  = (*Player)(nil)
)
