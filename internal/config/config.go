package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/tphx37/utuimp/internal/audioquality"
)

var saveMu sync.Mutex

type Config struct {
	// path remembers where this config was loaded from. It is deliberately not
	// serialized; it only ensures that Save("") persists back to the selected
	// --config file instead of silently falling back to the default profile.
	path string

	AppleDeveloperToken string `json:"apple_developer_token"`
	AppleUserToken      string `json:"apple_user_token"`
	AppleKeyID          string `json:"apple_key_id"`
	AppleTeamID         string `json:"apple_team_id"`
	StoreFront          string `json:"storefront"`
	AuthPort            int    `json:"auth_port"`
	Provider            string `json:"provider"`
	Theme               string `json:"theme"`
	AudioQuality        string `json:"audio_quality,omitempty"`
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

	// Language is the interface language ("en", "ru"); "" = from the locale.
	Language string `json:"language,omitempty"`
	// Services lists the enabled service tabs ("yandex", "apple", "spotify",
	// "local") in order; empty means all.
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
	// MusicDir is the local directory to scan for audio files
	// Supports MP3, FLAC, M4A. Scanned recursively.
	MusicDir string `json:"music_dir,omitempty"`
	// DiscordRPC enables Discord Rich Presence (defaults to true).
	DiscordRPC *bool `json:"discord_rpc,omitempty"`
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
	data, err := json.MarshalIndent(c, "", "  ")
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
	return nil
}
