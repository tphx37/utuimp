# Configuration

[← docs](README.md)

- [utuimp setup](#utuimp-setup)
- [Config file](#config-file)
- [Themes](#themes)
- [Where things are kept](#where-things-are-kept)
- [Command line](#command-line)
- [Environment variables](#environment-variables)

## utuimp setup

Everything is set inside utuimp, in the settings panel: press `,` (or type
`:settings`). The first start opens it by itself. `utuimp setup` starts
utuimp with the panel open:

```bash
utuimp setup                  # the settings
utuimp setup visualizer       # on one section
```

Sections: `services`, `interface`, `playback`, `visualizer`, `graphics`,
`integrations`, `sound`, `lyrics`, `library`. `↑`/`↓` choose a line, `←`/`→` or `enter` change it,
`enter` on a line of text types into it, `backspace` empties it.

Most changes apply at once: turning a service on or off shows or hides its
tab while utuimp runs, and the language, theme, animation, the view, the
key hints, the album art, the frame rate, the look (section Graphics), the
seek and volume steps, song notifications, the lyrics' size, the
visualizer and Discord status follow immediately, in every tab (the
settings stay open when the view changes under them). A few need a restart (a service's own
keys, the Apple Music quality and token source); the panel says so and `R`
restarts utuimp in place.

## Config file

Everything else is in `~/.config/utuimp/config.json` (Windows:
`%APPDATA%\utuimp\config.json`). utuimp writes it as you use it (the view,
the volume, the visualizer scene), so edit it while utuimp is closed.

### Look

| Field | Values | Default |
|---|---|---|
| `theme` | `system` (your terminal's colours), `classic`, `dracula`, `gruvbox`, `nord`, `mono` (black and white), or [your own](#themes) | `system` |
| `layout` | `""` boxed view, `"cover"` cover view (`v` sets it) | `""` |
| `cover_hide_lyrics` | hide lyrics in the cover view (`y` sets it) | `false` |
| `album_art` | album art in the boxed view (`:art` sets it) | `false` |
| `hide_hints` | the key hints at the bottom hidden (`H` sets it) | `false` |
| `frame_rate` | `""` as many frames a second as fit the screen (60, or 72 on a 144 Hz one), `"60"`, `"30"` (to spare the battery) | `""` |
| `intro` | the start screen (the logo over dancing ribbons) | `true` |
| `ui_motion` | `"off"`: nothing fades in or moves, and the interface is in cells (its pixel lines and pictures move). `"calm"`, `"lively"`, `"wild"`: panels and new songs fade in softly (the three are the same now) | `"lively"` |
| `ui_style` | `""` (the interface's lines, frames and cover drawn in pixels under the text where the terminal can: kitty, Ghostty), `"simple"` (cells) | `""` |
| `ui_cover` | `""` (the cover as a picture), `"cells"` | `""` |
| `ui_lines` | `""` (frames, rules, progress and volume lines that sway with the music and answer keys), `"calm"` (the same, still), `"cells"` | `""` |
| `ui_text` | `""` (lyrics and the player's words in the system's font), `"cells"` | `""` |
| `ui_buttons` | `""` (the player's buttons drawn), `"cells"` | `""` |
| `ui_light` | `""` (a light behind the cover), `"off"` | `""` |
| `language` | `"en"`, `"ru"`. Empty means from the system locale | `""` |

### Services

| Field | What it is |
|---|---|
| `services` | the tabs, in order: `"yandex"`, `"apple"`, `"spotify"`, `"youtube"`, `"soundcloud"`, `"local"`. Empty means all |
| `service` | the tab shown on start |
| `service_keys` | keys for tabs, e.g. `{"1": "yandex", "`": "local"}`. A service bound here loses its default number |
| `music_dir` | legacy single local music folder (default `~/Music`); used when `music_dirs` is empty |
| `music_dirs` | array of local music folders; takes precedence over `music_dir` |
| `librespot_path` | librespot, if not on `PATH` |
| `ytdlp_path`, `ffmpeg_path` | yt-dlp and ffmpeg, if not on `PATH` |
| `youtube_cookies` | where yt-dlp takes YouTube cookies from: a browser name (`"firefox"`, `"chrome"`…) or the path of a `cookies.txt` |
| `spotify_client_id` | your own Spotify app, see [Services](services.md#spotify) |
| `soundcloud_client_id`, `soundcloud_client_secret` | your SoundCloud API app, see [Services](services.md#soundcloud) |
| `apple_token_source` | `"own"` for your own MusicKit developer token. See [Apple Music tokens](apple-music-tokens.md) |
| `storefront` | Apple Music country (e.g. `"us"`) |
| `audio_quality` | Apple Music bitrate (`:quality` sets it) |

### Library

| Field | What it allows | Default |
|---|---|---|
| `library_delete` | deleting playlists, taking songs out of them, taking likes back | `false` |
| `library_edit` | renaming playlists | `false` |

### Playback and lyrics

| Field | What it is |
|---|---|
| `volume` | last volume, 0.0-1.0 |
| `eq_bands` | the equalizer (`e` sets it) |
| `translate_lang` | the language lyrics are translated into, e.g. `"en"`. Empty means from the locale |
| `lyrics_size` | the cover view's lyrics drawn as a picture: `"small"`, `""` (medium), `"large"` |
| `seek_step` | how far `←` `→` move in the song, in seconds; 0 means 10 |
| `volume_step` | how far `+` `-` turn the volume, in percent; 0 means 5 |
| `notify` | a desktop notification with the song and its cover when the next one starts: `""` never, `"away"` while utuimp's window is not the one in use, `"always"`. Needs `notify-send` (Linux) or `osascript` (macOS) |
| `lyrics_sources` | lyrics source picked per song (`c` in lyrics sets it) |

### Visualizer

| Field | Values | Default |
|---|---|---|
| `visualizer_scene` | the last scene (`tab` sets it) | `tunnel` |
| `visualizer_strength` | 0-200 (percent) | 100 |
| `visualizer_brightness` | 10-150 (percent) | 100 |
| `visualizer_background` | `""`, `"stars"`, `"sparks"` | `""` |
| `graphics` | `""` (auto), `"on"`, `"off"`: draw the visualizer as a picture in kitty and Ghostty | `""` |
| `visualizer_detail` | `""`, `"fine"` | `""` |

### Other

| Field | What it is |
|---|---|
| `discord_rpc` | Discord status on/off (default `true`) |
| `discord_client_id` | your own Discord application |
| `lastfm_api_key`, `lastfm_api_secret` | Last.fm keys for builds without them |
| `wsl` | audio workarounds for WSL2 |

Sign-in tokens (`*_token`, `*_refresh_token`, `*_token_expiry`) are written
by utuimp. Don't edit them by hand. Use `--relogin` instead
([Services](services.md#switching-accounts-and-signing-out)).

### Example

```json
{
  "theme": "nord",
  "layout": "cover",
  "services": ["yandex", "spotify", "local"],
  "service_keys": { "`": "local" },
  "music_dirs": ["~/Music", "~/Other Music"],
  "translate_lang": "en",
  "visualizer_strength": 50,
  "visualizer_background": "stars",
  "intro": false
}
```

## Themes

`system` takes the colours of your terminal's palette, so utuimp matches
whatever scheme the terminal uses (pywal, matugen…). With it, utuimp asks
the terminal for its 16 colours at start so the fades blend real colours.

For a theme of your own, put a JSON file in
`~/.config/utuimp/themes/<name>.json` and set `"theme": "<name>"`. Colours
are `#RRGGBB` or `#RGB`. A field you leave out comes from `classic`.

```json
{
  "primary": "#C678DD",
  "secondary": "#98C379",
  "muted": "#5C6370",
  "error": "#E06C75",
  "fg": "#ABB2BF",
  "subtle": "#61AFEF",
  "bg": "",
  "accent": "#E5C07B",
  "love": "#E06C75",
  "progress": "#C678DD",
  "border": "",
  "border_focus": "#C678DD"
}
```

Other fields: `active`, `surface`, `accent_warm`, `mode_normal_bg`,
`mode_search_bg`, `mode_command_bg`, `mode_chip_fg`, `glow_palette` (eight
colours, dark to bright). An empty `border` means the terminal's default
foreground. If the file can't be read, utuimp falls back to `classic`.

## Where things are kept

| Path | What |
|---|---|
| `~/.config/utuimp/config.json` | settings |
| `~/.config/utuimp/credentials.json` | client IDs, secrets and sign-ins, copied here too. To remove a secret, clear it in `utuimp setup` |
| `~/.config/utuimp/library-youtube.json` | YouTube Music favorites and playlists |
| `~/.config/utuimp/library-soundcloud.json` | SoundCloud favorites and playlists (without the API) |
| `~/.cache/utuimp/` | recently played, the Apple Music developer token, update checks |

On Windows the config directory is `%APPDATA%\utuimp\`.

Never post `config.json` or `credentials.json` anywhere. They hold your
sign-ins.

## Command line

```
utuimp [flags]
utuimp setup [section]
utuimp auth login | logout | status
utuimp auth yandex [logout]
utuimp auth lastfm login | logout | status
utuimp version
```

| Flag | What it does |
|---|---|
| `--config <file>` | use another config file |
| `--relogin[=service]` | sign out and in again: `yandex`, `apple`, `spotify`, `soundcloud`, or all |
| `--local` | start on the local files tab |
| `--apple` | start on the Apple Music tab |
| `--music-dir <dir>` | the local music folder (saved to the config) |
| `--no-discord` | no Discord RP status this run |
| `--no-update` | don't check for updates this run |
| `--window` | open in a terminal window of its own |
| `--demo` | fake player and fake songs, no accounts or network |
| `--debug` | save diagnostics and crash reports to the user cache (`utuimp/debug.log`) |

## Environment variables

| Variable | Effect |
|---|---|
| `UTUIMP_SPOTIFY_OUTPUT=librespot` | librespot plays Spotify's sound itself instead of utuimp (Linux) |
| `UTUIMP_CONSOLE=0` / `=1` | turn the Linux console mode off / force it on |
| `UTUIMP_FPS=<n>` | frames a second, 10 to 240, instead of what fits the screen (see below) |
| `UTUIMP_LINES_LOG=<file>` | for a bug report about the pixel interface: a line a frame with the time between frames, how long one took to draw, whether the screen was laid out anew and the picture bytes sent to the terminal, and for a slow frame what took the time |
| `UTUIMP_FRAMES=<file>` | for a bug report about stutter: a line a frame of every animation (the lines, the visualizer, the start screen, the words' effects) and every screen laid out, with when it began, the time since the last and how long it took, and any key or message that took long |
| `UTUIMP_CPUPROFILE=<file>` | for a bug report about speed: where the processor's time went, for `go tool pprof`, written when utuimp quits |
| `UTUIMP_VIZ_INFO=<file>` | for a bug report about the visualizer: a line a second with what it hears, its detail and how long a frame takes |

utuimp draws its animations at a pace that fits the screen: a frame lasts
a whole number of the screen's refreshes, or what moves jerks. That is 60
frames a second on a 60, 120 or 240 Hz screen, 72 on a 144 Hz one, 75 on
75 Hz. It asks Hyprland, sway or X11 (xrandr) how often the screen
refreshes; elsewhere it draws 60 a second, and `UTUIMP_FPS` sets it.
