// Package settings describes what can be set in utuimp: one list that the
// settings panel draws and changes. Titles, labels and help are English and
// translated where they are shown.
package settings

import (
	"os/exec"
	"slices"
	"strconv"

	"github.com/tphx37/utuimp/internal/auth"
	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// Kind is how an item is changed.
type Kind int

const (
	Toggle Kind = iota // on or off
	Choice             // one of Options
	Text               // a line of text
	Action             // something to do (open a panel, sign in); Get says how things stand
)

// On and Off are the values of a Toggle.
const (
	On  = "on"
	Off = "off"
)

// Option is one value of a Choice.
type Option struct {
	Value string
	Label string // English; "" shows Value
}

// Item is one setting.
type Item struct {
	Key   string // stable name, e.g. "services.apple"
	Title string
	Help  string // shown under the list while the item is selected
	Kind  Kind
	// Options lists a Choice's values.
	Options func(*config.Config) []Option
	Get     func(*config.Config) string
	Set     func(*config.Config, string)
	// Blocked says why the item can't be changed right now ("" when it can).
	Blocked func(*config.Config) string
	// Hidden leaves the item out.
	Hidden func(*config.Config) bool
	// Restart: the change takes effect after utuimp restarts.
	Restart bool
	// Secret values are shown as dots.
	Secret bool
}

// Section is a titled group of items.
type Section struct {
	Key   string
	Title string
	Items []Item
}

// ServiceKeys are the services in tab order.
var ServiceKeys = []string{"yandex", "apple", "spotify", "youtube", "soundcloud", "local"} //nolint:gochecknoglobals // a constant list

var serviceNames = map[string]string{ //nolint:gochecknoglobals // a constant table
	"yandex": "Yandex Music", "apple": "Apple Music", "spotify": "Spotify (Premium)",
	"youtube": "YouTube Music", "soundcloud": "SoundCloud", "local": "Local files",
}

// AppleMusicNotice explains the credentials needed for the online catalog.
const AppleMusicNotice = "Apple Music requires your own MusicKit developer token in apple_developer_token. Local files, including the Music media folder on macOS, work without a token."

func enabled(cfg *config.Config, key string) bool {
	return len(cfg.Services) == 0 || slices.Contains(cfg.Services, key)
}

func setEnabled(cfg *config.Config, key string, on bool) {
	var out []string
	for _, k := range ServiceKeys {
		if k == key && on || k != key && enabled(cfg, k) {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		out = []string{"local"} // never no tabs at all
	}
	cfg.Services = out
}

// animationOff says why the pixel interface can't be had: with the
// animation off it is plain cells.
func animationOff(c *config.Config) string {
	if c.Motion() == config.MotionOff {
		return "the animation is off (Interface, Animation)"
	}
	return ""
}

// part is a Choice for a part of the pixel interface (see config.UICover):
// field is where it is kept, "" its first option. It can't be changed with
// the pixel interface off, nor while blocked says why.
func part(key, title, help string, opts []Option, field func(*config.Config) *string, blocked func(*config.Config) string) Item {
	return Item{
		Key: key, Title: title, Help: help, Kind: Choice,
		Options: func(*config.Config) []Option { return opts },
		Get: func(c *config.Config) string {
			v := *field(c)
			for _, o := range opts {
				if o.Value == v {
					return v
				}
			}
			return "" // set by hand to something else: the default
		},
		Set: func(c *config.Config, v string) { *field(c) = v },
		Blocked: func(c *config.Config) string {
			if why := animationOff(c); why != "" {
				return why
			}
			if c.UIStyle == config.StyleSimple {
				return "the pixel interface is off"
			}
			if blocked != nil {
				return blocked(c)
			}
			return ""
		},
	}
}

// choice is a Choice among opts, kept by get and set; a value set by hand
// to something not among them shows as the first.
func choice(key, title, help string, opts []Option, get func(*config.Config) string, set func(*config.Config, string)) Item {
	return Item{
		Key: key, Title: title, Help: help, Kind: Choice,
		Options: func(*config.Config) []Option { return opts },
		Get: func(c *config.Config) string {
			v := get(c)
			for _, o := range opts {
				if o.Value == v {
					return v
				}
			}
			return opts[0].Value
		},
		Set: set,
	}
}

// numberChoice is a Choice among numbers (a step in seconds or percent): 0
// in the config is the default, def.
func numberChoice(key, title, help, unit string, vals []int, def int, field func(*config.Config) *int) Item {
	opts := make([]Option, len(vals))
	for i, v := range vals {
		opts[i] = Option{Value: strconv.Itoa(v), Label: strconv.Itoa(v) + unit}
	}
	return choice(key, title, help, opts,
		func(c *config.Config) string {
			if *field(c) <= 0 {
				return strconv.Itoa(def)
			}
			return strconv.Itoa(*field(c))
		},
		func(c *config.Config, v string) {
			n, err := strconv.Atoi(v)
			if err != nil {
				return
			}
			if n == def {
				n = 0 // the default: not written to the config
			}
			*field(c) = n
		})
}

// NotifierAvailable reports whether the desktop can be told of a new song
// here: notify-send on Linux and the BSDs, osascript on macOS.
var NotifierAvailable = func() bool { //nolint:gochecknoglobals // swapped in tests
	for _, name := range []string{"notify-send", "osascript"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

func onOff(b bool) string {
	if b {
		return On
	}
	return Off
}

func serviceItem(key string) Item {
	it := Item{
		Key: "services." + key, Title: serviceNames[key], Kind: Toggle,
		Get: func(c *config.Config) string { return onOff(enabled(c, key)) },
		Set: func(c *config.Config, v string) { setEnabled(c, key, v == On) },
	}
	if key == "apple" {
		it.Blocked = func(c *config.Config) string {
			if !enabled(c, key) && c.AppleTokenSource == "" && !auth.BuiltIn() {
				return "choose whose developer token to use first (the next line)"
			}
			return ""
		}
	}
	return it
}

func text(key, title, help string, field func(*config.Config) *string) Item {
	return Item{
		Key: key, Title: title, Help: help, Kind: Text, Restart: true,
		Get: func(c *config.Config) string { return *field(c) },
		Set: func(c *config.Config, v string) { *field(c) = v },
	}
}

// Sections returns everything that can be set, in the order it is shown.
func Sections() []Section {
	services := []Item{serviceItem("yandex"), serviceItem("apple"), {
		Key: "services.apple_token", Title: "Apple Music developer token", Kind: Choice, Restart: true,
		Help:   AppleMusicNotice,
		Hidden: func(*config.Config) bool { return auth.BuiltIn() },
		Options: func(*config.Config) []Option {
			return []Option{
				{Value: "", Label: "not chosen"},
				{Value: config.AppleTokenOwn, Label: "own (my MusicKit key)"},
			}
		},
		Get: func(c *config.Config) string { return c.AppleTokenSource },
		Set: func(c *config.Config, v string) {
			c.AppleTokenSource = v
			if v == "" {
				setEnabled(c, "apple", false)
			}
		},
	}, serviceItem("spotify")}
	spotifyID := text("services.spotify_client_id", "Spotify Client ID",
		"Optional. Spotify sometimes blocks the shared sign-in; your own free app at developer.spotify.com/dashboard avoids that. Empty = shared sign-in.",
		func(c *config.Config) *string { return &c.SpotifyClientID })
	setID := spotifyID.Set
	spotifyID.Set = func(c *config.Config, v string) {
		if v != c.SpotifyClientID {
			// Another app: the old sign-in doesn't apply.
			c.SpotifyToken, c.SpotifyRefreshToken, c.SpotifyTokenExpiry = "", "", 0
		}
		setID(c, v)
	}
	scReset := func(c *config.Config) {
		c.SoundCloudToken, c.SoundCloudRefreshToken, c.SoundCloudTokenExpiry = "", "", 0
	}
	scID := text("services.soundcloud_client_id", "SoundCloud Client ID",
		"Optional: your own SoundCloud API app (needs Artist Pro) for your likes and playlists. Empty = plays through yt-dlp, no account.",
		func(c *config.Config) *string { return &c.SoundCloudClientID })
	scSecret := text("services.soundcloud_client_secret", "SoundCloud Client Secret", scID.Help,
		func(c *config.Config) *string { return &c.SoundCloudClientSecret })
	scSecret.Secret = true
	for _, it := range []*Item{&scID, &scSecret} {
		set := it.Set
		get := it.Get
		it.Set = func(c *config.Config, v string) {
			if v != get(c) {
				scReset(c)
			}
			set(c, v)
		}
	}
	services = append(services, spotifyID, serviceItem("youtube"),
		text("services.youtube_cookies", "YouTube cookies",
			"If YouTube asks to confirm you're not a bot: a browser to take cookies from (firefox, chrome…) or a cookies.txt path. Empty = none.",
			func(c *config.Config) *string { return &c.YouTubeCookies }),
		serviceItem("soundcloud"), scID, scSecret, serviceItem("local"),
		text("services.music_dir", "Music folder", "Where local files are. Empty = ~/Music.",
			func(c *config.Config) *string { return &c.MusicDir }))

	return []Section{
		{Key: "services", Title: "Services", Items: services},
		{Key: "interface", Title: "Interface", Items: []Item{
			{
				Key: "interface.language", Title: "Language", Kind: Choice,
				Options: func(*config.Config) []Option {
					out := make([]Option, len(i18n.Languages))
					for i, l := range i18n.Languages {
						out[i] = Option{Value: l[0], Label: l[1]}
					}
					return out
				},
				Get: func(c *config.Config) string {
					if c.Language == "" {
						return i18n.Detect()
					}
					return c.Language
				},
				Set: func(c *config.Config, v string) { c.Language = v },
			},
			{
				Key: "interface.theme", Title: "Theme", Kind: Choice,
				Options: func(c *config.Config) []Option {
					names := styles.BuiltinThemeNames()
					if c.Theme != "" && !slices.Contains(names, c.Theme) {
						names = append(names, c.Theme) // a theme file of the user's
					}
					out := make([]Option, len(names))
					for i, n := range names {
						out[i] = Option{Value: n}
						if n == "mono" {
							out[i].Label = "black and white"
						}
					}
					return out
				},
				Get: func(c *config.Config) string { return c.Theme },
				Set: func(c *config.Config, v string) { c.Theme = v },
			},
			{
				Key: "interface.amoled", Title: "AMOLED background", Kind: Toggle,
				Help: "Use a pure black background behind the interface.",
				Get:  func(c *config.Config) string { return onOff(c.AMOLED) },
				Set:  func(c *config.Config, v string) { c.AMOLED = v == On },
			},
			{
				Key: "interface.motion", Title: "Animation", Kind: Choice,
				Help: "off: still, and the interface in plain cells (its pixel lines and pictures move, so they go too; for lines that stand still see Graphics, Frames and lines). calm: panels and new songs fade in.",
				Options: func(*config.Config) []Option {
					return []Option{{Value: config.MotionOff, Label: "off"}, {Value: config.MotionCalm, Label: "calm"}}
				},
				Get: func(c *config.Config) string {
					if c.Motion() == config.MotionOff {
						return config.MotionOff
					}
					return config.MotionCalm
				},
				Set: func(c *config.Config, v string) { c.UIMotion = v },
			},
			{
				Key: "interface.intro", Title: "Start screen", Kind: Toggle,
				Help: "The logo over dancing ribbons when utuimp starts.",
				Get:  func(c *config.Config) string { return onOff(c.IntroEnabled()) },
				Set: func(c *config.Config, v string) {
					if v == On {
						c.Intro = nil
						return
					}
					off := false
					c.Intro = &off
				},
			},
			choice("interface.view", "View",
				"boxed: panels side by side. cover: the album art large, with the lyrics beside it. v switches too.",
				[]Option{{Value: "", Label: "boxed"}, {Value: "cover", Label: "with the cover"}},
				func(c *config.Config) string { return c.Layout },
				func(c *config.Config, v string) { c.Layout = v }),
			{
				Key: "interface.cover_lyrics", Title: "Lyrics in the cover view", Kind: Toggle,
				Help: "Off: the cover view is the cover and the player alone. y switches too.",
				Get:  func(c *config.Config) string { return onOff(!c.CoverHideLyrics) },
				Set:  func(c *config.Config, v string) { c.CoverHideLyrics = v != On },
			},
			{
				Key: "interface.album_art", Title: "Album art in the boxed view", Kind: Toggle,
				Help: "The cover in the Now Playing box. :art switches too.",
				Get:  func(c *config.Config) string { return onOff(c.AlbumArt) },
				Set:  func(c *config.Config, v string) { c.AlbumArt = v == On },
			},
			{
				Key: "interface.hints", Title: "Key hints", Kind: Toggle,
				Help: "The keys at the bottom of the screen. H switches too.",
				Get:  func(c *config.Config) string { return onOff(!c.HideHints) },
				Set:  func(c *config.Config, v string) { c.HideHints = v != On },
			},
			choice("interface.frame_rate", "Frame rate",
				"auto: as many frames a second as fit your screen's refresh (60, or 72 on a 144 Hz screen). 30: half the work, for the battery; the animations step a little.",
				[]Option{{Value: "", Label: "auto"}, {Value: "60"}, {Value: "30", Label: "30 (battery)"}},
				func(c *config.Config) string { return c.FrameRate },
				func(c *config.Config, v string) { c.FrameRate = v }),
		}},
		{Key: "playback", Title: "Playback", Items: []Item{
			numberChoice("playback.seek_step", "Seek step",
				"How far ← and → move in the song.", " s", []int{5, 10, 15, 30}, 10,
				func(c *config.Config) *int { return &c.SeekStep }),
			numberChoice("playback.volume_step", "Volume step",
				"How far + and - turn the volume.", "%", []int{2, 5, 10}, 5,
				func(c *config.Config) *int { return &c.VolumeStep }),
			{
				Key: "playback.notify", Title: "Song notifications", Kind: Choice,
				Help:   "A desktop notification with the song and its cover when the next one starts. away: only while you are in another window.",
				Hidden: func(*config.Config) bool { return !NotifierAvailable() },
				Options: func(*config.Config) []Option {
					return []Option{{Value: "", Label: "off"}, {Value: config.NotifyAway, Label: "away"}, {Value: config.NotifyAlways, Label: "always"}}
				},
				Get: func(c *config.Config) string { return c.Notify },
				Set: func(c *config.Config, v string) { c.Notify = v },
			},
		}},
		{Key: "visualizer", Title: "Visualizer", Items: []Item{
			{
				Key: "visualizer.strength", Title: "Strength", Kind: Choice,
				Help: "How hard the picture moves, zooms and shakes. Less of it helps if the visualizer makes you dizzy.",
				Options: func(c *config.Config) []Option {
					vals := []string{"25", "50", "75", "100", "150"}
					if cur := strconv.Itoa(int(c.VizStrength()*100 + 0.5)); !slices.Contains(vals, cur) {
						vals = append(vals, cur) // set by hand in the config
					}
					out := make([]Option, len(vals))
					for i, v := range vals {
						out[i] = Option{Value: v, Label: v + "%"}
					}
					return out
				},
				Get: func(c *config.Config) string { return strconv.Itoa(int(c.VizStrength()*100 + 0.5)) },
				Set: func(c *config.Config, v string) {
					n, err := strconv.Atoi(v)
					if err != nil {
						return
					}
					if n == 100 {
						c.VisualizerStrength = nil
						return
					}
					c.VisualizerStrength = &n
				},
			},
			{
				Key: "visualizer.background", Title: "Background", Kind: Choice,
				Help: "Behind every scene. stars: tiny squares standing still. sparks: tiny squares drifting up, as behind the ribbons.",
				Options: func(*config.Config) []Option {
					return []Option{{Value: "", Label: "none"}, {Value: "stars"}, {Value: "sparks"}}
				},
				Get: func(c *config.Config) string { return c.VisualizerBackground },
				Set: func(c *config.Config, v string) { c.VisualizerBackground = v },
			},
			choice("visualizer.scene", "Scene",
				"The scene it opens with. tab switches while it is open.",
				[]Option{{Value: "", Label: "tunnel"}, {Value: "terrain"}, {Value: "nebula"}, {Value: "spectrum"}, {Value: "orb"}, {Value: "ribbons"}},
				func(c *config.Config) string {
					if c.VisualizerScene == "tunnel" {
						return ""
					}
					return c.VisualizerScene
				},
				func(c *config.Config, v string) { c.VisualizerScene = v }),
			{
				Key: "visualizer.brightness", Title: "Brightness", Kind: Choice,
				Help: "How bright the picture is. { and } change it while it is open.",
				Options: func(c *config.Config) []Option {
					vals := []string{"50", "70", "100", "120", "150"}
					if cur := strconv.Itoa(int(c.VizBrightness()*100 + 0.5)); !slices.Contains(vals, cur) {
						vals = append(vals, cur) // set with { and }, or by hand
						slices.SortFunc(vals, func(a, b string) int { x, _ := strconv.Atoi(a); y, _ := strconv.Atoi(b); return x - y })
					}
					out := make([]Option, len(vals))
					for i, v := range vals {
						out[i] = Option{Value: v, Label: v + "%"}
					}
					return out
				},
				Get: func(c *config.Config) string { return strconv.Itoa(int(c.VizBrightness()*100 + 0.5)) },
				Set: func(c *config.Config, v string) {
					n, err := strconv.Atoi(v)
					if err != nil {
						return
					}
					if n == 100 {
						c.VisualizerBrightness = nil
						return
					}
					c.VisualizerBrightness = &n
				},
			},
		}},
		{Key: "graphics", Title: "Graphics", Items: []Item{
			{
				Key: "graphics.style", Title: "Pixel interface", Kind: Choice,
				Help: "on: in kitty and Ghostty the interface is drawn in real pixels under the text, in the song's colours (each part below can be left in characters). off: plain cells, as in every other terminal.",
				Options: func(*config.Config) []Option {
					return []Option{{Value: "", Label: "on"}, {Value: config.StyleSimple, Label: "off"}}
				},
				Get: func(c *config.Config) string {
					if c.UIStyle == config.StyleSimple {
						return config.StyleSimple
					}
					return ""
				},
				Set:     func(c *config.Config, v string) { c.UIStyle = v },
				Blocked: animationOff,
			},
			part("graphics.cover", "Cover", "picture: the cover itself, with round corners. characters: drawn with coloured cells.",
				[]Option{{Value: "", Label: "picture"}, {Value: config.PartCells, Label: "characters"}},
				func(c *config.Config) *string { return &c.UICover }, nil),
			part("graphics.lines", "Frames and lines", "moving: fine lines that sway with the music, flash on the drum and answer keys. still: the same lines, standing still. characters: the frame characters of the terminal.",
				[]Option{{Value: "", Label: "moving"}, {Value: config.LinesCalm, Label: "still"}, {Value: config.PartCells, Label: "characters"}},
				func(c *config.Config) *string { return &c.UILines }, nil),
			part("graphics.text", "Text", "drawn: the lyrics large and the player's words (title, artist, times, volume) in the system's font. terminal: in the terminal's own letters.",
				[]Option{{Value: "", Label: "drawn"}, {Value: config.PartCells, Label: "terminal"}},
				func(c *config.Config) *string { return &c.UIText }, nil),
			part("graphics.buttons", "Buttons", "strokes: repeat, shuffle, play and favourite in fine strokes that spring when pressed. characters: ↺ ⇄ ▶ ♡.",
				[]Option{{Value: "", Label: "strokes"}, {Value: config.PartCells, Label: "characters"}},
				func(c *config.Config) *string { return &c.UIButtons }, nil),
			part("graphics.light", "Light behind the cover", "A glow in the cover's colours round it, breathing with the bass.",
				[]Option{{Value: "", Label: "on"}, {Value: config.LightOff, Label: "off"}},
				func(c *config.Config) *string { return &c.UILight },
				func(c *config.Config) string {
					if c.UICover == config.PartCells {
						return "the cover is in characters"
					}
					return ""
				}),
			{
				Key: "graphics.pixels", Title: "Visualizer as a picture", Kind: Choice,
				Help: "In kitty and Ghostty the visualizer is drawn as a picture instead of coloured cells. auto: where the terminal is known to show pictures. on: always try. off: cells only. Takes effect the next time the visualizer opens.",
				Options: func(*config.Config) []Option {
					return []Option{{Value: "", Label: "auto"}, {Value: "on"}, {Value: "off"}}
				},
				Get: func(c *config.Config) string {
					if c.Graphics == "on" || c.Graphics == "off" {
						return c.Graphics
					}
					return ""
				},
				Set: func(c *config.Config, v string) { c.Graphics = v },
			},
			{
				Key: "graphics.detail", Title: "Visualizer detail", Kind: Choice,
				Help: "normal: smooth and light on the processor. fine: several times sharper, and several processor cores while the visualizer is open.",
				Options: func(*config.Config) []Option {
					return []Option{{Value: "", Label: "normal"}, {Value: "fine"}}
				},
				Get: func(c *config.Config) string {
					if c.VisualizerDetail == "fine" {
						return "fine"
					}
					return ""
				},
				Set: func(c *config.Config, v string) { c.VisualizerDetail = v },
			},
		}},
		{Key: "integrations", Title: "Integrations", Items: []Item{
			{
				Key: "integrations.discord", Title: "Discord status", Kind: Toggle,
				Help: "Show what you're playing in your Discord status.",
				Get:  func(c *config.Config) string { return onOff(c.DiscordRPCEnabled()) },
				Set:  func(c *config.Config, v string) { on := v == On; c.DiscordRPC = &on },
			},
			{
				Key: "integrations.lastfm", Title: "Last.fm scrobbling", Kind: Action,
				Help: "Enter connects your Last.fm account (a browser page opens; allow access there, then press Enter here again). Enter on a connected account disconnects it.",
				Get: func(c *config.Config) string {
					if c.LastfmSessionKey != "" {
						return "connected"
					}
					return "not connected"
				},
			},
		}},
		{Key: "sound", Title: "Sound", Items: []Item{
			{
				Key: "sound.quality", Title: "Apple Music quality", Kind: Choice, Restart: true,
				Options: func(*config.Config) []Option {
					return []Option{{Value: "high", Label: "high (256 kbps)"}, {Value: "standard", Label: "standard (64 kbps)"}}
				},
				Get: func(c *config.Config) string {
					if k, err := c.AudioBitrateKbps(); err == nil && k < 128 {
						return "standard"
					}
					return "high"
				},
				Set: func(c *config.Config, v string) { c.AudioQuality = v },
			},
			{
				Key: "sound.equalizer", Title: "Equalizer", Kind: Action,
				Get: func(*config.Config) string { return "open" },
			},
		}},
		{Key: "lyrics", Title: "Lyrics", Items: []Item{
			{
				Key: "lyrics.translate", Title: "Translate into", Kind: Text,
				Help: "The language lyrics are translated into, as a code: en, ru, de… Empty = the interface language.",
				Get:  func(c *config.Config) string { return c.TranslateLang },
				Set:  func(c *config.Config, v string) { c.TranslateLang = v },
			},
			choice("lyrics.size", "Size",
				"The cover view's lyrics, where they are drawn as a picture.",
				[]Option{{Value: config.LyricsSmall, Label: "small"}, {Value: "", Label: "medium"}, {Value: config.LyricsLarge, Label: "large"}},
				func(c *config.Config) string { return c.LyricsSize },
				func(c *config.Config, v string) { c.LyricsSize = v }),
		}},
		{Key: "library", Title: "Library", Items: []Item{
			{
				Key: "library.delete", Title: "Deleting", Kind: Toggle,
				Help: "Allow deleting from the library: playlists, songs from playlists, and likes taken back.",
				Get:  func(c *config.Config) string { return onOff(c.LibraryDelete) },
				Set:  func(c *config.Config, v string) { c.LibraryDelete = v == On },
			},
			{
				Key: "library.edit", Title: "Editing", Kind: Toggle,
				Help: "Allow renaming playlists. Adding is always allowed.",
				Get:  func(c *config.Config) string { return onOff(c.LibraryEdit) },
				Set:  func(c *config.Config, v string) { c.LibraryEdit = v == On },
			},
		}},
		{Key: "about", Title: "About", Items: []Item{
			{
				Key: "about.open", Title: "About utuimp", Kind: Action,
				Get: func(*config.Config) string { return "open" },
			},
		}},
	}
}
