package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/audioquality"
)

var saveMu sync.Mutex

// startHome is the home directory the process started with; see
// errTestDefaultPath.
var startHome, _ = os.UserHomeDir() //nolint:gochecknoglobals // read once at start-up

// errTestDefaultPath stops a test from overwriting the real config: a
// config built in a test has no path, and Save("") would otherwise land in
// the user's ~/.config/utuimp/config.json. Tests that mean to save there set
// HOME to a temporary directory first.
var errTestDefaultPath = errors.New("config: a test may not save to the real default config path")

type Config struct {
	// path remembers where this config was loaded from. It is deliberately not
	// serialized; it only ensures that Save("") persists back to the selected
	// --config file instead of silently falling back to the default profile.
	path string
	// extra holds keys of the file this build doesn't know (written by a
	// newer one), saved back unchanged; see keep.go.
	extra map[string]json.RawMessage

	AppleDeveloperToken string `json:"apple_developer_token"`
	AppleUserToken      string `json:"apple_user_token"`
	// AppleTokenSource is whose developer token Apple Music may run on:
	// AppleTokenVibez (borrow the one of the vibez project), AppleTokenOwn
	// (only apple_developer_token, from the user's own MusicKit key) or
	// empty, when the user hasn't been asked yet.
	AppleTokenSource string `json:"apple_token_source,omitempty"`
	AppleKeyID       string `json:"apple_key_id"`
	AppleTeamID      string `json:"apple_team_id"`
	StoreFront       string `json:"storefront"`
	AuthPort         int    `json:"auth_port"`
	Provider         string `json:"provider"`
	Theme            string `json:"theme"`
	AudioQuality     string `json:"audio_quality,omitempty"`
	// Volume is the last user-set playback volume (0.0–1.0). nil means
	// "not yet saved" and the player default (1.0) is used on startup.
	Volume *float64 `json:"volume,omitempty"`
	// Last.fm scrobbling. LastfmAPIKey and LastfmAPISecret are typically
	// embedded in the binary at build time via ldflags; set them manually here
	// only when building from source without the embedded keys.
	LastfmAPIKey     string `json:"lastfm_api_key,omitempty"`
	LastfmAPISecret  string `json:"lastfm_api_secret,omitempty"`
	LastfmSessionKey string `json:"lastfm_session_key,omitempty"`

	// Yandex Music OAuth credentials (device-flow sign-in, see utuimp auth yandex).
	YandexToken        string `json:"yandex_token,omitempty"`
	YandexRefreshToken string `json:"yandex_refresh_token,omitempty"`
	// YandexTokenExpiry is the access token's expiry (unix seconds); 0 = unknown.
	YandexTokenExpiry int64 `json:"yandex_token_expiry,omitempty"`

	// Spotify: OAuth tokens from signing in. SpotifyClientID overrides the
	// public client used to sign in (rarely needed). LibrespotPath points at
	// librespot when it isn't on PATH.
	SpotifyClientID     string `json:"spotify_client_id,omitempty"`
	LibrespotPath       string `json:"librespot_path,omitempty"`
	SpotifyToken        string `json:"spotify_token,omitempty"`
	SpotifyRefreshToken string `json:"spotify_refresh_token,omitempty"`
	SpotifyTokenExpiry  int64  `json:"spotify_token_expiry,omitempty"`

	// YouTube Music plays through yt-dlp and ffmpeg (YtDlpPath / FFmpegPath
	// when they aren't on PATH). YouTubeCookies is yt-dlp's cookie source,
	// for when YouTube asks to prove you're not a bot: a browser name
	// ("firefox", "chrome"…) or the path of a cookies.txt file.
	YtDlpPath      string `json:"ytdlp_path,omitempty"`
	FFmpegPath     string `json:"ffmpeg_path,omitempty"`
	YouTubeCookies string `json:"youtube_cookies,omitempty"`

	// SoundCloud: the user's own app (soundcloud.com/you/apps) and the
	// OAuth tokens from signing in with it.
	SoundCloudClientID     string `json:"soundcloud_client_id,omitempty"`
	SoundCloudClientSecret string `json:"soundcloud_client_secret,omitempty"`
	SoundCloudToken        string `json:"soundcloud_token,omitempty"`
	SoundCloudRefreshToken string `json:"soundcloud_refresh_token,omitempty"`
	SoundCloudTokenExpiry  int64  `json:"soundcloud_token_expiry,omitempty"`

	// Language is the interface language ("en", "ru"); "" = from the locale.
	Language string `json:"language,omitempty"`
	// Services lists the enabled service tabs ("yandex", "apple", "spotify",
	// "youtube", "soundcloud", "local") in order; empty means all.
	Services []string `json:"services,omitempty"`
	// SetupDone is set once `utuimp setup` has run.
	SetupDone bool `json:"setup_done,omitempty"`

	// Service is the service tab shown on start ("yandex", "apple", "local", …).
	Service string `json:"service,omitempty"`
	// ServiceKeys maps keys to service tabs, e.g. {"1": "yandex"}. Missing
	// entries fall back to the defaults (1 yandex, 2 apple, 3 spotify,
	// ` local).
	ServiceKeys map[string]string `json:"service_keys,omitempty"`
	// EQBands stores the last saved 10-band equalizer settings. nil means flat.
	EQBands []EQBand `json:"eq_bands,omitempty"`
	// WSL enables audio tuning workarounds for WSL2 environments where Hyper-V
	// scheduler jitter causes audio underruns with default Chrome buffer sizes.
	WSL bool `json:"wsl,omitempty"`
	// AlbumArt starts the now-playing panel in the album-art view (the :art
	// command toggles it at runtime and persists the choice here).
	AlbumArt bool `json:"album_art,omitempty"`
	// Layout picks the interface: "" for the boxed one, "cover" for the
	// frameless cover view (the v key toggles it and saves it here).
	Layout string `json:"layout,omitempty"`
	// CoverHideLyrics hides the lyrics in the cover view (y), which then
	// shows the song and its controls beside the cover.
	CoverHideLyrics bool `json:"cover_hide_lyrics,omitempty"`
	// VisualizerScene is the visualizer scene last picked with tab
	// ("tunnel", "terrain", "ribbons" or "orb").
	VisualizerScene string `json:"visualizer_scene,omitempty"`
	// VisualizerStrength is how hard the visualizer moves, in percent of
	// its full swing (nil = 100): less for anyone it makes dizzy.
	VisualizerStrength *int `json:"visualizer_strength,omitempty"`
	// VisualizerBrightness is how bright the visualizer's picture is, in
	// percent (nil = 100).
	VisualizerBrightness *int `json:"visualizer_brightness,omitempty"`
	// VisualizerBackground is drawn behind every scene: "" for none,
	// "sparks" (drifting, as behind the ribbons) or "stars" (standing
	// still, a steady backdrop for the eye).
	VisualizerBackground string `json:"visualizer_background,omitempty"`
	// Graphics says whether pictures are drawn as real pixels in terminals
	// that can show them (kitty, Ghostty): "" or "auto" where the terminal
	// is known to, "on" always, "off" never (cells only, as everywhere
	// else).
	Graphics string `json:"graphics,omitempty"`
	// VisualizerDetail is how sharp the visualizer's picture is where it
	// is drawn in real pixels: "" (smooth, light on the processor) or
	// "fine" (several times the pixels; several cores).
	VisualizerDetail string `json:"visualizer_detail,omitempty"`
	// UIStyle is what the interface's lines are made of: "" (pictures of
	// fine lines that sway with the music, where the terminal shows real
	// pixels; cells elsewhere) or StyleSimple (cells everywhere).
	UIStyle string `json:"ui_style,omitempty"`
	// The parts of that picture, each on its own (UIStyle StyleSimple turns
	// them all off): "" is the picture, PartCells the terminal's cells.
	// UICover: the cover. UILines: the frames, rules, the progress and
	// volume lines and the mark of the chosen row; LinesCalm draws them
	// still, not swaying with the music nor answering keys. UIText: the
	// lyrics and the player's words in the system's font. UIButtons: the
	// player's buttons. UILight: the light behind the cover, "" or LightOff.
	UICover   string `json:"ui_cover,omitempty"`
	UILines   string `json:"ui_lines,omitempty"`
	UIText    string `json:"ui_text,omitempty"`
	UIButtons string `json:"ui_buttons,omitempty"`
	UILight   string `json:"ui_light,omitempty"`
	// HideHints keeps the key hints at the bottom of the screen out of the
	// way (H shows and hides them).
	HideHints bool `json:"hide_hints,omitempty"`
	// FrameRate is how many frames a second the animations are drawn at:
	// "" as many as fit the screen's refresh (60 or 72), "60", or "30" to
	// spare the battery.
	FrameRate string `json:"frame_rate,omitempty"`
	// SeekStep is how far ← and → move in the song, in seconds (0: 10).
	SeekStep int `json:"seek_step,omitempty"`
	// VolumeStep is how far + and - turn the volume, in percent (0: 5).
	VolumeStep int `json:"volume_step,omitempty"`
	// LyricsSize is the size of the cover view's lyrics drawn as a picture:
	// "" normal, "small" or "large".
	LyricsSize string `json:"lyrics_size,omitempty"`
	// Notify is when a new song is announced on the desktop: "" never,
	// "away" while utuimp's window is not the one in use, "always".
	Notify string `json:"notify,omitempty"`
	// UIMotion is how lively the interface is: "off", "calm" (a flash on
	// keys and song changes), "lively" (and the borders move to the beat;
	// the default) or "wild".
	UIMotion string `json:"ui_motion,omitempty"`
	// LibraryDelete allows deleting from the library in utuimp: playlists,
	// songs from playlists, and likes taken back (songs, saved albums,
	// followed artists). Off by default.
	LibraryDelete bool `json:"library_delete,omitempty"`
	// LibraryEdit allows changing what is in the library: renaming
	// playlists (off by default). Adding is always allowed.
	LibraryEdit bool `json:"library_edit,omitempty"`
	// MusicDir is the local directory to scan for audio files
	// Supports MP3, FLAC, M4A. Scanned recursively.
	MusicDir string `json:"music_dir,omitempty"`
	// DiscordRPC enables Discord Rich Presence (defaults to true).
	DiscordRPC *bool `json:"discord_rpc,omitempty"`
	// Intro shows the start screen (the logo over dancing ribbons) while
	// the services connect (defaults to true).
	Intro *bool `json:"intro,omitempty"`
	// DiscordClientID overrides the default Discord application client ID.
	DiscordClientID string `json:"discord_client_id,omitempty"`
	// TranslateLang is the language lyrics are translated into (ISO code such
	// as "ru"). Empty means derive it from the locale environment.
	TranslateLang string `json:"translate_lang,omitempty"`
	// LyricsSources remembers, per track, the lyrics service the user picked
	// with the source picker. Tracks not listed use automatic selection.
	LyricsSources map[string]string `json:"lyrics_sources,omitempty"`
}

// maxLyricsSourceOverrides bounds LyricsSources so the config file can't
// grow without limit.
const maxLyricsSourceOverrides = 1000

// SetLyricsSource records the lyrics service picked for a track; "" or
// "auto" clears the override.
func (c *Config) SetLyricsSource(trackID, source string) {
	if trackID == "" {
		return
	}
	if source == "" || source == "auto" {
		delete(c.LyricsSources, trackID)
		return
	}
	if c.LyricsSources == nil {
		c.LyricsSources = map[string]string{}
	}
	if _, ok := c.LyricsSources[trackID]; !ok && len(c.LyricsSources) >= maxLyricsSourceOverrides {
		for k := range c.LyricsSources { // drop an arbitrary entry
			delete(c.LyricsSources, k)
			break
		}
	}
	c.LyricsSources[trackID] = source
}

// StyleSimple is the Config.UIStyle that keeps the interface in cells.
const StyleSimple = "simple"

// The values of the parts of the interface's picture (see Config.UICover).
const (
	PartCells = "cells"
	LinesCalm = "calm"
	LightOff  = "off"
)

// Values of Config.AppleTokenSource.
const (
	AppleTokenVibez = "vibez"
	AppleTokenOwn   = "own"
)

type EQBand struct {
	Frequency float64 `json:"frequency"`
	Q         float64 `json:"q"`
	Gain      float64 `json:"gain"`
}

// VolumeOrDefault returns the saved volume, or 1.0 if none has been stored yet.
func (c *Config) VolumeOrDefault() float64 {
	if c.Volume != nil {
		return *c.Volume
	}
	return 1.0
}

// DiscordRPCEnabled reports whether Discord Rich Presence is enabled (default true).
func (c *Config) DiscordRPCEnabled() bool {
	if c.DiscordRPC != nil {
		return *c.DiscordRPC
	}
	return true
}

// IntroEnabled reports whether the start screen is shown (default true).
func (c *Config) IntroEnabled() bool {
	if c.Intro != nil {
		return *c.Intro
	}
	return true
}

// VizStrength is the visualizer's strength as a factor (1 = full swing).
func (c *Config) VizStrength() float64 {
	if c.VisualizerStrength == nil {
		return 1
	}
	return float64(min(200, max(0, *c.VisualizerStrength))) / 100
}

// VizBrightness is the visualizer's brightness as a factor (1 = as drawn).
func (c *Config) VizBrightness() float64 {
	if c.VisualizerBrightness == nil {
		return 1
	}
	return float64(min(150, max(10, *c.VisualizerBrightness))) / 100
}

// SeekStepDur is how far ← and → move in the song (see SeekStep).
func (c *Config) SeekStepDur() time.Duration {
	if c.SeekStep <= 0 {
		return 10 * time.Second
	}
	return time.Duration(min(c.SeekStep, 600)) * time.Second
}

// VolumeStepFrac is how far + and - turn the volume, 0..1 (see VolumeStep).
func (c *Config) VolumeStepFrac() float64 {
	if c.VolumeStep <= 0 {
		return 0.05
	}
	return float64(min(c.VolumeStep, 50)) / 100
}

// LyricsScale is how much larger than normal the cover view's lyrics are
// drawn (see LyricsSize).
func (c *Config) LyricsScale() float64 {
	switch c.LyricsSize {
	case LyricsSmall:
		return 0.8
	case LyricsLarge:
		return 1.25
	}
	return 1
}

// Lyrics sizes, see LyricsSize; notification modes, see Notify.
const (
	LyricsSmall  = "small"
	LyricsLarge  = "large"
	NotifyAway   = "away"
	NotifyAlways = "always"
)

// UI motion levels, see UIMotion.
const (
	MotionOff    = "off"
	MotionCalm   = "calm"
	MotionLively = "lively"
	MotionWild   = "wild"
)

// Motion is the interface's animation level; an unknown value is the
// default.
func (c *Config) Motion() string {
	switch c.UIMotion {
	case MotionOff, MotionCalm, MotionWild:
		return c.UIMotion
	}
	return MotionLively
}

// SetVolume updates the in-memory volume field. Call Save to persist it.
func (c *Config) SetVolume(v float64) {
	c.Volume = &v
}

func defaults() *Config {
	return &Config{
		AuthPort:     7777,
		Provider:     "apple",
		Theme:        "default",
		AudioQuality: "high",
	}
}

func (c *Config) AudioBitrateKbps() (int, error) {
	return audioquality.Parse(c.AudioQuality)
}

func (c *Config) SetAudioBitrate(kbps int) error {
	value, err := audioquality.ConfigValue(kbps)
	if err != nil {
		return err
	}
	c.AudioQuality = value
	return nil
}

func ConfigPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if runtime.GOOS == "windows" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("could not determine config directory: %w", err)
		}
		return filepath.Join(dir, "utuimp", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "utuimp", "config.json"), nil
}

func Load(override string) (*Config, error) {
	path, err := ConfigPath(override)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path comes from user config, not external input
	if os.IsNotExist(err) {
		cfg := defaults()
		cfg.path = path
		if saveErr := cfg.save(path); saveErr != nil {
			return nil, fmt.Errorf("creating default config: %w", saveErr)
		}
		fmt.Printf("Created default config at %s\n", path)
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.path = path
	cfg.absorb(data, path)
	cfg.normalize()
	return cfg, nil
}

// normalize replaces zero values with defaults so that an existing config file
// with missing or empty fields still behaves correctly.
func (c *Config) normalize() {
	if c.AuthPort == 0 {
		c.AuthPort = 7777
	}
	if c.Provider == "" {
		c.Provider = "apple"
	}
	if c.AudioQuality == "" {
		c.AudioQuality = "high"
	}
}

func (c *Config) Save(override string) error {
	saveMu.Lock()
	defer saveMu.Unlock()

	path := override
	if path == "" {
		path = c.path
	}
	if path == "" {
		if home, _ := os.UserHomeDir(); testing.Testing() && home == startHome {
			return errTestDefaultPath
		}
		var err error
		path, err = ConfigPath("")
		if err != nil {
			return err
		}
	}
	if err := c.save(path); err != nil {
		return err
	}
	c.path = path
	return nil
}

func (c *Config) save(path string) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := c.marshal()
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp config: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if runtime.GOOS != "windows" {
		if err := tmp.Chmod(0o600); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("chmod temp config: %w", err)
		}
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}
	cleanup = false
	// Mirror the credentials (see keep.go). A failure here leaves the
	// config itself saved, so it isn't reported.
	if creds := credentialsJSON(data); creds != nil {
		cp := credentialsPath(path)
		if err := os.WriteFile(cp+".tmp", creds, 0o600); err == nil {
			_ = os.Rename(cp+".tmp", cp)
		}
	}
	return nil
}
