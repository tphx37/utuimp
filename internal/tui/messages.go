package tui

import (
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
)

// Options configures optional TUI features at startup.
type Options struct {
	// MemProfiling enables live RSS display in the header.
	// Activate with --mem-profiling on the command line.
	MemProfiling bool
	// IconPath is the absolute path of the installed app icon (may be empty).
	IconPath string
	// Backend is a human-readable description of the audio backend, logged in
	// the debug view on startup. Leave empty if the engine starts asynchronously.
	Backend string
	// InitialTracks pre-populates the queue panel on launch (used by local mode).
	InitialTracks []provider.Track
	// ScanNotice summarises what a local-mode directory scan left out. It is
	// recorded in the debug log on startup, and shown in the status bar when
	// the scan produced no tracks at all, because an empty library otherwise
	// looks identical to a mistyped path.
	ScanNotice string
	// InitialLogs pre-populates the debug log on launch (e.g. broker token fetch logs).
	InitialLogs []string
	// HistoryName names the play-history file (per service); "" = "history".
	HistoryName string
	// EmptyQueueHint replaces "Queue is empty" (e.g. where local music goes).
	EmptyQueueHint string
	// Restart, when set, arranges for utuimp to start again once the
	// program quits (the settings use it to apply what needs a restart).
	Restart func()
	// OnSetting, when set, is told the key of every setting changed in the
	// settings panel (internal/settings), after the config has the new
	// value: the host applies what lives outside the TUI (tabs, Discord,
	// the theme, Last.fm).
	OnSetting func(key string)
	// OpenSettings opens the settings panel at start-up, on the section
	// with that key ("services" for the first one).
	OpenSettings string
	// Welcome greets a first-time user in the settings panel.
	Welcome bool
}

// RefreshLocalMsg requests a rescan after changing local music folders.
type RefreshLocalMsg struct{}

// ServiceOffMsg tells a service's Model its tab was turned off in the
// settings: it goes quiet.
type ServiceOffMsg struct{}

// InitStatusMsg updates the status text shown on the loading screen.
type InitStatusMsg string

// AuthPendingMsg asks the service screen to show the start screen while an
// external sign-in is in progress (including when its tab was opened later).
type AuthPendingMsg struct{}

// EngineReadyMsg is sent when the audio engine and provider are both ready.
type EngineReadyMsg struct {
	Player   player.Player
	Provider provider.Provider
	Backend  string // human-readable backend description, shown in the debug log
	// Tracks, when set, become the initial queue (local files).
	Tracks []provider.Track
	// Notice is shown in the status bar (e.g. a local scan that found nothing).
	Notice string
}

// InitErrMsg is sent when initialization fails fatally.
type InitErrMsg struct{ Err error }

// RestartMsg is sent when utuimp has self-updated and needs to re-exec.
// The model handles it by quitting cleanly so the caller can exec the new binary.
type RestartMsg struct{}

// DebugLogMsg appends a line to the TUI debug log (visible via :debug-logs).
// Use this to surface background-goroutine events (e.g. scrobbling) without
// printing to stderr.
type DebugLogMsg string

// playlistsForPickerMsg is the response to a library-playlists fetch for the
// playlist picker modal.
type playlistsForPickerMsg struct {
	playlists []provider.Playlist
	err       error
	gen       int // stale-response guard; discarded if != m.playlistPickerGen
}

// trackAddedToPlaylistMsg is the response to an AddToPlaylist call.
type trackAddedToPlaylistMsg struct {
	playlistName string
	err          error
}
