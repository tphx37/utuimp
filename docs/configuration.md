# Configuration

[← docs](README.md)

- [utuimp setup](#utuimp-setup)
- [Config file](#config-file)
- [Themes](#themes)
- [Where things are kept](#where-things-are-kept)
- [Command line](#command-line)
- [Environment variables](#environment-variables)

## utuimp setup

The first start asks everything. Later, `utuimp setup` shows a menu of
sections. You can also name one or several directly:

```bash
utuimp setup                  # menu
utuimp setup visualizer       # just one section
```

| Section | What it sets |
|---|---|
| `language` | interface language (English, Russian) |
| `discord` | the current song in your Discord status |
| `services` | which tabs to show, the music folder, your own Spotify / SoundCloud app, YouTube cookies |
| `visualizer` | strength and background |
| `interface` | fade-ins and the start screen |
| `library` | allow deleting and renaming in the library |

In the menu, type a section's number (`1 4` runs several), `a` for all of
them, or Enter to save and quit.

## Config file

Everything else is in `~/.config/utuimp/config.json` (Windows:
`%APPDATA%\utuimp\config.json`). utuimp writes it as you use it (the view,
the volume, the visualizer scene), so edit it while utuimp is closed.

### Look

| Field | Values | Default |
|---|---|---|
| `theme` | `system` (your terminal's colours), `classic`, `dracula`, `gruvbox`, `nord`, or [your own](#themes) | `system` |
| `layout` | `""` boxed view, `"cover"` cover view (`v` sets it) | `""` |
| `cover_hide_lyrics` | hide lyrics in the cover view (`y` sets it) | `false` |
| `album_art` | album art in the boxed view (`:art` sets it) | `false` |
| `intro` | the start screen (the logo over dancing ribbons) | `true` |
| `ui_motion` | `"off"`: nothing fades in. `"calm"`, `"lively"`, `"wild"`: panels and new songs fade in softly (the three are the same now) | `"lively"` |
| `language` | `"en"`, `"ru"`. Empty means from the system locale | `""` |

### Services

| Field | What it is |
|---|---|
| `services` | the tabs, in order: `"yandex"`, `"apple"`, `"spotify"`, `"youtube"`, `"soundcloud"`, `"local"`. Empty means all |
| `service` | the tab shown on start |
| `service_keys` | keys for tabs, e.g. `{"1": "yandex", "`": "local"}`. A service bound here loses its default number |
| `music_dir` | the local music folder (default `~/Music`) |
| `librespot_path` | librespot, if not on `PATH` |
| `ytdlp_path`, `ffmpeg_path` | yt-dlp and ffmpeg, if not on `PATH` |
| `youtube_cookies` | where yt-dlp takes YouTube cookies from: a browser name (`"firefox"`, `"chrome"`…) or the path of a `cookies.txt` |
| `spotify_client_id` | your own Spotify app, see [Services](services.md#spotify) |
| `soundcloud_client_id`, `soundcloud_client_secret` | your SoundCloud API app, see [Services](services.md#soundcloud) |
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
| `lyrics_sources` | lyrics source picked per song (`c` in lyrics sets it) |

### Visualizer

| Field | Values | Default |
|---|---|---|
| `visualizer_scene` | the last scene (`tab` sets it) | `tunnel` |
| `visualizer_strength` | 0-200 (percent) | 100 |
| `visualizer_brightness` | 10-150 (percent) | 100 |
| `visualizer_background` | `""`, `"stars"`, `"sparks"` | `""` |

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
  "music_dir": "~/Music",
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
utuimp setup [section…]
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
| `--debug` | more detailed logs |

## Environment variables

| Variable | Effect |
|---|---|
| `UTUIMP_SPOTIFY_OUTPUT=librespot` | librespot plays Spotify's sound itself instead of utuimp (Linux) |
| `UTUIMP_CONSOLE=0` / `=1` | turn the Linux console mode off / force it on |
