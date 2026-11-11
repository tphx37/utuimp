package discord

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tphx37/utuimp/internal/player"
)

// DefaultClientID is the registered Discord Application ID for utuimp.
const DefaultClientID = "1553056382482129088"

// clearReassert is how long after a pause the cleared presence is sent
// again. Discord rate-limits SET_ACTIVITY (about 5 per 20s) and can drop
// an update that comes in a quick play/pause burst; the repeat makes sure
// the card does not stay up showing the song as playing.
const clearReassert = 20 * time.Second

// reconnectCooldown is the time to wait before retrying to connect after a failure.
const reconnectCooldown = 15 * time.Second

// Service maintains the Discord Rich Presence session.
type Service struct {
	clientID string
	client   *IPCClient
	logMu    sync.RWMutex
	logger   func(string)
	mu       sync.Mutex

	nonceCounter atomic.Uint64
	lastTrackID  string
	lastPlaying  bool
	lastStart    int64
	lastConnErr  time.Time
	closed       bool
}

// NewService creates a Discord Rich Presence service.
func NewService(clientID string) *Service {
	if clientID == "" {
		clientID = DefaultClientID
	}
	return &Service{
		clientID: clientID,
	}
}

// SetLogger registers a logging callback.
func (s *Service) SetLogger(fn func(string)) {
	s.logMu.Lock()
	s.logger = fn
	s.logMu.Unlock()
}

func (s *Service) log(msg string) {
	s.logMu.RLock()
	fn := s.logger
	s.logMu.RUnlock()
	if fn != nil {
		fn("[discord] " + msg)
	}
}

type setActivityArgs struct {
	Pid      int       `json:"pid"`
	Activity *Activity `json:"activity"`
}

type setActivityCmd struct {
	Cmd   string          `json:"cmd"`
	Args  setActivityArgs `json:"args"`
	Nonce string          `json:"nonce"`
}

// Update updates the Rich Presence based on the player's current state.
func (s *Service) Update(st player.State) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	trackID := ""
	if st.Track != nil {
		trackID = st.Track.ID
	}

	act := BuildActivity(st)

	var currentStart int64
	if act != nil && act.Timestamps != nil {
		currentStart = act.Timestamps.Start
	}

	drift := currentStart - s.lastStart
	if drift < 0 {
		drift = -drift
	}

	s.send(trackID, st.Playing, currentStart, act, drift)
}

// reassertClear sends the cleared presence again if the last one sent was
// a clear (paused or stopped).
func (s *Service) reassertClear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.lastPlaying || s.client == nil {
		return
	}
	s.send(s.lastTrackID, false, 0, nil, -1)
}

// send writes act to Discord unless it repeats what was last sent. A
// negative drift forces the write. Must be called with s.mu held.
func (s *Service) send(trackID string, playing bool, currentStart int64, act *Activity, drift int64) {
	// Avoid re-sending identical presence if track and playing status haven't changed
	// and timestamp hasn't drifted (e.g. from seek or repeat-one loop).
	if trackID == s.lastTrackID && playing == s.lastPlaying && drift >= 0 && drift <= 2 && s.client != nil {
		return
	}

	// Ensure connection
	if err := s.ensureConnected(); err != nil {
		return
	}

	nonce := fmt.Sprintf("%d", s.nonceCounter.Add(1))
	cmd := setActivityCmd{
		Cmd: "SET_ACTIVITY",
		Args: setActivityArgs{
			Pid:      os.Getpid(),
			Activity: act,
		},
		Nonce: nonce,
	}

	payload, err := json.Marshal(cmd)
	if err != nil {
		s.log(fmt.Sprintf("marshal error: %v", err))
		return
	}

	if err := s.client.WriteFrame(OpFrame, payload); err != nil {
		s.log(fmt.Sprintf("write error: %v", err))
		_ = s.client.Close()
		s.client = nil
		s.lastConnErr = time.Now()
		return
	}

	s.lastTrackID = trackID
	s.lastPlaying = playing
	s.lastStart = currentStart

	if act != nil {
		s.log(fmt.Sprintf("presence updated: %s — %s", act.State, act.Details))
	} else {
		s.log("presence cleared")
	}
}

// ensureConnected verifies or establishes an IPC connection to Discord.
// Must be called with s.mu held.
func (s *Service) ensureConnected() error {
	if s.client != nil {
		return nil
	}

	if time.Since(s.lastConnErr) < reconnectCooldown {
		return ErrClosed
	}

	ipc, err := DialIPC()
	if err != nil {
		s.lastConnErr = time.Now()
		return err
	}

	if err := ipc.Handshake(s.clientID); err != nil {
		_ = ipc.Close()
		s.lastConnErr = time.Now()
		s.log(fmt.Sprintf("handshake failed: %v", err))
		return err
	}

	ipc.SetOnFrame(func(op uint32, payload []byte) {
		if op != OpFrame {
			return
		}
		var resp struct {
			Cmd  string `json:"cmd"`
			Evt  string `json:"evt"`
			Data struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"data"`
		}
		if err := json.Unmarshal(payload, &resp); err == nil {
			if resp.Evt == "ERROR" {
				s.log(fmt.Sprintf("%s error (%d): %s", resp.Cmd, resp.Data.Code, resp.Data.Message))
			}
		}
	})

	s.client = ipc
	s.log("connected to Discord IPC")
	return nil
}

// Close closes the Discord IPC connection.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	if s.client != nil {
		// Attempt to clear activity before disconnecting
		nonce := fmt.Sprintf("%d", s.nonceCounter.Add(1))
		cmd := setActivityCmd{
			Cmd: "SET_ACTIVITY",
			Args: setActivityArgs{
				Pid:      os.Getpid(),
				Activity: nil,
			},
			Nonce: nonce,
		}
		if payload, err := json.Marshal(cmd); err == nil {
			_ = s.client.WriteFrame(OpFrame, payload)
		}
		err := s.client.Close()
		s.client = nil
		return err
	}
	return nil
}

// Start launches a goroutine watching player state updates and synchronizing with Discord.
func Start(clientID string, p interface{ Subscribe() <-chan player.State }, log func(string)) *Service {
	s := NewService(clientID)
	if log != nil {
		s.SetLogger(log)
	}
	go func() {
		ch := p.Subscribe()
		t := time.NewTimer(clearReassert)
		t.Stop()
		playing := false
		for {
			select {
			case st, ok := <-ch:
				if !ok {
					t.Stop()
					_ = s.Close()
					return
				}
				s.Update(st)
				if st.Playing {
					t.Stop()
				} else if playing {
					t.Reset(clearReassert)
				}
				playing = st.Playing
			case <-t.C:
				s.reassertClear()
			}
		}
	}()
	return s
}
