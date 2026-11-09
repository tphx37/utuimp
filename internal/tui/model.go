package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tphx37/utuimp/internal/i18n"
	"image"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/tphx37/utuimp/internal/audioquality"
	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/lyrics"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/art"
	"github.com/tphx37/utuimp/internal/tui/styles"
	"github.com/tphx37/utuimp/internal/tui/views"
)

// ── Modal modes (vim-inspired) ────────────────────────────────────────────

type viewMode int

const (
	modeNormal         viewMode = iota
	modeSearch                  // '/' opens — query accumulates in searchQuery
	modeCommand                 // ':' opens — command accumulates in cmdBuf
	modePlaylistPicker          // 'p' in queue / ctrl+p in search
)

// ── ContentView interface ─────────────────────────────────────────────────

// ContentView is the interface every content panel must implement.
// To register a new panel: implement this interface and append to m.panels in New().
// Nothing else in the model needs to change.
type ContentView interface {
	NavKey() string   // normal-mode key to activate this panel
	NavLabel() string // short label shown in the status bar
	SetSize(w, h int)
	Update(msg tea.KeyPressMsg) tea.Cmd
	View() string
	Back() bool
}

// libraryPanel wraps views.LibraryModel to satisfy ContentView.
type libraryPanel struct{ m *views.LibraryModel }

func (p *libraryPanel) NavKey() string   { return "l" }
func (p *libraryPanel) NavLabel() string { return "library" }
func (p *libraryPanel) SetSize(w, h int) { p.m.SetSize(w, h) }
func (p *libraryPanel) Update(msg tea.KeyPressMsg) tea.Cmd {
	updated, cmd := p.m.Update(msg)
	p.m = updated
	return cmd
}
func (p *libraryPanel) View() string  { return p.m.View() }
func (p *libraryPanel) Back() bool    { return p.m.Back() }
func (p *libraryPanel) Init() tea.Cmd { return p.m.Init() }

// queuePanel wraps views.QueueModel to satisfy ContentView.
type queuePanel struct{ m *views.QueueModel }

func (p *queuePanel) NavKey() string   { return "" } // shown on the main screen
func (p *queuePanel) NavLabel() string { return "queue" }
func (p *queuePanel) SetSize(w, h int) { p.m.SetSize(w, h) }
func (p *queuePanel) Update(msg tea.KeyPressMsg) tea.Cmd {
	p.m.Update(msg)
	return nil
}
func (p *queuePanel) View() string                          { return p.m.View() }
func (p *queuePanel) Back() bool                            { return false }
func (p *queuePanel) SetTracks(tracks []provider.Track)     { p.m.SetTracks(tracks) }
func (p *queuePanel) SelectedTrack() (int, *provider.Track) { return p.m.SelectedTrack() }
func (p *queuePanel) Select(idx int)                        { p.m.Select(idx) }

// historyPanel lists recently played songs (the q panel).
type historyPanel struct{ m *views.QueueModel }

func (p *historyPanel) NavKey() string                     { return "q" }
func (p *historyPanel) NavLabel() string                   { return "history" }
func (p *historyPanel) SetSize(w, h int)                   { p.m.SetSize(w, h) }
func (p *historyPanel) Update(msg tea.KeyPressMsg) tea.Cmd { p.m.Update(msg); return nil }
func (p *historyPanel) View() string                       { return p.m.View() }
func (p *historyPanel) Back() bool                         { return false }

// lyricsPanel wraps views.LyricsModel to satisfy ContentView.
type lyricsPanel struct{ m *views.LyricsModel }

func (p *lyricsPanel) NavKey() string                     { return "y" }
func (p *lyricsPanel) NavLabel() string                   { return "lyrics" }
func (p *lyricsPanel) SetSize(w, h int)                   { p.m.SetSize(w, h) }
func (p *lyricsPanel) Update(msg tea.KeyPressMsg) tea.Cmd { return p.m.Update(msg) }
func (p *lyricsPanel) View() string                       { return p.m.View() }
func (p *lyricsPanel) Back() bool                         { return p.m.Back() }

// feedPanel wraps views.FeedModel to satisfy ContentView.
type feedPanel struct{ m *views.FeedModel }

func (p *feedPanel) NavKey() string                     { return "F" }
func (p *feedPanel) NavLabel() string                   { return "feed" }
func (p *feedPanel) SetSize(w, h int)                   { p.m.SetSize(w, h) }
func (p *feedPanel) Update(msg tea.KeyPressMsg) tea.Cmd { return p.m.Update(msg) }
func (p *feedPanel) View() string                       { return p.m.View() }
func (p *feedPanel) Back() bool                         { return false }

// eqPanel wraps views.EQModel to satisfy ContentView.
type eqPanel struct{ m *views.EQModel }

func (p *eqPanel) NavKey() string   { return "e" }
func (p *eqPanel) NavLabel() string { return "equalizer" }
func (p *eqPanel) SetSize(w, h int) { p.m.SetSize(w, h) }
func (p *eqPanel) Update(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "e" {
		return nil
	}
	return p.m.Update(msg)
}
func (p *eqPanel) View() string { return p.m.View() }
func (p *eqPanel) Back() bool   { return false }

// aboutPanel wraps views.AboutModel to satisfy ContentView.
type aboutPanel struct{ m *views.AboutModel }

func (p *aboutPanel) NavKey() string   { return "?" }
func (p *aboutPanel) NavLabel() string { return "about" }
func (p *aboutPanel) SetSize(w, h int) { p.m.SetSize(w, h) }
func (p *aboutPanel) Update(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "?" {
		return nil
	}
	return p.m.Update(msg)
}
func (p *aboutPanel) View() string { return p.m.View() }
func (p *aboutPanel) Back() bool   { return false }

// ── Messages ──────────────────────────────────────────────────────────────

type playerStateMsg player.State
type artworkLoadedMsg struct {
	url string
	gen int
	img image.Image
	err error
}
type searchResultMsg struct {
	result *provider.SearchResult
	query  string
	err    error
}

// searchDebounceMsg is emitted after the debounce delay. The gen field lets
// Update discard messages that belong to earlier keystrokes.
type searchDebounceMsg struct {
	query string
	gen   int
}

// radioResultMsg carries a station refill.
type radioResultMsg struct {
	tracks   []provider.Track
	err      error
	radioGen int // radio generation that produced this result
}
type loveSongMsg struct {
	trackID   string
	catalogID string
	title     string
	loved     bool
	err       error
}
type songRatingMsg struct {
	trackID   string
	catalogID string
	loved     bool
}
type tickMsg time.Time
type glowTickMsg time.Time
type introTickMsg time.Time
type memTickMsg struct{ stats string }
type errMsg struct{ err error }

type artworkCache struct {
	url      string
	img      image.Image
	rendered map[art.Size][]string
	failed   bool
}

// saveVolumeMsg is returned by every volume change (+/-, :vol, :mute). The
// service that made it saves the volume; App passes it on to the other
// services as a volumeSyncMsg, so all players share one volume. muted marks
// :mute, with the volume to come back to in preMute (not saved).
type saveVolumeMsg struct {
	vol     float64
	muted   bool
	preMute float64
}

// volumeSyncMsg sets this service's volume (and mute) to the one just
// chosen in another tab.
type volumeSyncMsg struct {
	vol     float64
	muted   bool
	preMute float64
}
type saveAudioQualityMsg struct {
	kbps      int
	savedOnly bool
}
type playlistCreatedMsg struct{ name string }
type SessionExpiredMsg struct{}
type SessionRestoredMsg struct{}
type lyricsResultMsg struct {
	trackID string
	result  *lyrics.Result
	err     error
}
type lyricsTranslatedMsg struct {
	trackID string
	lines   []string
	source  string // the song's detected language
	err     error
}
type feedResultMsg struct {
	groups []provider.RecommendationGroup
	err    error
}
type feedTracksMsg struct {
	item     provider.RecommendationItem
	tracks   []provider.Track
	play     bool // true = replace queue & play; false = append
	playNext bool
	err      error
}

type artistAlbumsMsg struct {
	artist provider.Artist
	albums []provider.Album
	err    error
}

type searchCollectionTracksMsg struct {
	label    string // album/playlist name for log messages
	tracks   []provider.Track
	play     bool // true = replace queue & play; false = append
	playNext bool
	err      error
}

// introFrames: logo types out letter-by-letter, then holds for 8 frames.
var introFrames = func() []string {
	logo := "♪ utuimp"
	runes := []rune(logo)
	frames := make([]string, 0, len(runes)+16)
	for i := range runes {
		frames = append(frames, string(runes[:i+1]))
	}
	for range 8 {
		frames = append(frames, logo)
	}
	return frames
}()

const introDone = -1

// ── Radio mode ──────────────────────────────────────────────────────────────

// radioMode holds state for the continuous radio feature: a station seeded
// from a track (or Yandex "My Wave"), auto-refilled as the queue runs low.
type radioMode struct {
	enabled        bool
	seed           *provider.Track
	refilling      bool            // background search in progress
	triggeredForID string          // ID of track for which we already fired a search
	skipped        map[string]bool // IDs/keys of tracks skipped due to unavailability
	retries        int             // consecutive failed refill attempts (circuit breaker)
	generation     int             // increments whenever radio starts/stops to ignore stale results
}

const radioMaxRetries = 5 // give up re-arming after this many consecutive failures

// radioLookahead is how many songs radio keeps queued after the current one.
const radioLookahead = 4

// radioTracksAhead counts the queued songs after cur (all of them when cur
// is not in the queue yet).
func (m *Model) radioTracksAhead(cur provider.Track) int {
	id := views.PlaybackID(cur)
	for i, t := range m.queueTracks {
		if views.PlaybackID(t) == id {
			return len(m.queueTracks) - 1 - i
		}
	}
	return len(m.queueTracks)
}

// ── Model ─────────────────────────────────────────────────────────────────

type Model struct {
	cfg      *config.Config
	provider provider.Provider
	player   player.Player

	width, height int

	playerState player.State
	stateCh     <-chan player.State

	// Album art view (:art). artMode mirrors cfg.AlbumArt; the cover is
	// fetched per track and the rendered lines are cached per size so they
	// only re-render on a track change or a resize.
	artMode          bool
	coverView        bool                                 // the frameless cover layout (v), see cover.go
	renderArt        func(image.Image, art.Size) []string // cover renderer, see artRenderer
	artwork          artworkCache
	artworkGen       int
	artHTTP          *http.Client
	supportsArtColor func() bool
	artCellAsp       float64          // terminal cell height/width ratio, for square art
	measureCellAsp   func() float64   // re-reads artCellAsp on resize, see cellAspect
	queueIDs         []string         // current playback queue (for "add to queue")
	queueTracks      []provider.Track // full track objects parallel to queueIDs
	queueMiniOffset  int              // scroll offset for the mini-queue in the split view
	queueMiniCursor  int              // selected row in the mini-queue (-1 = none)

	// Radio mode
	radio radioMode

	// Panels
	panels      []ContentView // registered content panels; add new ones in New()
	activePanel int           // index into panels; -1 = none active
	library     *libraryPanel
	queue       *queuePanel
	lyricsP     *lyricsPanel
	feedP       *feedPanel
	eqP         *eqPanel
	aboutP      *aboutPanel

	// Lyrics
	lyricsClient      *lyrics.Client
	lastLyricsTrackID string // ID of the track for which lyrics were last fetched
	// stateAt is when playerState.Position was last reported, so the lyrics
	// position can be interpolated between player updates (word sync needs
	// finer timing than the player pushes).
	stateAt time.Time
	// favRemoveID / favRemoveAt arm the second f press that un-favourites
	// a track (see favRemoveWindow).
	favRemoveID string
	favRemoveAt time.Time
	// lyricsClock is the position the lyrics are drawn at. It advances with
	// wall time and is nudged toward the player's reported position rather
	// than snapping to it, so karaoke highlighting glides instead of jittering
	// back and forth on every player update (see advanceLyricsClock).
	lyricsClock   time.Duration
	lyricsClockAt time.Time
	lyricsTicking bool // a lyricsTick (fast animation) loop is running
	// seekTarget/seekAt guard against stale positions right after a seek.
	seekTarget time.Duration
	seekAt     time.Time
	// emptyQueueHint replaces "Queue is empty" (Options.EmptyQueueHint).
	emptyQueueHint string
	// background is true while another service's tab is shown; such a model
	// leaves shared styling (the cover palette) alone.
	background bool
	// engineErr is why the service failed to start; shown until it does.
	engineErr string
	// lastWheel / lastWheelAt throttle mouse-wheel bursts.
	lastWheel   rune
	lastWheelAt time.Time
	// songMenu is the song actions window (key i); nil when closed.
	songMenu *songMenuState
	// busyLabel names a background load in progress (an album or playlist
	// being fetched from search or the feed); shown with a spinner.
	busyLabel string
	// trackChangedAt starts the title reveal animation on a new track.
	trackChangedAt time.Time
	noAnim         bool // render final frames only (tests)
	// history holds recently played songs, newest first, persisted to
	// historyPath ("" = not persisted).
	history        []provider.Track
	historyPath    string
	historyLastKey string // title+artist of the last song recorded
	historyP       *historyPanel

	// Search popup (not a panel)
	search *views.SearchModel

	// Modal state
	mode viewMode

	// Search accumulation (mode == modeSearch)
	searchQuery  string
	searchCursor int // rune index of the cursor within searchQuery
	searchGen    int // incremented on every keystroke; used to discard stale results

	// Command accumulation (mode == modeCommand)
	cmdBuf     string
	cmdSuggIdx int // currently highlighted suggestion (0-based)

	// Double-key tracking (for 'gg')
	lastKey string

	// Errors / status messages
	errMsg    string
	errExpiry time.Time

	// Debug log
	debugLog    []string
	debugView   bool
	debugScroll int // lines scrolled up from tail (0 = show latest)

	// Favorites: track IDs that the user has hearted this session.
	favorites map[string]bool

	// Animation
	glowStep   int
	introStep  int    // introDone (-1) when complete
	initStatus string // status text shown on the loading screen

	// Memory profiling (enabled with --mem-profiling)
	memProfiling bool
	memStats     string

	// Mute: preMuteVol holds the volume before mute so it can be restored.
	// -1 means not muted.
	preMuteVol float64

	// Playlist picker (modePlaylistPicker)
	playlistPickerTrack      *provider.Track
	playlistPickerItems      []provider.Playlist
	playlistPickerCursor     int
	playlistPickerLoading    bool
	playlistPickerReturnMode viewMode
	playlistPickerGen        int
}

func New(cfg *config.Config, prov provider.Provider, plyr player.Player, opts Options) *Model {
	if prov == nil {
		// The engine starts later (EngineReadyMsg); until then requests get
		// a "still connecting" error rather than a nil provider.
		prov = provider.NotReady{}
	}
	m := &Model{
		cfg:          cfg,
		provider:     prov,
		player:       plyr,
		activePanel:  -1,
		memProfiling: opts.MemProfiling,
		preMuteVol:   -1,
		artMode:      cfg.AlbumArt,
		coverView:    cfg.Layout == LayoutCover,
		artwork:      artworkCache{rendered: map[art.Size][]string{}},
		artHTTP:      &http.Client{Timeout: 5 * time.Second},
		// Album art needs at least a 256-colour terminal to look reasonable;
		// on 16-colour/ASCII terminals we skip it (and its download) entirely.
		supportsArtColor: art.SupportsColor,
		renderArt:        artRenderer(),
		// Measured cell height/width ratio, so album art renders as a true square.
		artCellAsp:      cellAspect(),
		measureCellAsp:  cellAspect,
		queueMiniCursor: -1,
		emptyQueueHint:  opts.EmptyQueueHint,
	}
	if plyr != nil {
		m.stateCh = plyr.Subscribe()
	}
	m.library = &libraryPanel{m: views.NewLibrary(prov)}
	m.queue = &queuePanel{m: views.NewQueue()}
	m.lyricsP = &lyricsPanel{m: views.NewLyrics()}
	m.lyricsClient = lyrics.NewClient()
	m.useServiceLyrics(prov)
	m.lyricsP.m.SetTranslateLang(m.translateLang())
	m.feedP = &feedPanel{m: views.NewFeed()}
	eqBands := configEQBandsToPlayer(cfg.EQBands)
	m.eqP = &eqPanel{m: views.NewEqualizer(eqBands)}
	m.search = views.NewSearch(prov)
	m.favorites = make(map[string]bool)
	m.aboutP = &aboutPanel{m: views.NewAbout()}
	m.historyP = &historyPanel{m: views.NewHistory()}
	m.historyPath = defaultHistoryPath(opts.HistoryName)
	m.history = loadHistory(m.historyPath)
	m.historyP.m.SetTracks(m.history)
	m.panels = []ContentView{m.library, m.historyP, m.lyricsP, m.feedP, m.eqP, m.aboutP}
	if m.coverView {
		m.activePanel = m.lyricsPanelIdx() // the cover view opens on the lyrics
	}
	for _, line := range opts.InitialLogs {
		m.appendLog(line)
	}
	if opts.Backend != "" {
		m.appendLog("[engine] backend: " + opts.Backend)
	}
	if opts.ScanNotice != "" {
		m.appendLog("[local] " + opts.ScanNotice)
		if len(opts.InitialTracks) == 0 {
			m.errMsg = opts.ScanNotice
			m.errExpiry = time.Now().Add(30 * time.Second)
		}
	}
	if len(opts.InitialTracks) > 0 {
		ids := make([]string, len(opts.InitialTracks))
		for i, t := range opts.InitialTracks {
			ids[i] = views.PlaybackID(t)
		}
		m.queueTracks = append([]provider.Track{}, opts.InitialTracks...)
		m.queueIDs = ids
		m.queue.SetTracks(m.queueTracks)
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tick(),
		glowTick(),
		introTick(),
		// The terminal's real colours let ANSI themes blend smooth gradients
		// (karaoke lyrics); terminals that don't answer just skip blending.
		tea.RequestForegroundColor,
		tea.RequestBackgroundColor,
	}
	if m.provider != nil {
		cmds = append(cmds, m.library.Init())
	}
	if m.stateCh != nil {
		cmds = append(cmds, waitForState(m.stateCh))
	}
	if m.player != nil {
		cmds = append(cmds, m.restoreAudioSettings()...)
	}
	return tea.Batch(cmds...)
}

func (m *Model) restoreAudioSettings() []tea.Cmd {
	var cmds []tea.Cmd
	if m.preMuteVol >= 0 {
		// Muted in another tab before this engine was up.
		cmds = append(cmds, m.playerCmd(func(p player.Player) error { return p.SetVolume(0) }))
	} else if m.cfg.Volume != nil {
		v := m.cfg.VolumeOrDefault()
		cmds = append(cmds, m.playerCmd(func(p player.Player) error {
			return p.SetVolume(v)
		}))
		m.appendLog(fmt.Sprintf("[vol] restored %.0f%% from config", v*100))
	}
	if len(m.cfg.EQBands) > 0 {
		bands := configEQBandsToPlayer(m.cfg.EQBands)
		cmds = append(cmds, m.playerCmd(func(p player.Player) error {
			return p.SetEqualizer(bands)
		}))
		m.appendLog("[eq] restored from config")
	}
	return cmds
}

// ── Timers ────────────────────────────────────────────────────────────────

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func glowTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return glowTickMsg(t) })
}

func introTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg { return introTickMsg(t) })
}

func memTick() tea.Cmd {
	return tea.Tick(3*time.Second, func(_ time.Time) tea.Msg {
		return memTickMsg{stats: collectMemStats()}
	})
}

func waitForState(ch <-chan player.State) tea.Cmd {
	return func() tea.Msg {
		s, ok := <-ch
		if !ok {
			return nil
		}
		// Drain any additional buffered states, keeping only the most recent.
		// During a track transition the player fires a rapid burst of events
		// (paused → buffering → playing). Processing each one floods the event
		// loop and starves glowTick, causing the animation to freeze. Collapsing
		// the burst into a single Update cycle keeps the UI responsive.
		for {
			select {
			case newer, ok := <-ch:
				if !ok {
					return playerStateMsg(s)
				}
				s = newer
			default:
				return playerStateMsg(s)
			}
		}
	}
}

// ── Update ────────────────────────────────────────────────────────────────

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// A terminal can start us before its window has a size: foot reports
		// 80x24 with zero pixels for the first few milliseconds, which leaves
		// the startup measurement at the 2.0 fallback. The resize that follows
		// carries the real pixel geometry, so measure again.
		if m.measureCellAsp != nil {
			m.artCellAsp = m.measureCellAsp()
		}
		inner := max(0, m.width-2)
		contentW := max(0, inner-2)
		panelH := m.panelHeight()
		m.library.SetSize(contentW, panelH)
		m.search.SetSize(contentW, panelH)
		m.lyricsP.SetSize(contentW, panelH)
		m.feedP.SetSize(contentW, panelH)
		m.eqP.SetSize(contentW, panelH)
		m.aboutP.SetSize(contentW, panelH)

	case tickMsg:
		if m.errMsg != "" && time.Now().After(m.errExpiry) {
			m.errMsg = ""
		}
		cmds = append(cmds, tick())

	case glowTickMsg:
		m.glowStep++
		if m.playerState.Track != nil {
			// The same smoothed clock drives the progress line.
			m.advanceLyricsClock()
		}
		if !m.lyricsTicking && m.fastAnimating() {
			m.lyricsTicking = true
			cmds = append(cmds, lyricsTick())
		}
		cmds = append(cmds, glowTick())

	case lyricsTickMsg:
		if m.fastAnimating() {
			if m.playerState.Track != nil {
				m.advanceLyricsClock()
			}
			cmds = append(cmds, lyricsTick())
		} else {
			m.lyricsTicking = false
		}

	case tea.ForegroundColorMsg:
		styles.SetTerminalColors(msg.Color, nil)

	case tea.BackgroundColorMsg:
		styles.SetTerminalColors(nil, msg.Color)

	case introTickMsg:
		if m.introStep != introDone {
			m.introStep++
			if m.introStep >= len(introFrames) {
				if m.player == nil {
					// Hold at last frame until the engine signals ready.
					m.introStep = len(introFrames) - 1
				} else {
					m.introStep = introDone
				}
			}
			cmds = append(cmds, introTick())
		}

	case playerStateMsg:
		wasPlaying := m.playerState.Playing
		s := player.State(msg)
		for _, line := range s.Logs {
			m.appendLog(line)
		}
		s.Logs = nil
		if s.Error != "" {
			if strings.Contains(s.Error, "CONTENT_RESTRICTED") {
				// Track is region-locked or unavailable in this storefront.
				// Log it silently and skip to the next track — same behaviour
				// as any streaming app encountering a restricted title.
				title := "track"
				if s.Track != nil {
					title = views.TrackLabel(*s.Track)
					if s.Track.Artist == "" && s.Track.Title == "" {
						title = s.Track.ID // placeholder queued by ID only
					}
				}
				m.appendLog(fmt.Sprintf("[skip] restricted: %s", title))
				if m.player != nil {
					cmds = append(cmds, m.playerCmd(func(p player.Player) error { return p.Next() }))
				}
			} else {
				m.appendLog("[error] " + s.Error)
				m.errMsg = s.Error
				m.errExpiry = time.Now().Add(4 * time.Second)
			}
			s.Error = ""
		}
		if s.SkippedID != "" {
			// JS silently skipped a track (CONTENT_RESTRICTED / unavailable).
			// Record the ID in the radio skip list so it won't be proposed
			// again this session, purge ALL blacklisted entries from the queue
			// (there may be duplicates from earlier refills), then re-arm the
			// radio so music keeps flowing without interruption.
			skippedID := s.SkippedID
			s.SkippedID = ""
			if m.radio.skipped == nil {
				m.radio.skipped = make(map[string]bool)
			}
			m.radio.skipped[skippedID] = true
			m.purgeSkippedFromQueue()
			if m.radio.enabled && !m.radio.refilling &&
				m.radio.retries < radioMaxRetries {
				m.radio.retries++
				m.radio.triggeredForID = ""
				m.radio.refilling = true
				cmds = append(cmds, m.runRadioSearch())
			} else if m.radio.retries >= radioMaxRetries {
				m.appendLog("[radio] max retries reached — giving up")
			}
		}
		if s.Track != nil {
			m.enrichTrackFromQueue(s.Track)
		}
		if s.Track == nil {
			styles.ClearArtworkPalette()
		}
		if s.Track == nil || s.Track.ArtworkURL == "" {
			if m.artwork.url != "" || m.artwork.img != nil || m.artwork.failed {
				m.artworkGen++
			}
			m.artwork = artworkCache{rendered: map[art.Size][]string{}}
		} else if s.Track.ArtworkURL != m.artwork.url {
			m.artworkGen++
			m.artwork = artworkCache{url: s.Track.ArtworkURL, rendered: map[art.Size][]string{}}
			// Only download covers while the art view is active; toggling
			// :art on fetches the current track's cover on demand.
			if m.artMode || m.coverView {
				cmds = append(cmds, m.fetchArtworkCmd(s.Track.ArtworkURL, m.artworkGen))
			}
		}
		if s.Track != nil && (m.playerState.Track == nil ||
			views.PlaybackID(*m.playerState.Track) != views.PlaybackID(*s.Track) ||
			m.playerState.Track.Title != s.Track.Title) {
			m.applyTrackPalette(s.Track)
			m.trackChangedAt = time.Now()
			m.appendLog("[playing] " + views.TrackLabel(*s.Track))
			// Log playParams so we can confirm which ID path is used.
			trackType := "catalog"
			if strings.HasPrefix(s.Track.ID, "i.") {
				trackType = "library"
			}
			pp := fmt.Sprintf("[playParams] id=%s type=%s", s.Track.ID, trackType)
			if s.Track.CatalogID != "" && s.Track.CatalogID != s.Track.ID {
				pp += " catalogId=" + s.Track.CatalogID
			}
			m.appendLog(pp)
			// Check whether the new track is already loved on Apple Music.
			cmds = append(cmds, m.checkSongRatingCmd(s.Track))
			// visible, otherwise mark stale so the fetch is deferred until
			// the user opens the panel (lazy loading).
			if id := views.PlaybackID(*s.Track); id != m.lastLyricsTrackID {
				lyricsOpen := m.activePanel >= 0 && m.panels[m.activePanel] == m.lyricsP
				if lyricsOpen {
					m.lastLyricsTrackID = id
					m.lyricsP.m.SetLoading()
					cmds = append(cmds, m.fetchLyricsCmd(s.Track))
				} else {
					m.lastLyricsTrackID = "" // stale; will fetch on panel open
				}
			}
			// Auto-scroll mini-queue to keep the current track visible.
			for i, t := range m.queueTracks {
				if t.Title == s.Track.Title {
					visibleRows := max(0, m.panelHeight()-2)
					if visibleRows > 0 && (i < m.queueMiniOffset || i >= m.queueMiniOffset+visibleRows) {
						m.queueMiniOffset = max(0, i-visibleRows/2)
					}
					break
				}
			}
		}
		// Radio: keep at least radioLookahead songs queued after the current
		// one, so the queue always shows what comes next.
		// triggeredForID makes it fire once per track (a refill that comes
		// back short asks again by itself, see radioResultMsg).
		if m.radio.enabled && !m.radio.refilling && s.Track != nil &&
			m.radioTracksAhead(*s.Track) < radioLookahead &&
			m.radio.triggeredForID != s.Track.ID {
			m.radio.triggeredForID = s.Track.ID
			m.radio.refilling = true
			cmds = append(cmds, m.runRadioSearch())
		}
		m.playerState = s
		m.stateAt = time.Now()
		// History: a song counts once it actually plays (the UI may show a
		// track before the player starts it, so a "track changed" check
		// alone misses songs started from the library).
		if s.Track != nil && s.Playing && !s.Loading {
			if key := s.Track.Title + "\x00" + s.Track.Artist; key != m.historyLastKey {
				m.historyLastKey = key
				m.recordHistory(*s.Track)
			}
		}
		// Keep the current lyrics line right even while the panel is closed.
		if s.Track != nil {
			m.advanceLyricsClock()
		}
		if !wasPlaying && m.playerState.Playing {
			m.appendLog("[player] playing")
		} else if wasPlaying && !m.playerState.Playing && !m.playerState.Loading {
			m.appendLog("[player] paused")
		}
		if m.stateCh != nil {
			cmds = append(cmds, waitForState(m.stateCh))
		}

	case artworkLoadedMsg:
		if msg.url != m.artwork.url || msg.gen != m.artworkGen {
			break
		}
		if msg.err != nil {
			m.artwork.failed = true
			m.artwork.img = nil
			m.artwork.rendered = map[art.Size][]string{}
			m.appendLog(fmt.Sprintf("[art] artwork unavailable: %v", msg.err))
			break
		}
		m.artwork.img = msg.img
		m.artwork.failed = false
		m.artwork.rendered = map[art.Size][]string{}

	case songRatingMsg:
		// Update favorite state to match what Apple Music reports.
		if msg.trackID != "" {
			m.favorites[msg.trackID] = msg.loved
		}
		if msg.catalogID != "" {
			m.favorites[msg.catalogID] = msg.loved
		}
	case DebugLogMsg:
		m.appendLog(string(msg))

	case lyricsResultMsg:
		// Discard stale results if the user skipped to a different track.
		if msg.trackID == m.lastLyricsTrackID {
			m.lyricsP.m.SetLyrics(msg.result, msg.err)
			if msg.err != nil {
				m.appendLog(fmt.Sprintf("[lyrics] not found: %v", msg.err))
			} else {
				m.lyricsClockAt = time.Time{}
				m.advanceLyricsClock()
				m.appendLog(fmt.Sprintf("[lyrics] loaded from %s (synced=%v, word=%v)",
					msg.result.Source.Label(), msg.result.Synced, msg.result.WordSynced))
				if m.lyricsP.m.NeedsTranslation() {
					cmds = append(cmds, m.translateLyricsCmd())
				}
			}
		}

	case lyricsTranslatedMsg:
		if msg.trackID == m.lastLyricsTrackID {
			m.lyricsP.m.SetTranslationSource(msg.source)
			m.lyricsP.m.SetTranslation(msg.lines, msg.err)
			if msg.err != nil {
				m.appendLog(fmt.Sprintf("[lyrics] translation: %v", msg.err))
			}
		}

	case views.LyricsLangSelectedMsg:
		if m.cfg != nil {
			m.cfg.TranslateLang = msg.Code
			if err := m.cfg.Save(""); err != nil {
				m.appendLog(fmt.Sprintf("[config] save failed: %v", err))
			}
		}
		m.appendLog("[lyrics] translate into " + msg.Code)
		if m.lyricsP.m.NeedsTranslation() {
			cmds = append(cmds, m.translateLyricsCmd())
		}

	case songDetailsMsg:
		m.handleSongDetails(msg)

	case songMenuAlbumMsg:
		m.handleSongMenuAlbum(msg)

	case views.LyricsTranslateRequestMsg:
		if m.lyricsP.m.NeedsTranslation() {
			cmds = append(cmds, m.translateLyricsCmd())
		}

	case views.LyricsSourceSelectedMsg:
		if t := m.playerState.Track; t != nil {
			id := views.PlaybackID(*t)
			if m.cfg != nil {
				m.cfg.SetLyricsSource(id, string(msg.Source))
				if err := m.cfg.Save(""); err != nil {
					m.appendLog(fmt.Sprintf("[lyrics] could not save source choice: %v", err))
				}
			}
			m.appendLog("[lyrics] source → " + msg.Source.Label())
			m.lastLyricsTrackID = id
			m.lyricsP.m.SetLoading()
			cmds = append(cmds, m.fetchLyricsCmd(t))
		}

	case views.LyricsSeekMsg:
		m.noteSeek(msg.Position)
		m.appendLog(fmt.Sprintf("[lyrics] seek → %s", views.FormatDuration(msg.Position)))
		pos := msg.Position
		cmds = append(cmds, m.playerCmd(func(p player.Player) error { return p.Seek(pos) }))

	case feedResultMsg:
		if msg.err != nil {
			m.feedP.m.SetError(msg.err)
			m.appendLog(fmt.Sprintf("[feed] error: %v", msg.err))
		} else {
			m.feedP.m.SetRecommendations(msg.groups)
			m.appendLog(fmt.Sprintf("[feed] loaded %d groups", len(msg.groups)))
		}

	case views.EQChangeMsg:
		if m.player != nil {
			if err := m.player.SetEqualizer(msg.Bands); err != nil {
				m.appendLog(fmt.Sprintf("[eq] error: %v", err))
			}
		}
		m.cfg.EQBands = playerEQBandsToConfig(msg.Bands)
		if err := m.cfg.Save(""); err != nil {
			m.appendLog(fmt.Sprintf("[eq] config save error: %v", err))
		}

	case feedTracksMsg:
		m.busyLabel = ""
		if msg.err != nil {
			m.errMsg = fmt.Sprintf(i18n.T("feed: %v"), msg.err)
			m.errExpiry = time.Now().Add(4 * time.Second)
			m.appendLog(fmt.Sprintf("[feed] track fetch error: %v", msg.err))
			break
		}
		if len(msg.tracks) == 0 {
			m.errMsg = i18n.T("feed: no playable tracks")
			m.errExpiry = time.Now().Add(3 * time.Second)
			break
		}
		ids := make([]string, len(msg.tracks))
		for i, t := range msg.tracks {
			ids[i] = views.PlaybackID(t)
		}
		if msg.play {
			m.queueTracks = msg.tracks
			m.queueIDs = ids
			m.queue.SetTracks(m.queueTracks)
			m.appendLog(fmt.Sprintf("[feed] playing %q (%d tracks)", msg.item.Title, len(ids)))
			return m, m.playerCmd(func(p player.Player) error { return p.SetQueue(ids) })
		}
		if msg.playNext {
			return m, m.playNextCmd(msg.item.Title, msg.tracks, ids)
		}
		m.queueTracks = append(m.queueTracks, msg.tracks...)
		m.queueIDs = append(m.queueIDs, ids...)
		m.queue.SetTracks(m.queueTracks)
		m.appendLog(fmt.Sprintf("[feed] queued %q (%d tracks)", msg.item.Title, len(ids)))
		return m, m.playerCmd(func(p player.Player) error { return p.AppendQueue(ids) })

	case radioResultMsg:
		if !m.radio.enabled || msg.radioGen != m.radio.generation {
			break
		}
		if msg.err != nil {
			m.appendLog(fmt.Sprintf("[radio] search error: %v", msg.err))
			m.radio.refilling = false
			break
		}
		// Drop tracks skipped while the request was in flight and ones
		// already queued (by ID and by artist||title).
		var tracks []provider.Track
		for _, t := range msg.tracks {
			id := views.PlaybackID(t)
			key := strings.ToLower(t.Artist + "||" + t.Title)
			if m.radio.skipped[id] {
				continue
			}
			dup := slices.Contains(m.queueIDs, id)
			for _, qt := range m.queueTracks {
				if dup || strings.ToLower(qt.Artist+"||"+qt.Title) == key {
					dup = true
					break
				}
			}
			if !dup {
				tracks = append(tracks, t)
			}
		}
		if len(tracks) == 0 {
			if m.radio.retries < radioMaxRetries {
				m.radio.retries++
				m.radio.refilling = true
				cmds = append(cmds, m.runRadioSearch())
			} else {
				m.radio.refilling = false
			}
			break
		}
		ids := make([]string, len(tracks))
		for i, t := range tracks {
			ids[i] = views.PlaybackID(t)
		}
		if m.player != nil {
			if err := m.player.AppendQueue(ids); err != nil {
				m.appendLog(fmt.Sprintf("[radio] queue error: %v", err))
				m.radio.refilling = false
				break
			}
		}
		m.queueTracks = append(m.queueTracks, tracks...)
		m.queueIDs = append(m.queueIDs, ids...)
		m.queue.SetTracks(m.queueTracks)
		m.radio.refilling = false
		m.radio.retries = 0 // successful refill — reset circuit breaker
		m.appendLog(fmt.Sprintf("[radio] refilled %d tracks", len(tracks)))
		// Stations often hand out a song or two at a time: top up until
		// radioLookahead songs wait after the current one.
		if cur := m.playerState.Track; cur != nil && m.radioTracksAhead(*cur) < radioLookahead {
			m.radio.refilling = true
			cmds = append(cmds, m.runRadioSearch())
		}

	case loveSongMsg:
		if msg.err != nil {
			// Revert optimistic toggle on failure.
			if msg.trackID != "" {
				m.favorites[msg.trackID] = !msg.loved
			}
			if msg.catalogID != "" {
				m.favorites[msg.catalogID] = !msg.loved
			}
			m.errMsg = fmt.Sprintf(i18n.T("favorite failed: %v"), msg.err)
			m.errExpiry = time.Now().Add(4 * time.Second)
			m.appendLog(fmt.Sprintf("[fav] ✗ %s: %v", msg.title, msg.err))
		} else {
			if msg.trackID != "" {
				m.favorites[msg.trackID] = msg.loved
			}
			if msg.catalogID != "" {
				m.favorites[msg.catalogID] = msg.loved
			}
			m.library.m.InvalidateTracksCache()
			state := "♡"
			if msg.loved {
				state = "♥"
			}
			m.appendLog(fmt.Sprintf("[fav] %s %s synced to Apple Music", state, msg.title))
		}
	case searchDebounceMsg:
		// Drop stale debounce ticks — only the latest keystroke wins.
		if msg.gen != m.searchGen || m.provider == nil {
			return m, nil
		}
		m.appendLog(fmt.Sprintf("[search] %q…", msg.query))
		prov := m.provider
		query := msg.query
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			result, err := prov.Search(ctx, query)
			return searchResultMsg{result: result, query: query, err: err}
		}

	case searchResultMsg:
		if msg.query != m.searchQuery {
			break
		}
		if msg.err != nil {
			m.appendLog(fmt.Sprintf("[search] error: %v", msg.err))
			m.search.SetResults(nil, false, msg.err)
		} else {
			if msg.result != nil {
				if len(msg.result.Warnings) > 0 {
					m.appendLog(fmt.Sprintf("[search] partial results: %s",
						strings.Join(msg.result.Warnings, "; ")))
				}
				m.appendLog(fmt.Sprintf("[search] %d track(s), %d album(s), %d playlist(s)",
					len(msg.result.Tracks), len(msg.result.Albums), len(msg.result.Playlists)))
			}
			m.search.SetResults(msg.result, false, nil)
		}

	case artistAlbumsMsg:
		if msg.err != nil {
			m.appendLog(fmt.Sprintf("[search] artist %q albums error: %v", msg.artist.Name, msg.err))
		} else {
			m.appendLog(fmt.Sprintf("[search] artist %q: %d album(s)", msg.artist.Name, len(msg.albums)))
		}
		m.search.SetArtistAlbums(msg.artist.ID, msg.albums, msg.err)

	case searchCollectionTracksMsg:
		m.busyLabel = ""
		if msg.err != nil {
			m.errMsg = fmt.Sprintf(i18n.T("search: %v"), msg.err)
			m.errExpiry = time.Now().Add(4 * time.Second)
			m.appendLog(fmt.Sprintf("[search] collection fetch error: %v", msg.err))
			break
		}
		if len(msg.tracks) == 0 {
			m.errMsg = i18n.T("no playable tracks found")
			m.errExpiry = time.Now().Add(3 * time.Second)
			break
		}
		ids := make([]string, len(msg.tracks))
		for i, t := range msg.tracks {
			ids[i] = views.PlaybackID(t)
		}
		if msg.play {
			m.queueTracks = msg.tracks
			m.queueIDs = ids
			m.queue.SetTracks(m.queueTracks)
			m.appendLog(fmt.Sprintf("[search] playing %q (%d tracks)", msg.label, len(ids)))
			m.mode = modeNormal
			m.playerState.Loading = true
			m.playerState.Playing = false
			m.playerState.Position = 0
			return m, m.playerCmd(func(p player.Player) error { return p.SetQueue(ids) })
		}
		if msg.playNext {
			m.mode = modeNormal
			return m, m.playNextCmd(msg.label, msg.tracks, ids)
		}
		m.queueTracks = append(m.queueTracks, msg.tracks...)
		m.queueIDs = append(m.queueIDs, ids...)
		m.queue.SetTracks(m.queueTracks)
		m.appendLog(fmt.Sprintf("[search] queued %q (%d tracks)", msg.label, len(ids)))
		return m, m.playerCmd(func(p player.Player) error { return p.AppendQueue(ids) })

	case views.QueueTracksMsg:
		if len(msg.Tracks) == 0 || len(msg.IDs) == 0 {
			break
		}
		if msg.PlayNext {
			return m, m.playNextCmd(msg.Label, msg.Tracks, msg.IDs)
		}
		m.queueTracks = append(m.queueTracks, msg.Tracks...)
		m.queueIDs = append(m.queueIDs, msg.IDs...)
		m.queue.SetTracks(m.queueTracks)
		m.appendLog(fmt.Sprintf("[library] queued %q (%d tracks)", msg.Label, len(msg.IDs)))
		return m, m.playerCmd(func(p player.Player) error { return p.AppendQueue(msg.IDs) })

	case views.PlayTracksMsg:
		if msg.Track != nil {
			m.playerState.Track = msg.Track
			m.applyTrackPalette(msg.Track)
		}
		m.playerState.Loading = true
		m.playerState.Playing = false
		m.playerState.Position = 0
		start := msg.StartIdx
		if msg.PlaylistID != "" || start < 0 || start >= len(msg.IDs) {
			start = 0
		}
		_, canJump := m.player.(player.QueueJumper)
		sameQueue := slices.Equal(msg.IDs, m.queueIDs)
		tracks := msg.Tracks
		if len(tracks) == 0 && msg.Track != nil {
			tracks = []provider.Track{*msg.Track}
		}
		ids := msg.IDs
		if start > 0 && !canJump && msg.PlaylistID == "" {
			// No way to start mid-queue: fall back to starting the queue
			// at the selection.
			ids, tracks, start = ids[start:], tracks[min(start, len(tracks)):], 0
		}
		m.queueTracks = tracks
		m.queueIDs = ids
		m.queue.SetTracks(m.queueTracks)
		m.queueMiniCursor = -1
		cmds = append(cmds, m.playerCmd(func(p player.Player) error {
			if msg.PlaylistID != "" {
				return p.SetPlaylist(msg.PlaylistID, msg.StartIdx)
			}
			j, ok := p.(player.QueueJumper)
			if sameQueue && ok {
				// The list is already what's playing: just move within it.
				return j.PlayAt(start)
			}
			if err := p.SetQueue(ids); err != nil {
				return err
			}
			if start > 0 && ok {
				return j.PlayAt(start)
			}
			return nil
		}))
		m.mode = modeNormal
		m.activePanel = -1

	case InitStatusMsg:
		m.initStatus = string(msg)

	case EngineReadyMsg:
		m.player = msg.Player
		m.provider = msg.Provider
		m.engineErr = ""
		m.useServiceLyrics(msg.Provider)
		m.stateCh = msg.Player.Subscribe()
		m.library.m = views.NewLibrary(msg.Provider)
		// Re-apply the window size that was stored from the earlier WindowSizeMsg,
		// since the new inner LibraryModel starts with zero dimensions.
		m.search = views.NewSearch(msg.Provider)
		if m.width > 0 {
			inner := max(0, m.width-2)
			contentW := max(0, inner-2)
			panelH := m.panelHeight()
			m.library.SetSize(contentW, panelH)
			m.search.SetSize(contentW, panelH)
		}
		if m.searchQuery != "" {
			cmds = append(cmds, m.scheduleSearch(m.searchQuery))
		}
		m.appendLog("[engine] backend: " + msg.Backend)
		if len(msg.Tracks) > 0 && len(m.queueTracks) == 0 {
			m.queueTracks = append([]provider.Track{}, msg.Tracks...)
			m.queueIDs = make([]string, len(msg.Tracks))
			for i, t := range msg.Tracks {
				m.queueIDs[i] = views.PlaybackID(t)
			}
			m.queue.SetTracks(m.queueTracks)
		}
		if msg.Notice != "" {
			m.appendLog("[engine] " + msg.Notice)
			if len(msg.Tracks) == 0 {
				m.errMsg = msg.Notice
				m.errExpiry = time.Now().Add(30 * time.Second)
			}
		}
		cmds = append(cmds, waitForState(m.stateCh), m.library.Init())
		cmds = append(cmds, m.restoreAudioSettings()...)
		if m.memProfiling {
			cmds = append(cmds, memTick())
		}

	case memTickMsg:
		m.memStats = msg.stats
		if m.memProfiling {
			return m, memTick()
		}

	case InitErrMsg:
		m.appendLog("[init error] " + msg.Err.Error())
		m.engineErr = msg.Err.Error()
		m.errMsg = msg.Err.Error()
		m.errExpiry = time.Now().Add(30 * time.Second)
		m.introStep = introDone

	case saveVolumeMsg:
		if !msg.muted {
			m.cfg.SetVolume(msg.vol)
			if err := m.cfg.Save(""); err != nil {
				m.appendLog(fmt.Sprintf("[vol] config save error: %v", err))
			}
		}

	case volumeSyncMsg:
		m.preMuteVol = -1
		if msg.muted {
			m.preMuteVol = msg.preMute
		}
		m.playerState.Volume = msg.vol
		if p := m.player; p != nil {
			vol := msg.vol
			cmds = append(cmds, func() tea.Msg {
				if err := p.SetVolume(vol); err != nil {
					return errMsg{err}
				}
				return nil
			})
		}

	case saveAudioQualityMsg:
		if err := m.cfg.SetAudioBitrate(msg.kbps); err != nil {
			m.appendLog(fmt.Sprintf("[quality] config error: %v", err))
			break
		}
		if err := m.cfg.Save(""); err != nil {
			m.appendLog(fmt.Sprintf("[quality] config save error: %v", err))
			break
		}
		if msg.savedOnly {
			m.errMsg = fmt.Sprintf(i18n.T("✓ Audio quality saved: %d kbps AAC (used next launch; current backend cannot switch live)"), msg.kbps)
		} else {
			m.errMsg = fmt.Sprintf(i18n.T("✓ Audio quality saved: %d kbps AAC"), msg.kbps)
		}
		m.errExpiry = time.Now().Add(3 * time.Second)

	case errMsg:
		m.appendLog("[error] " + msg.err.Error())
		m.errMsg = i18n.T(msg.err.Error())
		if errors.Is(msg.err, errNotConnected) && m.engineErr != "" {
			m.errMsg = m.engineErr
		}
		m.errExpiry = time.Now().Add(3 * time.Second)

	case playlistCreatedMsg:
		m.errMsg = i18n.T("✓ Playlist \"") + msg.name + "\" saved"
		m.errExpiry = time.Now().Add(4 * time.Second)

	case playlistsForPickerMsg:
		if msg.gen != m.playlistPickerGen {
			break // stale response; a newer fetch is in flight
		}
		m.playlistPickerLoading = false
		if msg.err != nil {
			m.mode = m.playlistPickerReturnMode
			m.errMsg = i18n.T("could not load playlists")
			m.errExpiry = time.Now().Add(3 * time.Second)
			m.appendLog(fmt.Sprintf("[playlist] fetch error: %v", msg.err))
			break
		}
		m.playlistPickerItems = m.playlistPickerItems[:0]
		for _, pl := range msg.playlists {
			if !pl.ReadOnly {
				m.playlistPickerItems = append(m.playlistPickerItems, pl)
			}
		}
		if m.playlistPickerCursor >= len(m.playlistPickerItems) {
			m.playlistPickerCursor = 0
		}

	case trackAddedToPlaylistMsg:
		if msg.err != nil {
			m.errMsg = fmt.Sprintf(i18n.T("✗ add to playlist: %v"), msg.err)
			m.appendLog(fmt.Sprintf("[playlist] add error: %v", msg.err))
		} else {
			m.errMsg = fmt.Sprintf(i18n.T("✓ Added to \"%s\""), msg.playlistName)
			m.appendLog(fmt.Sprintf("[playlist] added to %q", msg.playlistName))
		}
		m.errExpiry = time.Now().Add(3 * time.Second)

	case SessionExpiredMsg:
		m.errMsg = i18n.T("Session expired — opening browser to re-authenticate…")
		m.errExpiry = time.Now().Add(365 * 24 * time.Hour) // persists until restored

	case SessionRestoredMsg:
		m.errMsg = i18n.T("✓ Re-authenticated with Apple Music")
		m.errExpiry = time.Now().Add(5 * time.Second)

	case RestartMsg:
		return m, tea.Quit

	case tea.PasteMsg:
		cmd := m.handlePaste(msg.Content)
		cmds = append(cmds, cmd)

	case tea.KeyPressMsg:
		cmd := m.handleKey(msg)
		cmds = append(cmds, cmd)

	case tea.MouseWheelMsg:
		// One notch = one step. Touchpads and high-resolution wheels send
		// bursts; events closer than wheelGap in the same direction are
		// dropped.
		var code rune
		switch msg.Button {
		case tea.MouseWheelUp:
			code = tea.KeyUp
		case tea.MouseWheelDown:
			code = tea.KeyDown
		default:
			break
		}
		if code == 0 {
			break
		}
		now := time.Now()
		if code == m.lastWheel && now.Sub(m.lastWheelAt) < wheelGap {
			break
		}
		m.lastWheel, m.lastWheelAt = code, now
		cmds = append(cmds, m.handleKey(tea.KeyPressMsg{Code: code}))

	default:
		// Forward library background loads.
		prevDrillErr := m.library.m.DrillErr()
		prevLoadErr := m.library.m.LoadErr()
		updated, libCmd := m.library.m.Update(msg)
		if err := updated.DrillErr(); err != nil && prevDrillErr == nil {
			m.appendLog("[library] playlist tracks error: " + err.Error())
		}
		if err := updated.LoadErr(); err != nil && prevLoadErr == nil {
			m.appendLog("[library] load error: " + err.Error())
		}
		m.library.m = updated
		cmds = append(cmds, libCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()

	// ctrl+c always quits
	if k == "ctrl+c" {
		if m.player != nil {
			_ = m.player.Close()
		}
		return tea.Quit
	}

	if m.songMenu != nil {
		return m.handleSongMenuKey(k)
	}

	switch m.mode {
	case modeSearch:
		return m.handleSearchKey(k, msg)
	case modeCommand:
		return m.handleCommandKeyMsg(k, msg)
	case modePlaylistPicker:
		return m.handlePlaylistPickerKey(k)
	default:
		return m.handleNormalKey(msg, k)
	}
}

func (m *Model) handlePaste(content string) tea.Cmd {
	switch {
	case m.mode == modeSearch:
		return m.insertSearchText(content, true)
	case m.mode == modeCommand:
		m.insertCommandText(content, true)
		return nil
	default:
		return nil
	}
}

func (m *Model) handleSearchKey(k string, msg tea.KeyPressMsg) tea.Cmd {
	switch k {
	case "esc":
		if m.search.InArtist() {
			m.search.CloseArtist()
			return nil
		}
		m.mode = modeNormal
		return nil
	case "enter":
		// Artist: open their albums in place of the results.
		if a := m.search.SelectedArtist(); a != nil {
			return m.openArtistCmd(*a)
		}
		// Track: play now — replaces queue and starts immediately.
		if t := m.search.SelectedTrack(); t != nil {
			tc := *t
			m.appendLog("[queue] play now: " + views.TrackLabel(tc))
			m.queueTracks = []provider.Track{tc}
			m.queueIDs = []string{views.PlaybackID(tc)}
			m.playerState.Track = &tc
			m.playerState.Loading = true
			m.playerState.Playing = false
			m.playerState.Position = 0
			m.queue.SetTracks(m.queueTracks)
			m.mode = modeNormal
			return m.playerCmd(func(p player.Player) error { return p.SetQueue(m.queueIDs) })
		}
		// Album: fetch all tracks then play.
		if a := m.search.SelectedAlbum(); a != nil {
			m.playerState.Loading = true
			return m.fetchSearchCollectionCmd(a, nil, true, false)
		}
		// Playlist: fetch all tracks then play.
		if p := m.search.SelectedPlaylist(); p != nil {
			m.playerState.Loading = true
			return m.fetchSearchCollectionCmd(nil, p, true, false)
		}
		return nil
	case "tab":
		// Track: add to queue — appends without interrupting playback.
		if t := m.search.SelectedTrack(); t != nil {
			tc := *t
			m.appendLog("[queue] append: " + views.TrackLabel(tc))
			m.queueTracks = append(m.queueTracks, tc)
			m.queueIDs = append(m.queueIDs, views.PlaybackID(tc))
			m.queue.SetTracks(m.queueTracks)
			return m.playerCmd(func(p player.Player) error { return p.AppendQueue([]string{views.PlaybackID(tc)}) })
		}
		// Album: fetch all tracks then add to queue.
		if a := m.search.SelectedAlbum(); a != nil {
			return m.fetchSearchCollectionCmd(a, nil, false, false)
		}
		// Playlist: fetch all tracks then add to queue.
		if p := m.search.SelectedPlaylist(); p != nil {
			return m.fetchSearchCollectionCmd(nil, p, false, false)
		}
		return nil
	case "shift+tab":
		// Track: play next.
		if t := m.search.SelectedTrack(); t != nil {
			tc := *t
			return m.playNextCmd(views.TrackLabel(tc), []provider.Track{tc}, []string{views.PlaybackID(tc)})
		}
		// Album: fetch all tracks then play next.
		if a := m.search.SelectedAlbum(); a != nil {
			return m.fetchSearchCollectionCmd(a, nil, false, true)
		}
		// Playlist: fetch all tracks then play next.
		if p := m.search.SelectedPlaylist(); p != nil {
			return m.fetchSearchCollectionCmd(nil, p, false, true)
		}
		return nil
	case "ctrl+r":
		// Start radio seeded by the selected search track. A bare "r" would
		// steal a letter from the live search query, so this mirrors ctrl+p
		// (playlist picker) below.
		if t := m.search.SelectedTrack(); t != nil {
			return m.startRadioFrom(t)
		}
		return nil
	case "up", "down", "pgup", "pgdown":
		_, cmd := m.search.Update(msg)
		return cmd
	case "left":
		runes := []rune(m.searchQuery)
		m.searchCursor = min(max(m.searchCursor, 0), len(runes))
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return nil
	case "right":
		runes := []rune(m.searchQuery)
		m.searchCursor = min(max(m.searchCursor, 0), len(runes))
		if m.searchCursor < len(runes) {
			m.searchCursor++
		}
		return nil
	case "home", "ctrl+a":
		m.searchCursor = 0
		return nil
	case "end", "ctrl+e":
		m.searchCursor = len([]rune(m.searchQuery))
		return nil
	case "backspace":
		runes := []rune(m.searchQuery)
		m.searchCursor = min(max(m.searchCursor, 0), len(runes))
		if m.searchCursor > 0 {
			runes = append(runes[:m.searchCursor-1], runes[m.searchCursor:]...)
			m.searchQuery = string(runes)
			m.searchCursor--
			return m.scheduleSearch(m.searchQuery)
		}
		return nil
	case "delete":
		runes := []rune(m.searchQuery)
		m.searchCursor = min(max(m.searchCursor, 0), len(runes))
		if m.searchCursor < len(runes) {
			runes = append(runes[:m.searchCursor], runes[m.searchCursor+1:]...)
			m.searchQuery = string(runes)
			return m.scheduleSearch(m.searchQuery)
		}
		return nil
	case "ctrl+w":
		// Delete word before cursor.
		runes := []rune(m.searchQuery)
		m.searchCursor = min(max(m.searchCursor, 0), len(runes))
		if m.searchCursor > 0 {
			i := m.searchCursor - 1
			for i > 0 && runes[i-1] != ' ' {
				i--
			}
			runes = append(runes[:i], runes[m.searchCursor:]...)
			m.searchQuery = string(runes)
			m.searchCursor = i
			return m.scheduleSearch(m.searchQuery)
		}
		return nil
	case "ctrl+u":
		// Delete everything before cursor.
		runes := []rune(m.searchQuery)
		m.searchCursor = min(max(m.searchCursor, 0), len(runes))
		m.searchQuery = string(runes[m.searchCursor:])
		m.searchCursor = 0
		return m.scheduleSearch(m.searchQuery)
	case "ctrl+p":
		// Open playlist picker for the currently selected search result.
		if t := m.search.SelectedTrack(); t != nil {
			return m.openPlaylistPicker(t)
		}
		return nil
	case "space":
		return m.insertSearchText(" ", false)
	default:
		if text := extractPrintableKeyText(k, msg); text != "" {
			return m.insertSearchText(text, false)
		}
	}
	return nil
}

const maxInputRunes = views.MaxInputRunes

func sanitizeSingleLineInput(s string, maxRunes int, collapseNewlines bool) []rune {
	return views.SanitizeSingleLineInput(s, maxRunes, collapseNewlines)
}

func extractPrintableKeyText(k string, msg tea.KeyPressMsg) string {
	if msg.Mod&tea.ModCtrl != 0 {
		return ""
	}
	if msg.Text != "" {
		return msg.Text
	}
	if msg.Mod == 0 && utf8.RuneCountInString(k) == 1 {
		return k
	}
	return ""
}

func (m *Model) insertSearchText(raw string, collapseNewlines bool) tea.Cmd {
	runes := []rune(m.searchQuery)
	m.searchCursor = min(max(m.searchCursor, 0), len(runes))
	remaining := maxInputRunes - len(runes)
	if remaining <= 0 {
		return nil
	}
	ins := sanitizeSingleLineInput(raw, remaining, collapseNewlines)
	if len(ins) == 0 {
		return nil
	}
	next := make([]rune, 0, len(runes)+len(ins))
	next = append(next, runes[:m.searchCursor]...)
	next = append(next, ins...)
	next = append(next, runes[m.searchCursor:]...)
	m.searchQuery = string(next)
	m.searchCursor += len(ins)
	return m.scheduleSearch(m.searchQuery)
}

func (m *Model) insertCommandText(raw string, collapseNewlines bool) {
	curLen := utf8.RuneCountInString(m.cmdBuf)
	remaining := maxInputRunes - curLen
	if remaining <= 0 {
		return
	}
	ins := sanitizeSingleLineInput(raw, remaining, collapseNewlines)
	if len(ins) == 0 {
		return
	}
	m.cmdBuf += string(ins)
	m.cmdSuggIdx = 0
}

// ── Command palette ───────────────────────────────────────────────────────

// cmdEntry describes a single command shown in the command palette.
type cmdEntry struct {
	trigger     string // prefix matched against cmdBuf
	usage       string // full usage shown to the user
	description string
}

// allCommands is the master list shown in the command palette.
var allCommands = []cmdEntry{
	{"save", "save <name>", "Save the queue as a playlist"},
	{"seek", "seek <seconds>", "Jump to a position in the current song"},
	{"vol", "vol <0-100|+n|-n>", "Set, raise, or lower volume (e.g. vol 80, vol +10, vol -5)"},
	{"quality", "quality <high|standard|256|64>", "Set the Apple Music bitrate"},
	{"art", "art", "Toggle album-art view (cover + track info instead of the bar)"},
	{"mute", "mute", "Toggle mute"},
	{"about", "about", "Show information about utuimp"},
	{"debug-logs", "debug-logs", "Toggle debug log panel"},
	{"q", "q", "Quit utuimp"},
	{"quit", "quit", "Quit utuimp"},
}

// commandSuggestions returns commands whose trigger starts with the current
// cmdBuf, or all commands when the buffer is empty.
func (m *Model) commandSuggestions() []cmdEntry {
	var out []cmdEntry
	for _, c := range allCommands {
		if m.cmdBuf == "" || strings.HasPrefix(c.trigger, m.cmdBuf) ||
			strings.HasPrefix(c.usage, m.cmdBuf) {
			out = append(out, c)
		}
	}
	return out
}

func (m *Model) handleCommandKey(k string) tea.Cmd {
	return m.handleCommandKeyMsg(k, tea.KeyPressMsg{})
}

func (m *Model) handleCommandKeyMsg(k string, msg tea.KeyPressMsg) tea.Cmd {
	switch k {
	case "esc":
		m.mode = modeNormal
		m.cmdBuf = ""
		m.cmdSuggIdx = 0
	case "enter":
		cmd := m.cmdBuf
		m.cmdBuf = ""
		m.cmdSuggIdx = 0
		m.mode = modeNormal
		return m.executeCommand(cmd)
	case "tab":
		suggs := m.commandSuggestions()
		if len(suggs) > 0 {
			if m.cmdSuggIdx >= len(suggs) {
				m.cmdSuggIdx = len(suggs) - 1
			}
			m.cmdBuf = suggs[m.cmdSuggIdx].usage
			if idx := strings.Index(m.cmdBuf, " <"); idx >= 0 {
				m.cmdBuf = m.cmdBuf[:idx+1]
			}
		}
	case "up", "ctrl+p":
		suggs := m.commandSuggestions()
		if len(suggs) > 0 {
			m.cmdSuggIdx = max(0, min(len(suggs)-1, m.cmdSuggIdx)-1)
		}
	case "down", "ctrl+n":
		suggs := m.commandSuggestions()
		if len(suggs) > 0 {
			m.cmdSuggIdx = min(len(suggs)-1, m.cmdSuggIdx+1)
		}
	case "ctrl+u":
		m.cmdBuf = ""
		m.cmdSuggIdx = 0
	case "ctrl+w":
		runes := []rune(m.cmdBuf)
		i := len(runes)
		for i > 0 && runes[i-1] == ' ' {
			i--
		}
		for i > 0 && runes[i-1] != ' ' {
			i--
		}
		m.cmdBuf = string(runes[:i])
		m.cmdSuggIdx = 0
	case "backspace":
		if len(m.cmdBuf) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.cmdBuf)
			m.cmdBuf = m.cmdBuf[:len(m.cmdBuf)-size]
			m.cmdSuggIdx = 0
		}
	case "space":
		m.insertCommandText(" ", false)
	default:
		if text := extractPrintableKeyText(k, msg); text != "" {
			m.insertCommandText(text, false)
		}
	}
	return nil
}

func (m *Model) executeCommand(cmd string) tea.Cmd {
	cmd = strings.TrimSpace(cmd)
	if name, rest, _ := strings.Cut(cmd, " "); true {
		cmd = strings.TrimSpace(strings.ToLower(name) + " " + rest)
	}
	switch {
	case cmd == "q" || cmd == "quit":
		if m.player != nil {
			_ = m.player.Close()
		}
		return tea.Quit
	case cmd == "about":
		for i, p := range m.panels {
			if p == m.aboutP {
				m.activePanel = i
				break
			}
		}
		return nil
	case cmd == "debug-logs":
		m.debugView = !m.debugView
		m.debugScroll = 0
		return nil
	case cmd == "art":
		if !m.artMode && (m.supportsArtColor == nil || !m.supportsArtColor()) {
			m.errMsg = i18n.T("album art needs a terminal with at least 256 colours")
			m.errExpiry = time.Now().Add(3 * time.Second)
			return nil
		}
		m.artMode = !m.artMode
		m.cfg.AlbumArt = m.artMode
		if err := m.cfg.Save(""); err != nil {
			m.appendLog(fmt.Sprintf("[art] config save error: %v", err))
		}
		if m.artMode {
			m.appendLog("[art] album-art view on")
			// Covers aren't downloaded while the art view is off, so fetch
			// the current track's cover now if we don't have it yet.
			return m.fetchArtworkCmd(m.artwork.url, m.artworkGen)
		}
		m.appendLog("[art] album-art view off")
		return nil

	case cmd == "mute":
		if m.preMuteVol >= 0 {
			// Currently muted — restore previous volume.
			vol := m.preMuteVol
			m.preMuteVol = -1
			m.appendLog(fmt.Sprintf("[vol] unmuted → %.0f%%", vol*100))
			return m.volumeCmd(vol, saveVolumeMsg{vol: vol})
		}
		// Mute: save current volume and set to 0.
		m.preMuteVol = m.playerState.Volume
		m.appendLog("[vol] muted")
		return m.volumeCmd(0, saveVolumeMsg{muted: true, preMute: m.preMuteVol})

	case strings.HasPrefix(cmd, "quality"):
		name, arg, _ := strings.Cut(cmd, " ")
		arg = strings.TrimSpace(arg)
		if arg == "" {
			m.errMsg = ":" + name + " requires high, standard, 256, or 64"
			m.errExpiry = time.Now().Add(3 * time.Second)
			return nil
		}
		kbps, err := audioquality.Parse(arg)
		if err != nil {
			m.errMsg = err.Error()
			m.errExpiry = time.Now().Add(4 * time.Second)
			return nil
		}
		m.appendLog(fmt.Sprintf("[quality] → %d kbps AAC", kbps))
		p := m.player
		return func() tea.Msg {
			if p == nil {
				return errMsg{errNotConnected}
			}
			if err := p.SetAudioBitrate(kbps); err != nil {
				if errors.Is(err, player.ErrAudioBitrateSavedPreferenceOnly) {
					return saveAudioQualityMsg{kbps: kbps, savedOnly: true}
				}
				return errMsg{err}
			}
			return saveAudioQualityMsg{kbps: kbps}
		}

	case strings.HasPrefix(cmd, "vol"):
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "vol"))
		if arg == "" {
			m.errMsg = fmt.Sprintf(i18n.T("volume: %d%%"), int(m.playerState.Volume*100))
			m.errExpiry = time.Now().Add(3 * time.Second)
			return nil
		}
		var newVol float64
		switch {
		case strings.HasPrefix(arg, "+"):
			delta, err := strconv.Atoi(arg[1:])
			if err != nil || delta < 0 {
				m.errMsg = i18n.T(":vol +n requires a positive number")
				m.errExpiry = time.Now().Add(3 * time.Second)
				return nil
			}
			newVol = clamp(m.playerState.Volume+float64(delta)/100, 0, 1)
		case strings.HasPrefix(arg, "-"):
			delta, err := strconv.Atoi(arg[1:])
			if err != nil || delta < 0 {
				m.errMsg = i18n.T(":vol -n requires a positive number")
				m.errExpiry = time.Now().Add(3 * time.Second)
				return nil
			}
			newVol = clamp(m.playerState.Volume-float64(delta)/100, 0, 1)
		default:
			n, err := strconv.Atoi(arg)
			if err != nil || n < 0 || n > 100 {
				m.errMsg = i18n.T(":vol requires 0-100, +n, or -n")
				m.errExpiry = time.Now().Add(3 * time.Second)
				return nil
			}
			newVol = float64(n) / 100
		}
		m.preMuteVol = -1 // clear mute state on explicit vol change
		m.appendLog(fmt.Sprintf("[vol] → %.0f%%", newVol*100))
		vol := newVol
		p := m.player
		return func() tea.Msg {
			if p == nil {
				return errMsg{errNotConnected}
			}
			if err := p.SetVolume(vol); err != nil {
				return errMsg{err}
			}
			return saveVolumeMsg{vol: vol}
		}

	case strings.HasPrefix(cmd, "seek"):
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "seek"))
		if arg == "" {
			if m.playerState.Track != nil {
				m.errMsg = fmt.Sprintf(i18n.T("position: %s / %s"),
					views.FormatDuration(m.playerState.Position),
					views.FormatDuration(m.playerState.Track.Duration))
				m.errExpiry = time.Now().Add(3 * time.Second)
			}
			return nil
		}
		n, err := strconv.Atoi(arg)
		if err != nil || n < 0 {
			m.errMsg = i18n.T(":seek requires a non-negative number of seconds")
			m.errExpiry = time.Now().Add(3 * time.Second)
			return nil
		}
		pos := time.Duration(n) * time.Second
		m.appendLog(fmt.Sprintf("[player] seek → %s", views.FormatDuration(pos)))
		m.noteSeek(pos)
		return m.playerCmd(func(p player.Player) error { return p.Seek(pos) })

	case strings.HasPrefix(cmd, "save "), strings.HasPrefix(cmd, "save-playlist "):
		name := cmd
		for _, prefix := range []string{"save-playlist ", "save "} {
			name = strings.TrimPrefix(name, prefix)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			m.errMsg = i18n.T(":save requires a playlist name")
			m.errExpiry = time.Now().Add(3 * time.Second)
			return nil
		}
		ids := make([]string, len(m.queueTracks))
		for i, t := range m.queueTracks {
			ids[i] = views.PlaybackID(t)
		}
		return m.createPlaylistCmd(name, ids)
	}
	m.errMsg = fmt.Sprintf(i18n.T("unknown command: %s"), cmd)
	m.errExpiry = time.Now().Add(3 * time.Second)
	return nil
}

func (m *Model) createPlaylistCmd(name string, ids []string) tea.Cmd {
	m.appendLog(fmt.Sprintf("[playlist] creating %q with %d tracks", name, len(ids)))
	return func() tea.Msg {
		_, err := m.provider.CreatePlaylist(context.Background(), name, ids)
		if err != nil {
			return errMsg{fmt.Errorf("save-playlist: %w", err)}
		}
		return playlistCreatedMsg{name: name}
	}
}

func (m *Model) fetchArtworkCmd(url string, gen int) tea.Cmd {
	if url == "" || m.supportsArtColor == nil || !m.supportsArtColor() {
		return nil
	}
	if m.artwork.url == url && gen == m.artworkGen && (m.artwork.img != nil || m.artwork.failed) {
		return nil
	}
	client := m.artHTTP
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		img, err := art.FetchAndDecode(ctx, client, url, 5<<20)
		return artworkLoadedMsg{url: url, gen: gen, img: img, err: err}
	}
}

// fetchLyricsCmd fetches lyrics for t from LRCLIB asynchronously.
func (m *Model) fetchLyricsCmd(t *provider.Track) tea.Cmd {
	trackID := views.PlaybackID(*t)
	q := lyrics.Query{TrackID: t.ID, Artist: t.Artist, Title: t.Title, Album: t.Album, Duration: t.Duration}
	src := lyrics.SourceAuto
	if m.cfg != nil {
		src = lyrics.ParseSource(m.cfg.LyricsSources[trackID])
	}
	m.lyricsP.m.SetRequestedSource(src)
	client := m.lyricsClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res, err := client.FetchFrom(ctx, src, q)
		return lyricsResultMsg{trackID: trackID, result: res, err: err}
	}
}

// serviceLyrics is implemented by providers with their own lyrics.
type serviceLyrics interface {
	Name() string
	TrackLyrics(ctx context.Context, trackID string, synced bool) (string, error)
}

// useServiceLyrics enables the provider's own lyrics as a lyrics source.
func (m *Model) useServiceLyrics(prov provider.Provider) {
	if sl, ok := prov.(serviceLyrics); ok {
		m.lyricsClient.ServiceLyrics = sl.TrackLyrics
		lyrics.ServiceName = strings.TrimSuffix(sl.Name(), " Music")
	}
}

// translateLang is the language lyrics are translated into.
// favRemoveWindow is how soon the second f must follow the first to remove
// the playing track from favourites.
const favRemoveWindow = 1500 * time.Millisecond

func (m *Model) translateLang() string {
	if m.cfg != nil && m.cfg.TranslateLang != "" {
		return m.cfg.TranslateLang
	}
	return lyrics.DefaultTranslateLang()
}

// translateLyricsCmd fetches a translation of the loaded lyrics.
func (m *Model) translateLyricsCmd() tea.Cmd {
	lines := append([]lyrics.Line(nil), m.lyricsP.m.Lines()...)
	if len(lines) == 0 {
		return nil
	}
	lang := m.translateLang()
	m.lyricsP.m.SetTranslating(lang)
	trackID := m.lastLyricsTrackID
	client := m.lyricsClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		tr, err := client.Translate(ctx, lines, lang)
		return lyricsTranslatedMsg{trackID: trackID, lines: tr.Lines, source: tr.Source, err: err}
	}
}

// lyricsTickInterval drives karaoke redraws (25 fps) while the lyrics panel
// is open and playing; the 100 ms glow tick alone looks steppy.
const lyricsTickInterval = 40 * time.Millisecond

type lyricsTickMsg struct{}

func lyricsTick() tea.Cmd {
	return tea.Tick(lyricsTickInterval, func(time.Time) tea.Msg { return lyricsTickMsg{} })
}

// titleReveal is how long a new track's title takes to write itself in.
const titleReveal = 450 * time.Millisecond

// fastAnimating reports whether something needs the 25 fps tick: karaoke
// lyrics while playing, or a title reveal in progress.
func (m *Model) fastAnimating() bool {
	if m.lyricsVisible() && m.playerState.Playing {
		return true
	}
	return !m.trackChangedAt.IsZero() && time.Since(m.trackChangedAt) < titleReveal
}

// revealLine shows a centred line written in from the left, for the first
// titleReveal after a track change.
func (m *Model) revealLine(line string) string {
	if m.noAnim || m.trackChangedAt.IsZero() {
		return line
	}
	t := float64(time.Since(m.trackChangedAt)) / float64(titleReveal)
	if t >= 1 {
		return line
	}
	t = 1 - (1-t)*(1-t) // ease out
	plain := ansi.Strip(line)
	lead := len(plain) - len(strings.TrimLeft(plain, " "))
	w := lipgloss.Width(line)
	return ansi.Truncate(line, lead+int(t*float64(w-lead)), "")
}

// lyricsVisible reports whether the lyrics panel is open with a track.
func (m *Model) lyricsVisible() bool {
	return m.activePanel >= 0 && m.panels[m.activePanel] == m.lyricsP && m.playerState.Track != nil
}

// advanceLyricsClock moves the lyrics clock forward by the wall time since
// the last call and corrects a fraction of its drift from the player's
// position. Small backward corrections are absorbed rather than shown, so the
// highlight never steps back; a large difference (seek, track change, pause)
// is applied at once.
func (m *Model) advanceLyricsClock() {
	now := time.Now()
	target := m.lyricsPosition()
	const snapAfter = 700 * time.Millisecond
	if m.lyricsClockAt.IsZero() || !m.playerState.Playing || m.playerState.Loading {
		m.lyricsClock = target
	} else {
		predicted := m.lyricsClock + now.Sub(m.lyricsClockAt)
		drift := target - predicted
		switch {
		case drift > snapAfter || drift < -snapAfter:
			m.lyricsClock = target
		default:
			next := predicted + drift/6
			if next < m.lyricsClock {
				next = m.lyricsClock // never run backwards
			}
			m.lyricsClock = next
		}
	}
	m.lyricsClockAt = now
	m.lyricsP.m.SetPosition(m.lyricsClock)
}

// setBusy shows label with a spinner until the load finishes.
func (m *Model) setBusy(label string) { m.busyLabel = label }

// seekGrace is how long after a seek player reports far from the seek
// target are treated as stale (sent before the seek took effect).
const seekGrace = 1500 * time.Millisecond

// noteSeek moves the displayed position to pos at once and arms seekGrace,
// so a player update still carrying the old position can't drag the lyrics
// and progress line back.
func (m *Model) noteSeek(pos time.Duration) {
	m.playerState.Position = pos
	m.stateAt = time.Now()
	m.seekTarget, m.seekAt = pos, time.Now()
	m.lyricsClockAt = time.Time{} // jump, don't glide
	m.advanceLyricsClock()
}

// smoothPosition is the playback position for animated elements (progress
// line): the smoothed lyrics clock when it is running, else the player's.
func (m *Model) smoothPosition() time.Duration {
	if m.lyricsClockAt.IsZero() {
		return m.playerState.Position
	}
	return m.lyricsClock
}

// lyricsPosition is the playback position extrapolated from the last player
// update, so word highlighting advances smoothly between updates.
func (m *Model) lyricsPosition() time.Duration {
	pos := m.playerState.Position
	if m.playerState.Playing && !m.playerState.Loading && !m.stateAt.IsZero() {
		pos += min(time.Since(m.stateAt), 2*time.Second)
	}
	if !m.seekAt.IsZero() {
		since := time.Since(m.seekAt)
		expected := m.seekTarget
		if m.playerState.Playing {
			expected += since
		}
		switch d := pos - expected; {
		case since > seekGrace:
			m.seekAt = time.Time{}
		case d > time.Second || d < -time.Second:
			pos = expected // stale pre-seek report
		default:
			m.seekAt = time.Time{} // the player has caught up
		}
	}
	if t := m.playerState.Track; t != nil && t.Duration > 0 && pos > t.Duration {
		pos = t.Duration
	}
	return pos
}

// fetchFeedCmd fetches personalised recommendations from the provider asynchronously.
func (m *Model) fetchFeedCmd() tea.Cmd {
	prov := m.provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		groups, err := prov.GetRecommendations(ctx)
		return feedResultMsg{groups: groups, err: err}
	}
}

// openArtistCmd shows the artist in the search view and fetches their albums.
func (m *Model) openArtistCmd(artist provider.Artist) tea.Cmd {
	m.search.OpenArtist(artist)
	m.appendLog(fmt.Sprintf("[search] loading artist %q (id=%s)…", artist.Name, artist.ID))
	prov := m.provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		albums, err := prov.GetArtistAlbums(ctx, artist.ID)
		return artistAlbumsMsg{artist: artist, albums: albums, err: err}
	}
}

// fetchSearchCollectionCmd loads all tracks for a search result album or playlist,
// then either replaces the queue (play=true) or appends to it (play=false).
//
// Album routing: search results only contain catalog albums (library albums are
// excluded from Search to avoid returning a subset of the full release), so
// GetAlbumTracks is always used directly with the album's catalog ID.
//
// Playlist routing: IDs starting with "p." are library playlists; everything else
// uses the catalog endpoint.
func (m *Model) fetchSearchCollectionCmd(album *provider.Album, playlist *provider.Playlist, play bool, playNext bool) tea.Cmd {
	prov := m.provider
	if album != nil {
		snap := *album
		m.appendLog(fmt.Sprintf("[search] loading album %q (id=%s)…", snap.Title, snap.ID))
		m.setBusy("loading album “" + snap.Title + "”…")
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			tracks, err := prov.GetAlbumTracks(ctx, snap.ID)
			return searchCollectionTracksMsg{label: snap.Title, tracks: tracks, play: play, playNext: playNext, err: err}
		}
	}
	if playlist != nil {
		snap := *playlist
		isLibrary := strings.HasPrefix(snap.ID, "p.")
		m.appendLog(fmt.Sprintf("[search] loading playlist %q…", snap.Name))
		m.setBusy("loading playlist “" + snap.Name + "”…")
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var tracks []provider.Track
			var err error
			if isLibrary {
				tracks, err = prov.GetPlaylistTracks(ctx, snap.ID)
			} else {
				tracks, err = prov.GetCatalogPlaylistTracks(ctx, snap.ID)
			}
			return searchCollectionTracksMsg{label: snap.Name, tracks: tracks, play: play, playNext: playNext, err: err}
		}
	}
	return nil
}

// fetchFeedItemTracksCmd loads the tracks for a recommendation item then either
// plays them (play=true) or appends them to the queue.
func (m *Model) fetchFeedItemTracksCmd(item *provider.RecommendationItem, play bool, playNext bool) tea.Cmd {
	snap := *item
	prov := m.provider
	m.appendLog(fmt.Sprintf("[feed] loading %q (%s)…", snap.Title, snap.Kind))
	m.setBusy("loading " + snap.Kind + " “" + snap.Title + "”…")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var tracks []provider.Track
		var err error
		switch snap.Kind {
		case "album":
			tracks, err = prov.GetAlbumTracks(ctx, snap.ID)
		case "playlist":
			tracks, err = prov.GetCatalogPlaylistTracks(ctx, snap.ID)
		default:
			err = fmt.Errorf("unknown kind %q", snap.Kind)
		}
		return feedTracksMsg{item: snap, tracks: tracks, play: play, playNext: playNext, err: err}
	}
}

// startRadioFrom activates radio mode seeded by the given track, replacing
// any radio session already in progress.
func (m *Model) startRadioFrom(seed *provider.Track) tea.Cmd {
	if seed == nil {
		return nil
	}
	if r, ok := m.provider.(interface{ ResetStation(string) }); ok {
		// Yandex: start a fresh "My Wave" from this song.
		r.ResetStation(views.PlaybackID(*seed))
	}
	m.radio.generation++
	m.radio.enabled = true
	m.radio.seed = seed
	m.radio.skipped = nil
	m.radio.refilling = true // guard against double-trigger while search is in flight
	m.radio.triggeredForID = ""
	m.radio.retries = 0
	m.appendLog(fmt.Sprintf("[radio] started from %q", seed.Title))
	return tea.Batch(m.dropQueueAfter(seed), m.runRadioSearch())
}

// toggleRadio starts radio (on Yandex: "My Wave" by this song) from seed, or
// stops it when it is already running from that same song. Running "My
// Wave" from the feed, or radio from another song, switches to this one.
func (m *Model) toggleRadio(seed *provider.Track) tea.Cmd {
	if m.radio.enabled && m.radio.seed != nil && seed != nil &&
		views.PlaybackID(*m.radio.seed) == views.PlaybackID(*seed) {
		m.stopRadio()
		return nil
	}
	if seed == nil {
		if m.radio.enabled {
			m.stopRadio()
		}
		return nil
	}
	return m.startRadioFrom(seed)
}

// dropQueueAfter removes any tracks queued after seed's position so a
// freshly started radio session plays next, rather than waiting for
// whatever was already lined up (e.g. the rest of an album) to finish —
// the last-track refill trigger otherwise never fires until that happens.
// Tracks up to and including seed are left alone. If seed isn't in the
// queue at all (e.g. seeded from a search result), it's inserted as the
// next track to play instead.
func (m *Model) dropQueueAfter(seed *provider.Track) tea.Cmd {
	seedID := views.PlaybackID(*seed)
	seedIdx := -1
	for i, t := range m.queueTracks {
		if views.PlaybackID(t) == seedID {
			seedIdx = i
			break
		}
	}
	if seedIdx < 0 {
		return m.playNextCmd(views.TrackLabel(*seed), []provider.Track{*seed}, []string{seedID})
	}
	origLen := len(m.queueTracks)
	if seedIdx == origLen-1 {
		return nil
	}
	// Blacklist the dropped tracks so the imminent radio refill can't hand
	// them right back — station results are often the very same tracks
	// (same album/playlist context as the seed), and the refill's dedup
	// only checks what's still in the queue.
	if m.radio.skipped == nil {
		m.radio.skipped = make(map[string]bool)
	}
	for _, t := range m.queueTracks[seedIdx+1:] {
		m.radio.skipped[views.PlaybackID(t)] = true
		m.radio.skipped[strings.ToLower(t.Artist+"||"+t.Title)] = true
	}
	m.queueTracks = m.queueTracks[:seedIdx+1]
	m.queueIDs = m.queueIDs[:seedIdx+1]
	m.queue.SetTracks(m.queueTracks)
	m.appendLog(fmt.Sprintf("[radio] dropped %d queued track(s) ahead of the seed", origLen-seedIdx-1))
	return m.playerCmd(func(p player.Player) error {
		for i := origLen - 1; i > seedIdx; i-- {
			if err := p.RemoveFromQueue(i); err != nil {
				return err
			}
		}
		return nil
	})
}

// stopRadio disables radio mode and clears its state.
func (m *Model) stopRadio() {
	if !m.radio.enabled {
		return
	}
	m.radio.enabled = false
	m.radio.seed = nil
	m.radio.skipped = nil
	m.radio.refilling = false
	m.radio.triggeredForID = ""
	m.radio.generation++
	m.appendLog("[radio] stopped")
}

// runRadioSearch fetches the next batch of tracks for the active radio
// station and returns a radioResultMsg refill, mirroring
// runDiscoverySearch's dedup/exclude handling.
func (m *Model) runRadioSearch() tea.Cmd {
	if !m.radio.enabled || m.radio.seed == nil {
		return nil
	}
	seed := m.radio.seed
	catalogID := seed.CatalogID
	if catalogID == "" {
		catalogID = seed.ID
	}
	prov := m.provider
	radioGen := m.radio.generation

	exclude := make(map[string]bool, len(m.radio.skipped)+len(m.queueIDs))
	for k := range m.radio.skipped {
		exclude[k] = true
	}
	for _, id := range m.queueIDs {
		exclude[id] = true
	}
	for _, t := range m.queueTracks {
		exclude[strings.ToLower(t.Artist+"||"+t.Title)] = true
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res, err := prov.GetStationTracks(ctx, catalogID)
		if err != nil {
			return radioResultMsg{radioGen: radioGen, err: err}
		}
		var merged []provider.Track
		seen := map[string]bool{}
		for _, t := range res {
			id := views.PlaybackID(t)
			key := strings.ToLower(t.Artist + "||" + t.Title)
			if exclude[id] || exclude[key] || seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, t)
		}
		if len(merged) == 0 {
			return radioResultMsg{radioGen: radioGen}
		}
		return radioResultMsg{radioGen: radioGen, tracks: merged}
	}
}

func (m *Model) forwardToActivePanel(msg tea.KeyPressMsg) tea.Cmd {
	if m.activePanel < 0 {
		return nil
	}
	k := msg.String()
	if m.panels[m.activePanel] == m.feedP {
		switch k {
		case "r":
			m.feedP.m.SetLoading()
			return m.fetchFeedCmd()
		case "enter":
			if item := m.feedP.m.SelectedItem(); item != nil {
				if strings.HasPrefix(item.ID, "wave:") {
					// A station: play it as endless radio, refilled as it goes.
					if r, ok := m.provider.(interface{ ResetStation(string) }); ok {
						r.ResetStation(item.ID)
					}
					m.queueTracks, m.queueIDs = nil, nil
					m.queue.SetTracks(nil)
					m.activePanel = -1
					seed := &provider.Track{ID: item.ID, CatalogID: item.ID, Title: item.Title}
					m.radio.generation++
					m.radio.enabled = true
					m.radio.seed = seed
					m.radio.skipped = nil
					m.radio.refilling = true
					m.radio.triggeredForID = ""
					m.radio.retries = 0
					m.appendLog("[radio] " + item.Title)
					// Empty the player's queue too, so the first batch
					// starts playing instead of queueing behind old songs.
					return tea.Sequence(
						m.playerCmd(func(p player.Player) error { return p.ClearQueue() }),
						m.runRadioSearch(),
					)
				}
				m.playerState.Loading = true
				return m.fetchFeedItemTracksCmd(item, true, false)
			}
		case "tab":
			if item := m.feedP.m.SelectedItem(); item != nil {
				return m.fetchFeedItemTracksCmd(item, false, false)
			}
		case "shift+tab":
			if item := m.feedP.m.SelectedItem(); item != nil {
				return m.fetchFeedItemTracksCmd(item, false, true)
			}
		default:
			return m.feedP.m.Update(msg)
		}
		return nil
	}
	return m.panels[m.activePanel].Update(msg)
}

func (m *Model) handleNormalKey(msg tea.KeyPressMsg, k string) tea.Cmd {
	// When debug log is open, j/k/G scroll it; esc back/closes it.
	if m.debugView {
		switch k {
		case "j", "down":
			if m.debugScroll > 0 {
				m.debugScroll--
			}
			return nil
		case "k", "up":
			m.debugScroll++
			return nil
		case "G":
			m.debugScroll = 0
			return nil
		case "esc":
			m.debugView = false
			return nil
		}
	}

	if k == "i" {
		m.lastKey = ""
		return m.openSongMenu()
	}

	if k == "v" && !m.lyricsP.m.PickerOpen() {
		m.lastKey = ""
		return m.toggleCoverView()
	}

	// Cover view: esc on the queue goes back to the lyrics.
	if m.coverView && k == "esc" && m.activePanel < 0 && m.queueMiniCursor < 0 {
		m.lastKey = ""
		return m.openLyricsPanel()
	}

	if k == "L" {
		m.library.m.ResetTopLevel()
		for i, p := range m.panels {
			if p == m.library {
				m.activePanel = i
				m.lastKey = ""
				return nil
			}
		}
	}

	// Panel nav keys toggle their panel (always checked first).
	for i, p := range m.panels {
		if k == p.NavKey() {
			if m.activePanel == i {
				m.activePanel = -1 // toggle off
				if m.coverView && p != m.lyricsP {
					m.lastKey = ""
					return m.openLyricsPanel() // the cover view falls back to the lyrics
				}
			} else {
				m.activePanel = i
				// Trigger a feed load when opening the feed panel for the first time.
				if p == m.feedP && m.feedP.m.NeedsLoad() {
					m.feedP.m.SetLoading()
					m.lastKey = ""
					return m.fetchFeedCmd()
				}
				// Lazy-load lyrics when the panel is opened and the current
				// track hasn't been fetched yet.
				if p == m.lyricsP && m.lastLyricsTrackID == "" && m.playerState.Track != nil {
					id := views.PlaybackID(*m.playerState.Track)
					m.lastLyricsTrackID = id
					m.lyricsP.m.SetLoading()
					m.lastKey = ""
					return m.fetchLyricsCmd(m.playerState.Track)
				}
			}
			m.lastKey = ""
			return nil
		}
	}

	// ':' opens command mode from anywhere — checked before any panel handler.
	if k == ":" {
		m.mode = modeCommand
		m.cmdBuf = ""
		return nil
	}

	// '/' opens search from anywhere too (no panel uses it), so it also
	// works over the lyrics of the cover view; esc returns to that panel.
	if k == "/" && !m.lyricsP.m.PickerOpen() {
		m.lastKey = ""
		m.mode = modeSearch
		m.searchCursor = len([]rune(m.searchQuery))
		return nil
	}

	// History panel: play, queue or seed radio from a recently played song.
	if m.activePanel >= 0 && m.panels[m.activePanel] == m.historyP {
		_, t := m.historyP.m.SelectedTrack()
		switch k {
		case "esc":
			m.activePanel = -1
			m.lastKey = ""
			return nil
		case "enter":
			if t != nil {
				return m.playNowCmd(*t)
			}
			return nil
		case "tab":
			if t != nil {
				return m.playNextCmd(views.TrackLabel(*t), []provider.Track{*t}, []string{views.PlaybackID(*t)})
			}
			return nil
		case "p":
			if t != nil {
				return m.openPlaylistPicker(t)
			}
			return nil
		case "R":
			return m.toggleRadio(t)
		case "up", "down", "k", "j", "g", "G", "home", "end", "pgup", "pgdown":
			return m.historyP.Update(msg)
		}
	}

	// A song selected in the main-screen queue turns the queue keys on
	// (they shadow the global meaning of d/p/s/R until esc deselects).
	if m.activePanel < 0 && m.queueMiniCursor >= 0 {
		if cmd, ok := m.queueOp(k, m.queueMiniCursor); ok {
			return cmd
		}
	}

	if m.activePanel >= 0 && m.panels[m.activePanel] == m.eqP {
		switch k {
		case "esc":
			m.activePanel = -1
			m.lastKey = ""
			return nil
		case "left", "h", "right", "l", "up", "k", "down", "j", "0", "r":
			return m.eqP.Update(msg)
		}
	}

	// Player control keys always work, even when other panels are active.
	switch k {
	case "space":
		return m.togglePlayPause()

	case "n":
		m.lastKey = ""
		m.appendLog("[player] next")
		m.playerState.Loading = true
		return m.playerCmd(func(p player.Player) error { return p.Next() })

	case "p":
		m.lastKey = ""
		m.appendLog("[player] previous")
		m.playerState.Loading = true
		return m.playerCmd(func(p player.Player) error { return p.Previous() })

	case "+", "=":
		m.lastKey = ""
		return m.adjustVolume(0.05)

	case "-":
		m.lastKey = ""
		return m.adjustVolume(-0.05)

	case "r":
		m.lastKey = ""
		// Cycle: off → all → one → off
		next := player.RepeatModeOff
		switch m.playerState.RepeatMode {
		case player.RepeatModeOff:
			next = player.RepeatModeAll
		case player.RepeatModeAll:
			next = player.RepeatModeOne
		case player.RepeatModeOne:
			next = player.RepeatModeOff
		}
		m.playerState.RepeatMode = next
		m.appendLog(fmt.Sprintf("[player] repeat → %d", next))
		return m.playerCmd(func(p player.Player) error { return p.SetRepeat(next) })

	case "s":
		m.lastKey = ""
		on := !m.playerState.ShuffleMode
		m.playerState.ShuffleMode = on
		m.appendLog(fmt.Sprintf("[player] shuffle → %v", on))
		return m.playerCmd(func(p player.Player) error { return p.SetShuffle(on) })

	case "f":
		m.lastKey = ""
		if m.playerState.Track != nil {
			t := m.playerState.Track
			if t.CatalogID == "" {
				if idx := playingQueueIdx(m.queueTracks, t); idx >= 0 && idx < len(m.queueTracks) {
					t.CatalogID = m.queueTracks[idx].CatalogID
				}
			}
			loved := !m.isTrackFavorite(t)
			if !loved {
				// Removing a favourite is easy to do by accident, so it
				// takes a second f within favRemoveWindow.
				id := views.PlaybackID(*t)
				if m.favRemoveID != id || time.Since(m.favRemoveAt) > favRemoveWindow {
					m.favRemoveID, m.favRemoveAt = id, time.Now()
					m.errMsg = i18n.T("press f again to remove from favorites")
					m.errExpiry = time.Now().Add(favRemoveWindow)
					return nil
				}
				m.errMsg, m.favRemoveID = "", ""
			}
			m.setTrackFavorite(t, loved)
			m.appendLog(fmt.Sprintf("[fav] %s → %v", t.Title, loved))
			// Sync to Apple Music asynchronously.
			return m.loveSongCmd(t, loved)
		}
	case "R":
		m.lastKey = ""
		return m.toggleRadio(m.playerState.Track)

	case "left":
		m.lastKey = ""
		if m.activePanel >= 0 && !m.lyricsVisible() { // the lyrics don't use ←/→: seek
			return m.forwardToActivePanel(msg)
		}
		if m.playerState.Track != nil {
			newPos := max(0, m.playerState.Position-10*time.Second)
			m.appendLog(fmt.Sprintf("[player] seek ← %s", views.FormatDuration(newPos)))
			pos := newPos
			m.noteSeek(pos)
			return m.playerCmd(func(p player.Player) error { return p.Seek(pos) })
		}

	case "right":
		m.lastKey = ""
		if m.activePanel >= 0 && !m.lyricsVisible() { // the lyrics don't use ←/→: seek
			return m.forwardToActivePanel(msg)
		}
		if m.playerState.Track != nil {
			newPos := m.playerState.Position + 10*time.Second
			if dur := m.playerState.Track.Duration; dur > 0 && newPos > dur {
				newPos = dur
			}
			m.appendLog(fmt.Sprintf("[player] seek → %s", views.FormatDuration(newPos)))
			pos := newPos
			m.noteSeek(pos)
			return m.playerCmd(func(p player.Player) error { return p.Seek(pos) })
		}
	}

	// Forward remaining keys to other active panels (e.g. library).
	if m.activePanel >= 0 {
		if k == "esc" {
			if m.panels[m.activePanel].Back() {
				m.lastKey = ""
				return nil
			}
			closed := m.panels[m.activePanel]
			m.activePanel = -1
			m.lastKey = ""
			if m.coverView && closed != m.lyricsP {
				return m.openLyricsPanel() // the cover view falls back to the lyrics
			}
			return nil
		}
		return m.forwardToActivePanel(msg)
	}

	// Keys that only work when no panel is covering the content area.
	switch k {
	case "/":
		m.mode = modeSearch
		m.searchCursor = len([]rune(m.searchQuery))
	case "j", "down":
		m.lastKey = ""
		m.moveQueueMiniCursor(1)

	case "k", "up":
		m.lastKey = ""
		m.moveQueueMiniCursor(-1)

	case "esc":
		m.queueMiniCursor = -1

	case "g":
		if m.lastKey == "g" {
			m.lastKey = ""
		} else {
			m.lastKey = "g"
		}

	case "G":
		m.lastKey = ""

	case "enter":
		m.lastKey = ""
		if m.queueMiniCursor >= 0 {
			return m.playQueueFrom(m.queueMiniCursor)
		}
		if t := m.search.SelectedTrack(); t != nil {
			tc := *t
			m.queueTracks = []provider.Track{tc}
			m.queueIDs = []string{views.PlaybackID(tc)}
			m.playerState.Track = &tc
			m.playerState.Loading = true
			m.playerState.Playing = false
			m.playerState.Position = 0
			m.queue.SetTracks(m.queueTracks)
			return m.playerCmd(func(p player.Player) error { return p.SetQueue(m.queueIDs) })
		}

	case "a":
		m.lastKey = ""
		if t := m.search.SelectedTrack(); t != nil {
			tc := *t
			m.queueTracks = append(m.queueTracks, tc)
			m.queueIDs = append(m.queueIDs, views.PlaybackID(tc))
			m.queue.SetTracks(m.queueTracks)
			return m.playerCmd(func(p player.Player) error { return p.AppendQueue([]string{views.PlaybackID(tc)}) })
		}

	default:
		m.lastKey = ""
	}

	return nil
}

// queueOp applies a queue-editing key to the song at idx. ok is false for
// keys it doesn't handle.
func (m *Model) queueOp(k string, idx int) (tea.Cmd, bool) {
	if idx < 0 || idx >= len(m.queueTracks) || idx >= len(m.queueIDs) {
		return nil, false
	}
	t := m.queueTracks[idx]
	switch k {
	case "enter":
		return m.playQueueFrom(idx), true
	case "esc":
		m.queueMiniCursor = -1
		return nil, true
	case "d", "x", "delete":
		m.queueTracks = append(m.queueTracks[:idx], m.queueTracks[idx+1:]...)
		m.queueIDs = append(m.queueIDs[:idx], m.queueIDs[idx+1:]...)
		m.queue.SetTracks(m.queueTracks)
		if m.queueMiniCursor >= len(m.queueTracks) {
			m.queueMiniCursor = len(m.queueTracks) - 1
		}
		m.appendLog(fmt.Sprintf("[queue] removed #%d: %s", idx+1, views.TrackLabel(t)))
		i := idx
		return m.playerCmd(func(p player.Player) error { return p.RemoveFromQueue(i) }), true
	case "K", "shift+up", "ctrl+up":
		if idx == 0 {
			return nil, true
		}
		m.queueTracks[idx-1], m.queueTracks[idx] = m.queueTracks[idx], m.queueTracks[idx-1]
		m.queueIDs[idx-1], m.queueIDs[idx] = m.queueIDs[idx], m.queueIDs[idx-1]
		m.queue.SetTracks(m.queueTracks)
		m.moveQueueMiniCursor(-1)
		m.appendLog(fmt.Sprintf("[queue] moved #%d up", idx+1))
		from, to := idx, idx-1
		return m.playerCmd(func(p player.Player) error { return p.MoveInQueue(from, to) }), true
	case "J", "shift+down", "ctrl+down":
		if idx >= len(m.queueTracks)-1 {
			return nil, true
		}
		m.queueTracks[idx], m.queueTracks[idx+1] = m.queueTracks[idx+1], m.queueTracks[idx]
		m.queueIDs[idx], m.queueIDs[idx+1] = m.queueIDs[idx+1], m.queueIDs[idx]
		m.queue.SetTracks(m.queueTracks)
		m.moveQueueMiniCursor(1)
		m.appendLog(fmt.Sprintf("[queue] moved #%d down", idx+1))
		from, to := idx, idx+1
		return m.playerCmd(func(p player.Player) error { return p.MoveInQueue(from, to) }), true
	case "c":
		m.appendLog("[queue] cleared")
		m.queueTracks, m.queueIDs = nil, nil
		m.queue.SetTracks(nil)
		m.queueMiniCursor = -1
		return m.playerCmd(func(p player.Player) error { return p.ClearQueue() }), true
	case "s":
		m.mode = modeCommand
		m.cmdBuf = "save "
		return nil, true
	case "p":
		tc := t
		return m.openPlaylistPicker(&tc), true
	case "R":
		tc := t
		return m.toggleRadio(&tc), true
	}
	return nil, false
}

// playNowCmd plays t right away: it goes in after the current song and the
// player skips to it, so the rest of the queue is kept.
func (m *Model) playNowCmd(t provider.Track) tea.Cmd {
	label := views.TrackLabel(t)
	ids := []string{views.PlaybackID(t)}
	if len(m.queueTracks) == 0 || m.playerState.Track == nil {
		return m.playNextCmd(label, []provider.Track{t}, ids)
	}
	insert := m.playNextCmd(label, []provider.Track{t}, ids)
	m.playerState.Loading = true
	return tea.Sequence(insert, m.playerCmd(func(p player.Player) error { return p.Next() }))
}

// maxHistory bounds the play history.
const maxHistory = 200

// recordHistory puts t at the top of the play history (dropping an older
// entry for the same song) and saves it.
func (m *Model) recordHistory(t provider.Track) {
	id := views.PlaybackID(t)
	if id == "" && t.Title == "" {
		return
	}
	out := make([]provider.Track, 0, min(len(m.history)+1, maxHistory))
	out = append(out, t)
	for _, h := range m.history {
		if views.PlaybackID(h) == id && h.Title == t.Title {
			continue
		}
		if len(out) == maxHistory {
			break
		}
		out = append(out, h)
	}
	m.history = out
	m.historyP.m.SetTracks(m.history)
	saveHistory(m.historyPath, m.history)
}

func defaultHistoryPath(name string) string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		return ""
	}
	if name == "" {
		name = "history"
	}
	return filepath.Join(dir, "utuimp", name+".json")
}

func loadHistory(path string) []provider.Track {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // our own cache file
	if err != nil {
		return nil
	}
	var h []provider.Track
	if json.Unmarshal(data, &h) != nil {
		return nil
	}
	return h[:min(len(h), maxHistory)]
}

func saveHistory(path string, h []provider.Track) {
	if path == "" {
		return
	}
	data, err := json.Marshal(h)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

// playQueueFrom starts playback at queue position idx, dropping the tracks
// before it (the queue panel's Enter behaviour).
func (m *Model) playQueueFrom(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.queueTracks) || idx >= len(m.queueIDs) {
		return nil
	}
	t := m.queueTracks[idx]
	if _, ok := m.player.(player.QueueJumper); ok {
		// Keep the queue; just continue from the chosen song.
		m.queueMiniCursor = -1
		m.appendLog(fmt.Sprintf("[queue] jump to position %d: %s", idx+1, views.TrackLabel(t)))
		m.playerState.Loading = true
		m.playerState.Playing = false
		m.playerState.Position = 0
		return m.playerCmd(func(p player.Player) error { return p.(player.QueueJumper).PlayAt(idx) })
	}
	ids := m.queueIDs[idx:]
	m.queueTracks = m.queueTracks[idx:]
	m.queueIDs = ids
	m.queue.SetTracks(m.queueTracks)
	m.queueMiniCursor = -1
	m.queueMiniOffset = 0
	m.appendLog(fmt.Sprintf("[queue] playing from position %d: %s", idx+1, views.TrackLabel(t)))
	m.playerState.Loading = true
	m.playerState.Playing = false
	m.playerState.Position = 0
	return m.playerCmd(func(p player.Player) error { return p.SetQueue(ids) })
}

// moveQueueMiniCursor moves the main-screen queue selection by delta,
// starting from the playing track, and scrolls to keep it visible.
func (m *Model) moveQueueMiniCursor(delta int) {
	n := len(m.queueTracks)
	if n == 0 {
		return
	}
	if m.queueMiniCursor < 0 {
		m.queueMiniCursor = max(playingQueueIdx(m.queueTracks, m.playerState.Track), 0)
	} else {
		m.queueMiniCursor += delta
	}
	m.queueMiniCursor = max(0, min(m.queueMiniCursor, n-1))
	rows := max(1, m.panelHeight()-2)
	if m.queueMiniCursor < m.queueMiniOffset {
		m.queueMiniOffset = m.queueMiniCursor
	} else if m.queueMiniCursor >= m.queueMiniOffset+rows {
		m.queueMiniOffset = m.queueMiniCursor - rows + 1
	}
}

func (m *Model) scheduleSearch(query string) tea.Cmd {
	if query == "" {
		m.searchGen++
		m.search.SetResults(nil, false, nil)
		return nil
	}
	m.searchGen++
	gen := m.searchGen
	m.search.SetResults(nil, true, nil)
	return func() tea.Msg {
		time.Sleep(400 * time.Millisecond)
		return searchDebounceMsg{query: query, gen: gen}
	}
}

// loveSongCmd calls provider.LoveSong asynchronously and returns a loveSongMsg.
func (m *Model) isTrackFavorite(t *provider.Track) bool {
	if t == nil {
		return false
	}
	if t.ID != "" && m.favorites[t.ID] {
		return true
	}
	if t.CatalogID != "" && m.favorites[t.CatalogID] {
		return true
	}
	return false
}

func (m *Model) setTrackFavorite(t *provider.Track, loved bool) {
	if t == nil {
		return
	}
	if t.ID != "" {
		m.favorites[t.ID] = loved
	}
	if t.CatalogID != "" {
		m.favorites[t.CatalogID] = loved
	}
}

// loveSongCmd calls provider.LoveSong asynchronously and returns a loveSongMsg.
func (m *Model) loveSongCmd(t *provider.Track, loved bool) tea.Cmd {
	if m.provider == nil || t == nil {
		return nil
	}
	catalogID := t.CatalogID
	if catalogID == "" {
		catalogID = t.ID
	}
	trackID := t.ID
	title := t.Title
	prov := m.provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := prov.LoveSong(ctx, catalogID, loved)
		return loveSongMsg{trackID: trackID, catalogID: catalogID, title: title, loved: loved, err: err}
	}
}

// checkSongRatingCmd checks whether the track is already Loved on Apple Music
// and returns a songRatingMsg so the model can update m.favorites accordingly.
func (m *Model) checkSongRatingCmd(t *provider.Track) tea.Cmd {
	if m.provider == nil || t == nil {
		return nil
	}
	catalogID := t.CatalogID
	if catalogID == "" {
		catalogID = t.ID
	}
	trackID := t.ID
	prov := m.provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		loved, _ := prov.GetSongRating(ctx, catalogID)
		return songRatingMsg{trackID: trackID, catalogID: catalogID, loved: loved}
	}
}

// purgeSkippedFromQueue removes every queued track whose PlaybackID appears in
// the radio skip list. It also tells the JS player to remove those
// slots so both sides stay in sync. Iterates from the end so index removal
// doesn't shift the positions of entries not yet processed.
func (m *Model) purgeSkippedFromQueue() {
	if len(m.radio.skipped) == 0 {
		return
	}
	changed := false
	for i := len(m.queueIDs) - 1; i >= 0; i-- {
		if !m.radio.skipped[m.queueIDs[i]] {
			continue
		}
		m.appendLog(fmt.Sprintf("[skip] removing from queue: %s — %s",
			m.queueTracks[i].Artist, m.queueTracks[i].Title))
		if m.player != nil {
			idx := i // capture for closure
			_ = m.player.RemoveFromQueue(idx)
		}
		m.queueTracks = slices.Delete(m.queueTracks, i, i+1)
		m.queueIDs = slices.Delete(m.queueIDs, i, i+1)
		changed = true
	}
	if changed {
		m.queue.SetTracks(m.queueTracks)
	}
}

func (m *Model) togglePlayPause() tea.Cmd {
	return func() tea.Msg {
		if m.player == nil {
			return errMsg{errNotConnected}
		}
		var err error
		if m.playerState.Playing {
			err = m.player.Pause()
		} else {
			err = m.player.Play()
		}
		if err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m *Model) playerCmd(fn func(player.Player) error) tea.Cmd {
	p := m.player
	return func() tea.Msg {
		if p == nil {
			return errMsg{errNotConnected}
		}
		if err := fn(p); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m *Model) playNextCmd(label string, tracks []provider.Track, ids []string) tea.Cmd {
	insertIdx := 0
	if m.playerState.Track != nil {
		for i, t := range m.queueTracks {
			if views.PlaybackID(t) == views.PlaybackID(*m.playerState.Track) {
				insertIdx = i + 1
				break
			}
		}
	}

	origLen := len(m.queueTracks)
	if origLen == 0 {
		m.queueTracks = tracks
		m.queueIDs = ids
		m.playerState.Track = &tracks[0]
		m.playerState.Loading = true
		m.playerState.Playing = false
		m.playerState.Position = 0
		m.queue.SetTracks(m.queueTracks)
		m.appendLog(fmt.Sprintf("[queue] play now: %s", label))
		return m.playerCmd(func(p player.Player) error { return p.SetQueue(ids) })
	}

	if insertIdx > origLen {
		insertIdx = origLen
	}

	// Insert locally
	m.queueTracks = append(m.queueTracks[:insertIdx], append(tracks, m.queueTracks[insertIdx:]...)...)
	m.queueIDs = append(m.queueIDs[:insertIdx], append(ids, m.queueIDs[insertIdx:]...)...)
	m.queue.SetTracks(m.queueTracks)

	m.appendLog(fmt.Sprintf("[queue] play next: %s (%d track(s))", label, len(tracks)))

	return m.playerCmd(func(p player.Player) error {
		if nq, ok := p.(player.NextQueuer); ok {
			return nq.PlayNext(ids)
		}
		if err := p.AppendQueue(ids); err != nil {
			return err
		}
		for i := range len(ids) {
			from := origLen + i
			to := insertIdx + i
			if err := p.MoveInQueue(from, to); err != nil {
				return err
			}
		}
		return nil
	})
}

// volumeCmd sets the player's volume and then reports done (saveVolumeMsg,
// which App shares with the other services).
func (m *Model) volumeCmd(vol float64, done saveVolumeMsg) tea.Cmd {
	p := m.player
	return func() tea.Msg {
		if p == nil {
			return errMsg{errNotConnected}
		}
		if err := p.SetVolume(vol); err != nil {
			return errMsg{err}
		}
		return done
	}
}

func (m *Model) adjustVolume(delta float64) tea.Cmd {
	m.preMuteVol = -1 // changing the volume ends a mute
	return func() tea.Msg {
		if m.player == nil {
			return errMsg{errNotConnected}
		}
		newVol := clamp(m.playerState.Volume+delta, 0, 1)
		if err := m.player.SetVolume(newVol); err != nil {
			return errMsg{err}
		}
		return saveVolumeMsg{vol: newVol}
	}
}

// ── View ──────────────────────────────────────────────────────────────────

func (m *Model) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}
	var content string
	switch {
	case m.mode == modePlaylistPicker:
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderPlaylistPickerModal())
	case m.introStep != introDone:
		content = m.renderIntro()
	default:
		if m.coverView {
			content = m.renderCoverLayout()
		} else {
			content = m.renderBoxLayout()
		}
		if m.songMenu != nil {
			content = m.overlay(content, m.renderSongMenu())
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = m.windowTitle()
	// Receive wheel events ourselves: otherwise the terminal turns one
	// notch into several arrow presses (see the MouseWheelMsg case).
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// errNotConnected is reported for player actions before the service is up.
var errNotConnected = errors.New("not connected yet")

// wheelGap is the minimum time between two wheel steps in one direction.
const wheelGap = 35 * time.Millisecond

// windowTitle names the terminal window: "utuimp", plus the song while one
// is loaded ("utuimp · Title — Artist").
func (m *Model) windowTitle() string {
	if t := m.playerState.Track; t != nil && (t.Title != "" || t.Artist != "") {
		prefix := "utuimp · "
		if !m.playerState.Playing && !m.playerState.Loading {
			prefix = "utuimp ⏸ "
		}
		return prefix + views.TrackLabel(*t)
	}
	return "utuimp"
}

// renderBoxLayout renders the full UI as rounded boxes, styled after the
// terminal's own palette:
//
//	╭─ Now Playing ───────────────────────╮
//	│  …track, progress line, controls…   │  (nowPlayingHeight rows)
//	╰──────────────────────────── ♪ 72% ─╯
//	╭─────────────────────────────────────╮
//	│ Queue                               │  (panelH rows)
//	╰─────────────────────────────────────╯
//	  spc play/pause · n/p next/prev …       (status rows, unboxed)
//
// The focused box's border uses the accent colour.
func (m *Model) renderBoxLayout() string {
	m.lyricsP.m.SetBare(false)
	w := m.width
	inner := w - 2 // visual width between the │ border chars
	npH := m.nowPlayingHeight()
	panelH := m.panelHeight()

	// Any open panel (library, history, lyrics, …) takes the full width.
	fullWidth := m.activePanel >= 0 || m.mode == modeSearch || m.mode == modeCommand || m.debugView

	var sb strings.Builder

	// ── Now Playing box ──
	npFocused := !fullWidth
	sb.WriteString(boxTop(w, i18n.T("Now Playing"), npFocused) + "\n")
	for _, line := range m.nowPlayingLines(inner-2, npH) {
		sb.WriteString(boxRow(line, w, npFocused) + "\n")
	}
	sb.WriteString(boxBottom(w, m.volumeLabel(), npFocused) + "\n")

	// ── Panel box(es) ──
	if fullWidth {
		var lines []string
		switch {
		case m.debugView:
			lines = m.debugLogLines(inner-2, panelH)
		case m.mode == modeSearch:
			lines = m.searchLines(inner-2, panelH)
		case m.mode == modeCommand:
			lines = m.commandLines(inner-2, panelH)
		default:
			p := m.panels[m.activePanel]
			p.SetSize(inner-2, panelH)
			lines = toLines(p.View(), panelH)
		}
		sb.WriteString(boxTop(w, i18n.T(""), true) + "\n")
		for i := range panelH {
			sb.WriteString(boxRow(safeIdx(lines, i), w, true) + "\n")
		}
		sb.WriteString(boxBottom(w, "", true) + "\n")
	} else {
		qLines := m.queuePanelLines(inner-2, panelH)
		sb.WriteString(boxTop(w, i18n.T(""), false) + "\n")
		for i := range panelH {
			sb.WriteString(boxRow(safeIdx(qLines, i), w, false) + "\n")
		}
		sb.WriteString(boxBottom(w, "", false) + "\n")
	}

	// ── Status rows (context/mode then playback), unboxed ──
	status := m.statusLines(inner - 2)
	for i, line := range status {
		sb.WriteString("  " + padRight(line, w-2))
		if i < len(status)-1 {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// volumeLabel is the volume indicator shown in the Now Playing box's bottom
// border (plus memory stats when profiling).
func (m *Model) volumeLabel() string {
	var vol string
	if m.preMuteVol >= 0 {
		vol = styles.Playing.Render(i18n.T("🔇 muted"))
	} else {
		vol = styles.QueueItemMuted.Render(fmt.Sprintf(i18n.T("♪ %d%%"), int(m.playerState.Volume*100)))
	}
	if m.memStats != "" {
		vol = styles.QueueItemMuted.Render(m.memStats) + "  " + vol
	}
	return vol
}

func borderStyle(focused bool) lipgloss.Style {
	if focused {
		return lipgloss.NewStyle().Foreground(styles.ColorBorderFocus)
	}
	return lipgloss.NewStyle().Foreground(styles.ColorBorder)
}

// boxTop renders a rounded top border w cells wide, with an optional title
// set into it: "╭─ Title ─────╮".
func boxTop(w int, title string, focused bool) string {
	if w < 2 {
		return ""
	}
	bs := borderStyle(focused)
	if title == "" || w < lipgloss.Width(title)+6 {
		return bs.Render("╭" + strings.Repeat("─", w-2) + "╮")
	}
	ts := lipgloss.NewStyle().Bold(true).Foreground(styles.ColorFg)
	if focused {
		ts = ts.Foreground(styles.ColorBorderFocus)
	}
	fill := w - 2 - 3 - lipgloss.Width(title)
	return bs.Render("╭─ ") + ts.Render(title) + bs.Render(" "+strings.Repeat("─", fill)+"╮")
}

// boxBottom renders a rounded bottom border w cells wide, with an optional
// (pre-styled) label set into its right end: "╰───── label ─╯".
func boxBottom(w int, label string, focused bool) string {
	if w < 2 {
		return ""
	}
	bs := borderStyle(focused)
	lw := lipgloss.Width(label)
	if label == "" || w < lw+6 {
		return bs.Render("╰" + strings.Repeat("─", w-2) + "╯")
	}
	fill := w - 2 - 3 - lw
	return bs.Render("╰"+strings.Repeat("─", fill)+" ") + label + bs.Render(" ─╯")
}

// boxRow renders one content row of a box w cells wide: "│ content │".
func boxRow(content string, w int, focused bool) string {
	if w < 4 {
		return ""
	}
	bs := borderStyle(focused)
	return bs.Render("│") + " " + padRight(content, w-4) + " " + bs.Render("│")
}

func (m *Model) renderIntro() string {
	if m.introStep < 0 || m.introStep >= len(introFrames) {
		return ""
	}

	frame := introFrames[m.introStep]
	glowIdx := m.glowStep % len(styles.GlowPalette)

	var logo strings.Builder
	for _, r := range frame {
		logo.WriteString(lipgloss.NewStyle().
			Foreground(styles.GlowPalette[glowIdx]).
			Render(string(r)))
	}

	var subtitle string
	if m.introStep >= len("♪ utuimp") {
		statusText := m.initStatus
		if statusText == "" {
			statusText = "connecting…"
		}
		subtitle = "\n" + centerStr(
			lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(statusText),
			m.width,
		)
	}

	topPad := max(0, (m.height-3)/2)
	return strings.Repeat("\n", topPad) +
		centerStr(logo.String(), m.width) +
		subtitle
}

// artModeActive reports whether the now-playing panel is currently showing
// (or loading) the album-art view rather than the progress bar. Art mode
// falls back to the bar layout when the current track has no artwork, the
// fetch failed, or the terminal can't show enough colours.
func (m *Model) artModeActive() bool {
	if !m.artMode || m.supportsArtColor == nil || !m.supportsArtColor() {
		return false
	}
	t := m.playerState.Track
	// m.artwork.url must match the current track so a cover left over from
	// the previous track is never shown while the next one is in flight.
	return t != nil && t.ArtworkURL != "" && m.artwork.url == t.ArtworkURL && !m.artwork.failed
}

func (m *Model) nowPlayingHeight() int {
	if !m.artModeActive() {
		return 9
	}
	// The art view trades panel rows for a bigger cover: grow with the
	// terminal, but always leave the split panels a usable height.
	return min(24, max(12, m.height-14))
}

// nowPlayingLines returns exactly h lines for the Now Playing section.
func (m *Model) nowPlayingLines(contentW, h int) []string {
	if m.artModeActive() && h >= 8 {
		return m.nowPlayingArtLines(contentW, h)
	}
	return m.nowPlayingTextLines(contentW, h)
}

// nowPlayingArtLines renders the album-art view: the cover centred, with the
// track line and elapsed time beneath it — no progress bar or controls. The
// bottom four rows hold a separator, "Artist — Title", "Album • elapsed /
// total", and the status line; the rows above are the cover, vertically
// centred, sized square via the measured cell aspect ratio. While the cover
// is still downloading its rows stay blank and it pops in when loaded.
func (m *Model) nowPlayingArtLines(contentW, h int) []string {
	t := m.playerState.Track
	if t == nil {
		return m.nowPlayingTextLines(contentW, h)
	}

	aspect := m.artCellAsp
	if aspect <= 0 {
		aspect = 2.0
	}
	artColsFor := func(rows int) int { return int(math.Round(float64(rows) * aspect)) }
	artRegion := h - 4
	artRows := artRegion
	artCols := artColsFor(artRows)
	for artRows > 2 && artCols > contentW {
		artRows--
		artCols = artColsFor(artRows)
	}
	if artRows < 2 || artCols < 4 {
		return m.nowPlayingTextLines(contentW, h)
	}

	size := art.Size{Width: artCols, Height: artRows}
	if m.artwork.rendered == nil {
		m.artwork.rendered = map[art.Size][]string{}
	}
	artLines := m.artwork.rendered[size]
	if artLines == nil && m.artwork.img != nil {
		const maxRenderedArtworkSizes = 16
		if len(m.artwork.rendered) >= maxRenderedArtworkSizes {
			m.artwork.rendered = map[art.Size][]string{}
		}
		artLines = m.renderArt(m.artwork.img, size)
		m.artwork.rendered[size] = artLines
	}

	lines := make([]string, 0, h)
	artTop := max(0, (artRegion-artRows)/2)
	for i := range artRegion {
		if ai := i - artTop; ai >= 0 && ai < len(artLines) {
			lines = append(lines, centerStr(artLines[ai], contentW))
		} else {
			lines = append(lines, "")
		}
	}

	trackLine := m.revealLine(m.renderTrackTitleLine(t, contentW, true))
	albumLine := m.revealLine(m.renderAlbumMetaLine(t, contentW, true))
	lines = append(lines, "", trackLine, albumLine, m.statusLine(contentW))
	return lines
}

// enrichTrackFromQueue merges catalog metadata (content rating, digital master,
// release date, artwork palette, genres, audio variants) from the queued provider.Track into
// the player-reported Track, since the JS player bridge only reports playback
// fields (id, title, artist, album, artworkURL, duration).
func (m *Model) enrichTrackFromQueue(t *provider.Track) {
	if t == nil {
		return
	}
	idx := enrichQueueIdx(m.queueTracks, t)
	if idx < 0 || idx >= len(m.queueTracks) {
		return
	}
	qt := m.queueTracks[idx]
	if t.CatalogID == "" {
		t.CatalogID = qt.CatalogID
	}
	if len(t.Genres) == 0 && len(qt.Genres) > 0 {
		t.Genres = qt.Genres
	}
	if t.DiscNumber == 0 {
		t.DiscNumber = qt.DiscNumber
	}
	if t.TrackNumber == 0 {
		t.TrackNumber = qt.TrackNumber
	}
	if t.ContentRating == "" {
		t.ContentRating = qt.ContentRating
	}
	if !t.IsAppleDigitalMaster && qt.IsAppleDigitalMaster {
		t.IsAppleDigitalMaster = true
	}
	if t.ReleaseDate == "" {
		t.ReleaseDate = qt.ReleaseDate
	}
	if !t.ArtworkPalette.HasColors() && qt.ArtworkPalette.HasColors() {
		t.ArtworkPalette = qt.ArtworkPalette
	}
	if len(t.AudioVariants) == 0 {
		t.AudioVariants = qt.AudioVariants
	}
}

// enrichQueueIdx locates the queued track matching playing for metadata
// enrichment. Unlike playingQueueIdx (which falls back to Title alone),
// enrichment requires either an exact PlaybackID/ID/CatalogID match or a full
// Title+Artist+Album match so a different track with the same title cannot
// lend its badges or artwork palette to the one that is playing.
func enrichQueueIdx(tracks []provider.Track, playing *provider.Track) int {
	if playing == nil {
		return -1
	}
	playingID := views.PlaybackID(*playing)
	for i, t := range tracks {
		if views.PlaybackID(t) == playingID ||
			(playing.ID != "" && (t.ID == playing.ID || t.CatalogID == playing.ID)) ||
			(playing.CatalogID != "" && (t.ID == playing.CatalogID || t.CatalogID == playing.CatalogID)) {
			return i
		}
	}
	for i, t := range tracks {
		if t.Title == playing.Title &&
			(playing.Artist == "" || strings.EqualFold(t.Artist, playing.Artist)) &&
			(playing.Album == "" || strings.EqualFold(t.Album, playing.Album)) {
			return i
		}
	}
	return -1
}

// canSwitchByKey reports whether a service hotkey (1, 2, `…) may switch
// tabs: anywhere except where those keys are typed or pick an item (search
// and command input, pickers, the song menu).
func (m *Model) canSwitchByKey() bool {
	return m.mode == modeNormal &&
		m.songMenu == nil && !m.lyricsP.m.PickerOpen()
}

func (m *Model) applyTrackPalette(t *provider.Track) {
	if m.background {
		return
	}
	if t == nil || !t.ArtworkPalette.HasColors() {
		styles.ClearArtworkPalette()
		return
	}
	p := t.ArtworkPalette
	styles.ApplyArtworkPalette(p.BgColor, p.TextColor1, p.TextColor2, p.TextColor3, p.TextColor4)
}

func (m *Model) renderTrackTitleLine(t *provider.Track, contentW int, truncate bool) string {
	muted := styles.QueueItemMuted
	var titleStr string
	if m.playerState.Playing || m.playerState.Loading {
		titleStr = styles.NowPlayingTitlePlaying.Render(t.Title)
	} else {
		titleStr = styles.NowPlayingTitle.Render(t.Title)
	}
	if strings.EqualFold(t.ContentRating, "explicit") {
		titleStr += " " + muted.Render("🅴")
	}
	line := titleStr + muted.Render(" — ") + styles.NowPlayingTrackArtist.Render(t.Artist)
	if truncate {
		line = ansi.Truncate(line, contentW, "…")
	}
	return centerStr(line, contentW)
}

func (m *Model) renderAlbumMetaLine(t *provider.Track, contentW int, truncate bool) string {
	albumText := t.Album
	if year := t.ReleaseYear(); year != "" && albumText != "" {
		albumText += " (" + year + ")"
	}
	elapsed := views.FormatDuration(m.playerState.Position)
	total := views.FormatDuration(t.Duration)

	build := func(withFormat bool) string {
		line := styles.NowPlayingAlbum.Render(albumText + " • ")
		if t.IsAppleDigitalMaster {
			line += styles.NowPlayingLabel.Render(i18n.T("✦ Master")) + styles.NowPlayingAlbum.Render(" • ")
		}
		if output, catalog := formatBadge(t, m.playerState.Bitrate); withFormat && (output != "" || catalog != "") {
			if output != "" {
				line += styles.NowPlayingLabel.Render(output)
			}
			if catalog != "" {
				if output != "" {
					line += styles.NowPlayingAlbum.Render(" ")
				}
				line += styles.NowPlayingAlbum.Render("(catalog: " + catalog + ")")
			}
			line += styles.NowPlayingAlbum.Render(" • ")
		}
		return line + styles.NowPlayingTime.Render(elapsed+" / "+total)
	}
	line := build(true)
	if truncate {
		// The format badge is the first thing to go on a narrow panel, so
		// truncation never eats the time instead.
		if lipgloss.Width(line) > contentW {
			line = build(false)
		}
		line = ansi.Truncate(line, contentW, "…")
	}
	return centerStr(line, contentW)
}

// formatBadge describes the playing track's audio for the Now Playing panel.
// output is what utuimp is playing: the AAC bitrate for streaming, the file's
// format for a local file. catalog names what Apple Music holds beyond lossy
// stereo ("Hi-Res", "Lossless", "Atmos"). The two are kept apart because
// streaming is AAC whatever the catalog holds, and "Lossless" on its own would
// read as what is coming out of the speakers. Streaming tracks whose catalog
// entry offers nothing more return no badge: the header already shows the
// bitrate.
func formatBadge(t *provider.Track, bitrateKbps int) (output, catalog string) {
	if t == nil {
		return "", ""
	}
	if path, ok := strings.CutPrefix(t.ID, "local:"); ok {
		ext := strings.TrimPrefix(filepath.Ext(path), ".")
		return strings.ToUpper(ext), ""
	}
	var parts []string
	switch {
	case slices.Contains(t.AudioVariants, "hi-res-lossless"):
		parts = append(parts, "Hi-Res")
	case slices.Contains(t.AudioVariants, "lossless"):
		parts = append(parts, "Lossless")
	}
	if slices.Contains(t.AudioVariants, "dolby-atmos") {
		parts = append(parts, "Atmos")
	}
	if len(parts) == 0 {
		return "", ""
	}
	if bitrateKbps > 0 {
		output = fmt.Sprintf("AAC %d", bitrateKbps)
	}
	return output, strings.Join(parts, ", ")
}

// artRenderer picks the album-art renderer: dithered sextants where the
// terminal can draw them, the plain half-block renderer otherwise.
func artRenderer() func(image.Image, art.Size) []string {
	if art.SupportsSextants() {
		return art.RenderDithered
	}
	return art.RenderHalfBlocks
}

// statusLine renders the centred error/status line, or "" when there is no
// message. Messages prefixed with '✓' are rendered as success (green); all
// others are treated as warnings/errors (red with ⚠ prefix).
func (m *Model) statusLine(contentW int) string {
	if m.errMsg == "" {
		if m.busyLabel == "" {
			return ""
		}
		frames := [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		text := truncateStr(m.busyLabel, max(10, contentW-2))
		return centerStr(styles.Spinner.Render(frames[m.glowStep%len(frames)])+" "+styles.QueueItemMuted.Render(text), contentW)
	}
	if strings.HasPrefix(m.errMsg, "✓") {
		text := truncateStr(m.errMsg, max(10, contentW))
		return centerStr(styles.ControlActive.Render(text), contentW)
	}
	const prefix = "⚠  "
	errText := truncateStr(m.errMsg, max(10, contentW-len([]rune(prefix))))
	return centerStr(styles.ErrorStyle.Render(prefix+errText), contentW)
}

func (m *Model) nowPlayingTextLines(contentW, h int) []string {
	muted := styles.QueueItemMuted

	t := m.playerState.Track
	if t == nil {
		lines := make([]string, h)
		mid := h / 2
		lines[h-2] = centerStr(muted.Render(i18n.T("press ? for about")), contentW)
		if m.engineErr != "" {
			// A service that failed to start: keep saying why.
			wrapped := lipgloss.NewStyle().Width(max(20, contentW-6)).Render(m.engineErr)
			rows := strings.Split(wrapped, "\n")
			top := max(0, (h-len(rows))/2-1)
			for i, r := range rows {
				if top+i < h-2 {
					lines[top+i] = centerStr(styles.ErrorStyle.Render(strings.TrimRight(r, " ")), contentW)
				}
			}
			return lines
		}
		if m.errMsg != "" {
			// statusLine renders this identically and truncates to the width,
			// which a message long enough to explain itself needs.
			lines[max(0, mid-2)] = m.statusLine(contentW)
		}
		return lines
	}

	trackLine := m.revealLine(m.renderTrackTitleLine(t, contentW, false))
	albumLine := m.revealLine(m.renderAlbumMetaLine(t, contentW, false))

	// Progress bar — centred, slightly narrower than full width
	barW := max(10, contentW-8)
	progressLine := centerStr(views.RenderProgressBar(m.smoothPosition(), t.Duration, barW), contentW)

	// Controls: ↺  ⇄  ▶/⏸  ♡/♥
	var playIcon string
	var playStyle lipgloss.Style
	switch {
	case m.playerState.Loading:
		spinnerFrames := [10]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		playIcon = spinnerFrames[m.glowStep%10]
		playStyle = styles.ControlActive
	case m.playerState.Playing:
		playIcon = "⏸"
		playStyle = styles.ControlActive
	default:
		playIcon = "▶"
		playStyle = styles.Paused
	}

	repeatIcon, repeatStyle := "↺", muted
	switch m.playerState.RepeatMode {
	case player.RepeatModeAll:
		repeatIcon, repeatStyle = "↺", styles.ControlActive
	case player.RepeatModeOne:
		repeatIcon, repeatStyle = "↻", styles.ControlActive
	}
	shuffleStyle := muted
	if m.playerState.ShuffleMode {
		shuffleStyle = styles.ControlActive
	}

	heartIcon, heartStyle := "♡", muted
	if m.isTrackFavorite(t) {
		heartIcon, heartStyle = "♥", styles.FavoriteActive
	}

	controls := centerStr(
		repeatStyle.Render(repeatIcon)+"   "+
			shuffleStyle.Render("⇄")+"   "+
			playStyle.Render(playIcon)+"   "+
			heartStyle.Render(heartIcon),
		contentW,
	)

	errLine := m.statusLine(contentW)

	lines := []string{
		"",
		trackLine,
		albumLine,
		"",
		progressLine,
		"",
		controls,
		"",
		errLine,
	}

	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines[:h]
}

// playingQueueIdx returns the queue position of the track the player reports as
// playing, or -1 if it isn't in the queue. Identity comes first, the way
// dropQueueAfter matches: a queue can hold the same song twice, or two
// different songs sharing a title, and matching on title alone marks every one
// of them. Title is kept only as a last resort, because the library fallback
// can swap a catalog entry for its library copy mid-playback, which leaves the
// two IDs legitimately different.
func playingQueueIdx(tracks []provider.Track, playing *provider.Track) int {
	if playing == nil {
		return -1
	}
	playingID := views.PlaybackID(*playing)
	for i, t := range tracks {
		if views.PlaybackID(t) == playingID {
			return i
		}
	}
	for i, t := range tracks {
		if t.Title == playing.Title {
			return i
		}
	}
	return -1
}

// queuePanelLines returns the Queue panel lines for the left split.
func (m *Model) queuePanelLines(w, h int) []string {
	total := len(m.queue.m.Tracks())

	// Header: "Queue  12 tracks"
	var headerLabel string
	if total > 0 {
		countStr := styles.QueueItemMuted.Render(fmt.Sprintf(i18n.T("  %d tracks"), total))
		headerLabel = styles.Header.Render(i18n.T("Queue")) + countStr
	} else {
		headerLabel = styles.Header.Render(i18n.T("Queue"))
	}
	sep := styles.QueueItemMuted.Render(strings.Repeat("─", 5))

	if m.queueMiniCursor >= total {
		m.queueMiniCursor = -1
	}
	indexW := len(fmt.Sprintf("%d", total)) // digit width for index numbers
	tracks := m.queue.m.Tracks()
	playingIdx := playingQueueIdx(tracks, m.playerState.Track)

	var trackLines []string
	for i, t := range tracks {
		idx := fmt.Sprintf("%*d. ", indexW, i+1)
		label := views.TrackLabel(t)
		if i == m.queueMiniCursor {
			marker := "▸ "
			if i == playingIdx {
				marker = "▶ "
			}
			line := truncateStr(label, w-2-indexW-2)
			trackLines = append(trackLines, styles.Selected.Render(idx+marker+line))
			continue
		}
		if i == playingIdx {
			numStr := styles.ControlActive.Render(idx)
			line := truncateStr(label, w-2-indexW-2)
			trackLines = append(trackLines, numStr+styles.ControlActive.Render("▶ "+line))
		} else {
			numStr := styles.QueueItemMuted.Render(idx)
			line := truncateStr(label, w-2-indexW-2)
			trackLines = append(trackLines, numStr+styles.QueueItem.Render(line))
		}
	}
	if len(trackLines) == 0 {
		trackLines = []string{styles.QueueItemMuted.Render(i18n.T("  Queue is empty"))}
		if m.emptyQueueHint != "" {
			trackLines = nil
			for l := range strings.SplitSeq(m.emptyQueueHint, "\n") {
				trackLines = append(trackLines, styles.QueueItemMuted.Render("  "+l))
			}
		}
	}

	// header + sep occupy 2 lines; remaining rows hold track entries.
	visibleRows := max(0, h-2)
	// Clamp offset so we never scroll past the end.
	maxOffset := max(0, len(trackLines)-visibleRows)
	if m.queueMiniOffset > maxOffset {
		m.queueMiniOffset = maxOffset
	}
	visible := trackLines[m.queueMiniOffset:]
	if len(visible) > visibleRows {
		visible = visible[:visibleRows]
	}

	result := append([]string{headerLabel, sep}, visible...)
	for len(result) < h {
		result = append(result, "")
	}
	return result
}

// searchLines renders the search popup inline (full-width in the split area).
func (m *Model) searchLines(contentW, h int) []string {
	accent := lipgloss.NewStyle().Foreground(styles.ColorAccent)
	muted := lipgloss.NewStyle().Foreground(styles.ColorMuted)
	textStyle := lipgloss.NewStyle().Foreground(styles.ColorFg)
	cursor := accent.Render("█")

	runes := []rune(m.searchQuery)
	cur := min(max(m.searchCursor, 0), len(runes))
	before := textStyle.Render(string(runes[:cur]))
	after := textStyle.Render(string(runes[cur:]))

	inputLine := accent.Render("/") + "  " + before + cursor + after
	sep := muted.Render(strings.Repeat("─", contentW))

	// Reserve input(1) + sep(1) + footerSep(1) + footer(1) = 4 lines.
	listH := max(1, h-4)
	m.search.SetSize(contentW, listH)
	listView := m.search.View()
	if listView == "" && !m.search.Loading() && m.searchQuery != "" {
		listView = "  " + muted.Render(i18n.T("no results"))
	}

	listLines := toLines(listView, listH)
	footerSep := muted.Render(strings.Repeat("─", contentW))

	// Make the footer hints context-sensitive to the selected item kind.
	kind := "track"
	if m.search.SelectedAlbum() != nil {
		kind = "album"
	} else if m.search.SelectedPlaylist() != nil {
		kind = "playlist"
	}
	esc := " close"
	if m.search.InArtist() {
		esc = " back"
	}
	footer := "  " + accent.Render(i18n.T("Enter")) + muted.Render(" play "+kind) +
		"  ·  " + accent.Render(i18n.T("Tab")) + muted.Render(i18n.T(" add to queue")) +
		"  ·  " + accent.Render(i18n.T("Shift+Tab")) + muted.Render(i18n.T(" play next")) +
		"  ·  " + accent.Render(i18n.T("ctrl+p")) + muted.Render(i18n.T(" add to playlist")) +
		"  ·  " + accent.Render(i18n.T("Esc")) + muted.Render(esc)
	if m.search.SelectedArtist() != nil {
		footer = "  " + accent.Render(i18n.T("Enter")) + muted.Render(i18n.T(" open artist")) +
			"  ·  " + accent.Render(i18n.T("Esc")) + muted.Render(esc)
	}

	result := append([]string{inputLine, sep}, listLines...)
	result = append(result, footerSep, footer)
	for len(result) < h {
		result = append(result, "")
	}
	return result[:h]
}

// statusNavLines is the top status line: mode chip + context-aware shortcuts,
// wrapped to fit width w.
func (m *Model) statusNavLines(w int) []string {
	muted := styles.QueueItemMuted
	accent := styles.KeyName
	dot := muted.Render("  ·  ")

	switch m.mode {
	case modeSearch:
		runes := []rune(m.searchQuery)
		cur := min(max(m.searchCursor, 0), len(runes))
		before := styles.Header.Render(string(runes[:cur]))
		after := styles.Header.Render(string(runes[cur:]))
		parts := []string{
			styles.ModeSearch.Render(i18n.T("SEARCH")) + "  " +
				accent.Render("/") + before + accent.Render("█") + after,
			accent.Render(i18n.T("Ctrl+U")) + muted.Render(i18n.T(" clear")),
		}
		return wrapFit(parts, dot, w)
	case modeCommand:
		return []string{styles.ModeCommand.Render(i18n.T("CMD")) + "  " +
			muted.Render(":") + m.cmdBuf + accent.Render("_") +
			muted.Render(i18n.T("  Tab complete · ↑/↓ navigate · Esc cancel"))}
	default:
		var parts []string
		switch {
		case m.debugView:
			parts = []string{
				styles.ModeNormal.Render(i18n.T("DEBUG")),
				accent.Render(i18n.T("j/k")) + muted.Render(i18n.T(" scroll")),
				accent.Render(i18n.T("esc")) + muted.Render(i18n.T(" close")),
			}
		case m.activePanel >= 0 && m.panels[m.activePanel] == m.historyP:
			parts = []string{
				styles.ModeNormal.Render(i18n.T("HISTORY")),
				accent.Render(i18n.T("Enter")) + muted.Render(i18n.T(" play now")),
				accent.Render(i18n.T("Tab")) + muted.Render(i18n.T(" play next")),
				accent.Render(i18n.T("p")) + muted.Render(i18n.T(" add to playlist")),
				accent.Render(i18n.T("R")) + muted.Render(i18n.T(" radio")),
				accent.Render(i18n.T("i")) + muted.Render(i18n.T(" song menu")),
				accent.Render(i18n.T("esc")) + muted.Render(i18n.T(" close")),
			}
		case m.activePanel < 0 && m.queueMiniCursor >= 0:
			parts = []string{
				styles.ModeNormal.Render(i18n.T("QUEUE")),
				accent.Render(i18n.T("Enter")) + muted.Render(i18n.T(" play")),
				accent.Render(i18n.T("d")) + muted.Render(i18n.T(" remove")),
				accent.Render(i18n.T("Shift+↑/↓ / K/J")) + muted.Render(i18n.T(" move")),
				accent.Render(i18n.T("p")) + muted.Render(i18n.T(" add to playlist")),
				accent.Render(i18n.T("c")) + muted.Render(i18n.T(" clear")),
				accent.Render(i18n.T("s")) + muted.Render(i18n.T(" :save")),
				accent.Render(i18n.T("R")) + muted.Render(i18n.T(" radio")),
				accent.Render(i18n.T("i")) + muted.Render(i18n.T(" song menu")),
				accent.Render(i18n.T("esc")) + muted.Render(i18n.T(" done")),
			}
		case m.activePanel >= 0 && m.panels[m.activePanel] == m.library:
			parts = []string{
				styles.ModeNormal.Render(i18n.T("LIBRARY")),
				accent.Render(i18n.T("Enter")) + muted.Render(i18n.T(" browse/play")),
				accent.Render(i18n.T("Tab")) + muted.Render(i18n.T(" queue")),
				accent.Render(i18n.T("Shift+Tab")) + muted.Render(i18n.T(" next")),
				accent.Render(i18n.T("esc")) + muted.Render(i18n.T(" back/close")),
			}
		case m.coverView && m.mode == modeNormal && m.activePanel >= 0 &&
			m.panels[m.activePanel] == m.lyricsP && !m.lyricsP.m.PickerOpen():
			// The cover view's home screen: everything that works here.
			parts = m.coverHomeHints()
		case m.activePanel >= 0 && m.panels[m.activePanel] == m.lyricsP:
			parts = []string{
				styles.ModeNormal.Render(i18n.T("LYRICS")),
				accent.Render(i18n.T("Enter")) + muted.Render(i18n.T(" seek")),
				accent.Render(i18n.T("j/k")) + muted.Render(i18n.T(" scroll")),
				accent.Render(i18n.T("t")) + muted.Render(i18n.T(" translate")),
				accent.Render(i18n.T("T")) + muted.Render(i18n.T(" language")),
				accent.Render(i18n.T("c")) + muted.Render(i18n.T(" source")),
				accent.Render(i18n.T("g/G")) + muted.Render(i18n.T(" top/bottom")),
				accent.Render(i18n.T("esc")) + muted.Render(i18n.T(" close")),
			}
			if m.lyricsP.m.PickerOpen() {
				parts = []string{
					styles.ModeNormal.Render(i18n.T("PICK")),
					accent.Render(i18n.T("j/k")) + muted.Render(i18n.T(" choose")),
					accent.Render("1-…") + muted.Render(i18n.T(" quick pick")),
					accent.Render(i18n.T("Enter")) + muted.Render(i18n.T(" select")),
					accent.Render(i18n.T("esc")) + muted.Render(i18n.T(" cancel")),
				}
			}
		case m.activePanel >= 0 && m.panels[m.activePanel] == m.feedP:
			parts = []string{
				styles.ModeNormal.Render(i18n.T("FEED")),
				accent.Render(i18n.T("Enter")) + muted.Render(i18n.T(" play")),
				accent.Render(i18n.T("Tab")) + muted.Render(i18n.T(" queue")),
				accent.Render(i18n.T("Shift+Tab")) + muted.Render(i18n.T(" next")),
				accent.Render(i18n.T("j/k")) + muted.Render(i18n.T(" navigate")),
				accent.Render(i18n.T("r")) + muted.Render(i18n.T(" refresh")),
				accent.Render(i18n.T("esc")) + muted.Render(i18n.T(" close")),
			}
		case m.activePanel >= 0 && m.panels[m.activePanel] == m.eqP:
			parts = []string{
				styles.ModeNormal.Render(i18n.T("EQUALIZER")),
				accent.Render("←/→") + muted.Render(i18n.T(" band")),
				accent.Render("↑/↓") + muted.Render(i18n.T(" gain")),
				accent.Render("0") + muted.Render(i18n.T(" reset band")),
				accent.Render(i18n.T("r")) + muted.Render(i18n.T(" reset all")),
				accent.Render(i18n.T("e")) + muted.Render(i18n.T(" close")),
			}
		case m.activePanel >= 0 && m.panels[m.activePanel] == m.aboutP:
			parts = []string{
				styles.ModeNormal.Render(i18n.T("ABOUT")),
				accent.Render(i18n.T("esc/?")) + muted.Render(i18n.T(" close")),
			}
		default:
			parts = []string{
				styles.ModeNormal.Render(i18n.T("NORMAL")),
				accent.Render(":") + muted.Render(i18n.T(" command")),
				accent.Render("/") + muted.Render(i18n.T(" search")),
				accent.Render(i18n.T("l")) + muted.Render(i18n.T(" library")),
				accent.Render(i18n.T("q")) + muted.Render(i18n.T(" history")),
				accent.Render("↑/↓") + muted.Render(i18n.T(" pick song")),
				accent.Render(i18n.T("y")) + muted.Render(i18n.T(" lyrics")),
				accent.Render(i18n.T("F")) + muted.Render(i18n.T(" feed")),
				accent.Render(i18n.T("e")) + muted.Render(i18n.T(" equalizer")),
				accent.Render(i18n.T("i")) + muted.Render(i18n.T(" song menu")),
				accent.Render("v") + muted.Render(m.layoutHint()),
				accent.Render("?") + muted.Render(i18n.T(" about")),
				accent.Render(i18n.T(":q")) + muted.Render(i18n.T(" quit")),
			}
			if m.coverView {
				parts = append(parts, accent.Render(i18n.T("esc"))+muted.Render(i18n.T(" lyrics")))
			}
		}
		return wrapFit(parts, dot, w)
	}
}

// statusPlayLines is the bottom status line: always shows playback controls,
// wrapped to fit width w.
func (m *Model) statusPlayLines(w int) []string {
	muted := styles.QueueItemMuted
	accent := styles.KeyName
	dot := muted.Render("  ·  ")

	radioName := "radio"
	if _, wave := m.provider.(interface{ ResetStation(string) }); wave {
		radioName = "song wave" // Yandex: "My Wave" seeded by the current song
	}
	radioHint := accent.Render(i18n.T("R")) + muted.Render(" "+i18n.T(radioName))
	if m.radio.enabled && m.radio.seed != nil {
		on := radioName
		if strings.HasPrefix(m.radio.seed.ID, "wave:") {
			on = "my wave"
		}
		radioHint = accent.Render(i18n.T("R")) + styles.Playing.Render(" ● "+i18n.T(on))
	}

	parts := []string{
		accent.Render(i18n.T("spc")) + muted.Render(i18n.T(" play/pause")),
		accent.Render(i18n.T("n/p")) + muted.Render(i18n.T(" next/prev")),
		accent.Render("←/→") + muted.Render(i18n.T(" seek ±10s")),
		accent.Render(i18n.T("f")) + muted.Render(i18n.T(" favorite")),
		accent.Render(i18n.T("s")) + muted.Render(i18n.T(" shuffle")),
		accent.Render(i18n.T("r")) + muted.Render(i18n.T(" repeat")),
		radioHint,
		accent.Render("+/-") + muted.Render(i18n.T(" volume")),
	}
	return wrapFit(parts, dot, w)
}

// commandLines renders the command palette in the panel area when CMD mode is active.
func (m *Model) commandLines(_ int, h int) []string {
	muted := styles.QueueItemMuted
	accent := styles.KeyName
	header := accent.Render(i18n.T("Commands"))
	sep := muted.Render(strings.Repeat("─", 8))

	suggs := m.commandSuggestions()
	var rows []string
	for i, c := range suggs {
		cursor := "  "
		nameStyle := styles.QueueItem
		descStyle := muted
		if i == m.cmdSuggIdx {
			cursor = styles.Playing.Render("▶ ")
			nameStyle = styles.Playing
			descStyle = styles.QueueItem
		}
		usage := nameStyle.Render(fmt.Sprintf(i18n.T("%-20s"), c.usage))
		desc := descStyle.Render(i18n.T(c.description))
		rows = append(rows, cursor+usage+" "+desc)
	}
	if len(rows) == 0 {
		rows = []string{"  " + muted.Render(i18n.T("no matching commands"))}
	}

	result := append([]string{"", header, sep, ""}, rows...)
	for len(result) < h {
		result = append(result, "")
	}
	return result[:h]
}

// statusLines returns every status row — nav hints then playback controls —
// each already wrapped to fit width w. The count varies with terminal width,
// so panelHeight consults it rather than assuming a fixed two rows.
func (m *Model) statusLines(w int) []string {
	return append(m.statusNavLines(w), m.statusPlayLines(w)...)
}

// panelHeight returns the number of rows available inside the panel box.
// Fixed overhead = the two boxes' top and bottom borders (4), plus the
// now-playing section, which grows in art mode, and the status rows, which
// grow as hints wrap on a narrow terminal — so both are measured rather than
// assumed.
func (m *Model) panelHeight() int {
	if m.coverView {
		return m.coverPanelHeight()
	}
	fixedOverhead := 4 + m.nowPlayingHeight() + len(m.statusLines(m.width-4))
	return max(3, m.height-fixedOverhead)
}

// ── Helpers ───────────────────────────────────────────────────────────────

// padRight pads s on the right with spaces to reach visual width w, and clips
// it when it is wider. Clipping is the backstop that keeps an over-long line
// from pushing past the right border and breaking the vertical rule; it is
// ANSI-aware so styled content is never cut mid-escape-sequence.
func padRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := lipgloss.Width(s)
	switch {
	case sw > w:
		return ansi.Truncate(s, w, "…")
	case sw == w:
		return s
	default:
		return s + strings.Repeat(" ", w-sw)
	}
}

// wrapFit packs parts into as many lines as needed, each at most visual width
// w, joining the parts on a line with sep. Nothing is ever hidden: a narrow
// terminal costs extra rows rather than dropped hints. Parts are never split,
// so styled segments stay intact — a lone part wider than w gets its own line
// and is clipped by padRight. Always returns at least one line.
func wrapFit(parts []string, sep string, w int) []string {
	if len(parts) == 0 || w <= 0 {
		return []string{""}
	}

	sepW := lipgloss.Width(sep)
	var lines []string
	cur, curW := "", 0

	for _, p := range parts {
		pw := lipgloss.Width(p)
		switch {
		case cur == "":
			cur, curW = p, pw
		case curW+sepW+pw <= w:
			cur += sep + p
			curW += sepW + pw
		default:
			lines = append(lines, cur)
			cur, curW = p, pw
		}
	}
	return append(lines, cur)
}

// toLines splits s into exactly h lines, padding/truncating as needed.
func toLines(s string, h int) []string {
	lines := strings.Split(s, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines[:h]
}

// safeIdx returns lines[i] or "" if i is out of range.
func safeIdx(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

// truncateStr truncates s to at most maxW runes, adding "…" if cut.
func truncateStr(s string, maxW int) string {
	if maxW <= 1 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxW {
		return s
	}
	return string(runes[:maxW-1]) + "…"
}

// appendLog adds a timestamped entry to the debug log, capped at 500 lines.
func (m *Model) appendLog(line string) {
	const maxLines = 500
	ts := time.Now().Format("15:04:05")
	m.debugLog = append(m.debugLog, ts+"  "+line)
	if len(m.debugLog) > maxLines {
		m.debugLog = m.debugLog[len(m.debugLog)-maxLines:]
	}
}

// debugLogLines renders the debug log as exactly h lines for the split area.
func (m *Model) debugLogLines(w, h int) []string {
	accent := styles.Header
	muted := styles.QueueItemMuted

	header := accent.Render(i18n.T("Debug Log")) + "  " + muted.Render(i18n.T("esc / :debug-logs to close · k/j scroll"))
	sep := muted.Render(strings.Repeat("─", 9))
	contentH := max(0, h-2)

	total := len(m.debugLog)
	scroll := min(m.debugScroll, max(0, total-contentH))
	start := max(0, total-contentH-scroll)
	end := min(total, start+contentH)

	// Maximum text width: panel width minus the 2-char indent and 1-char safety margin.
	maxW := max(1, w-3)

	truncate := func(s string) string {
		r := []rune(s)
		if len(r) > maxW {
			return string(r[:maxW-1]) + "…"
		}
		return s
	}

	lines := []string{header, sep}
	if total == 0 {
		lines = append(lines, "  "+muted.Render(i18n.T("no log entries yet")))
	} else {
		for _, entry := range m.debugLog[start:end] {
			clipped := truncate(entry)
			var rendered string
			switch {
			case strings.Contains(entry, "[error]"):
				rendered = styles.ErrorStyle.Render(clipped)
			case strings.Contains(entry, "[playing]"):
				rendered = styles.Playing.Render(clipped)
			case strings.Contains(entry, "[js:"):
				rendered = styles.Header.Render(clipped)
			default:
				rendered = muted.Render(clipped)
			}
			lines = append(lines, "  "+rendered)
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines[:h]
}

func centerStr(s string, width int) string {
	w := lipgloss.Width(s)
	pad := max(0, (width-w)/2)
	return strings.Repeat(" ", pad) + s
}

// qualityLabel converts a streaming bitrate (kbps) to a human-readable label.
// Apple Music tiers: AAC ≤ 320 kbps, Lossless ~1411 kbps, Hi-Res > 2000 kbps.
// Returns "" when bitrate is 0 (unknown / nothing playing).
func qualityLabel(kbps int) string {
	switch {
	case kbps <= 0:
		return ""
	case kbps > 2000:
		return "Hi-Res"
	case kbps > 320:
		return "Lossless"
	default:
		return fmt.Sprintf("%d kbps", kbps)
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ── Playlist picker ───────────────────────────────────────────────────────

const pickerMaxVisible = 8

// openPlaylistPicker enters modePlaylistPicker for the given track, saving the
// current mode so it can be restored on close.
func (m *Model) openPlaylistPicker(t *provider.Track) tea.Cmd {
	m.playlistPickerTrack = t
	m.playlistPickerLoading = true
	m.playlistPickerCursor = 0
	m.playlistPickerItems = nil
	m.playlistPickerReturnMode = m.mode
	m.mode = modePlaylistPicker
	return m.fetchPlaylistsForPickerCmd()
}

// fetchPlaylistsForPickerCmd fires a background fetch of library playlists.
// The generation counter prevents stale responses from landing after the picker
// has been closed and reopened.
func (m *Model) fetchPlaylistsForPickerCmd() tea.Cmd {
	m.playlistPickerGen++
	gen := m.playlistPickerGen
	prov := m.provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		playlists, err := prov.GetLibraryPlaylists(ctx)
		return playlistsForPickerMsg{playlists: playlists, err: err, gen: gen}
	}
}

// handlePlaylistPickerKey handles key presses while the playlist picker is open.
func (m *Model) handlePlaylistPickerKey(k string) tea.Cmd {
	switch k {
	case "esc":
		m.mode = m.playlistPickerReturnMode
		return nil
	case "up", "k":
		if m.playlistPickerCursor > 0 {
			m.playlistPickerCursor--
		}
	case "down", "j":
		if m.playlistPickerCursor < len(m.playlistPickerItems)-1 {
			m.playlistPickerCursor++
		}
	case "enter":
		if m.playlistPickerLoading || len(m.playlistPickerItems) == 0 {
			return nil
		}
		if m.playlistPickerCursor >= len(m.playlistPickerItems) {
			m.playlistPickerCursor = len(m.playlistPickerItems) - 1
		}
		pl := m.playlistPickerItems[m.playlistPickerCursor]
		t := m.playlistPickerTrack
		m.mode = m.playlistPickerReturnMode
		prov := m.provider
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			// Prefer catalog ID for playback-sourced tracks; fall back to library ID.
			trackID := t.ID
			if !strings.HasPrefix(t.ID, "i.") && t.CatalogID != "" {
				trackID = t.CatalogID
			}
			err := prov.AddToPlaylist(ctx, pl.ID, trackID)
			return trackAddedToPlaylistMsg{playlistName: pl.Name, err: err}
		}
	}
	return nil
}

// pickerWindow returns the start and end indices of the visible playlist items
// window, keeping the cursor roughly centred.
func (m *Model) pickerWindow() (start, end int) {
	n := len(m.playlistPickerItems)
	visible := min(pickerMaxVisible, n)
	start = max(0, m.playlistPickerCursor-visible/2)
	end = start + visible
	if end > n {
		end = n
		start = max(0, end-visible)
	}
	return
}

// renderPlaylistPickerModal renders the centered "Add to playlist" modal.
func (m *Model) renderPlaylistPickerModal() string {
	innerW := min(46, m.width-8)
	sep := styles.QueueItemMuted.Render(strings.Repeat("─", innerW))

	var lines []string

	// Title line
	title := "Add to playlist"
	if m.playlistPickerTrack != nil {
		title = "+ " + truncateStr(m.playlistPickerTrack.Title, innerW-3)
	}
	lines = append(lines, styles.NowPlayingTitlePlaying.Render(title))
	lines = append(lines, sep)

	switch {
	case m.playlistPickerLoading:
		lines = append(lines, styles.QueueItemMuted.Render(i18n.T("  Loading playlists…")))
	case len(m.playlistPickerItems) == 0:
		lines = append(lines, styles.QueueItemMuted.Render(i18n.T("  No playlists found")))
	default:
		start, end := m.pickerWindow()
		if start > 0 {
			lines = append(lines, styles.QueueItemMuted.Render(fmt.Sprintf(i18n.T("  ↑ %d more"), start)))
		}
		for i := start; i < end; i++ {
			name := truncateStr(m.playlistPickerItems[i].Name, innerW-3)
			if i == m.playlistPickerCursor {
				lines = append(lines, styles.Playing.Render("▶ "+name))
			} else {
				lines = append(lines, styles.QueueItem.Render("  "+name))
			}
		}
		remaining := len(m.playlistPickerItems) - end
		if remaining > 0 {
			lines = append(lines, styles.QueueItemMuted.Render(fmt.Sprintf(i18n.T("  ↓ %d more"), remaining)))
		}
	}

	lines = append(lines, sep)
	lines = append(lines, styles.QueueItemMuted.Render(i18n.T("  ↑↓/jk navigate · ⏎ add · esc cancel")))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorPrimary).
		Padding(0, 1).
		Width(innerW).
		Render(strings.Join(lines, "\n"))
}

func configEQBandsToPlayer(bands []config.EQBand) []player.EQBand {
	if len(bands) == 0 {
		return nil
	}
	out := make([]player.EQBand, len(bands))
	for i, b := range bands {
		out[i] = player.EQBand{Frequency: b.Frequency, Q: b.Q, Gain: b.Gain}
	}
	return out
}

func playerEQBandsToConfig(bands []player.EQBand) []config.EQBand {
	out := make([]config.EQBand, len(bands))
	for i, b := range bands {
		out[i] = config.EQBand{Frequency: b.Frequency, Q: b.Q, Gain: b.Gain}
	}
	return out
}
