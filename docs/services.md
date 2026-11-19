# Services

[← docs](README.md)

- [Yandex Music](#yandex-music)
- [Apple Music](#apple-music)
- [Spotify](#spotify)
- [YouTube Music](#youtube-music)
- [SoundCloud](#soundcloud)
- [Internet radio](#internet-radio)
- [Local files](#local-files)
- [Switching accounts and signing out](#switching-accounts-and-signing-out)
- [Last.fm](#lastfm)
- [Discord status](#discord-status)
- [System media controls](#system-media-controls)

Choose which services you use in `utuimp setup services`. Each one is a
tab. The first time you open it, it signs in. Sign-ins are saved, so this
happens once.

| Service | Account | Subscription | Extra programs |
|---|---|---|---|
| Yandex Music | Yandex | Yandex Plus | none |
| Apple Music | Apple ID | Apple Music | your own [MusicKit token](apple-music-tokens.md) |
| Spotify | Spotify | Premium | [librespot](https://github.com/librespot-org/librespot) |
| YouTube Music | none | none | [yt-dlp](https://github.com/yt-dlp/yt-dlp), [FFmpeg](https://ffmpeg.org) |
| SoundCloud | none / API app | none / Artist Pro with API | [yt-dlp](https://github.com/yt-dlp/yt-dlp), [FFmpeg](https://ffmpeg.org) |
| Internet radio | none | none | none |
| Local files | none | none | none |

## Yandex Music

The first time, utuimp shows a link and a code. Open the link, enter the
code, and you are signed in. The same from the command line:

```bash
utuimp auth yandex          # sign in
utuimp auth yandex logout   # forget the sign-in
```

`F` opens the feed with **My Wave**, and `R` is My Wave by the current song,
see [Radio and My Wave](usage.md#radio-and-my-wave). Lyrics come from Yandex
Music first when it has them.

## Apple Music

The first time, sign in in the browser window that opens.

The online catalog requires your own MusicKit developer token. Configure
`apple_developer_token` and select `own` in `utuimp setup services`.
See [Apple Music tokens](apple-music-tokens.md). On macOS, the local Music
media folder works through Local files without any token:
`utuimp --local --music-dir "$HOME/Music/Music/Media"`.

`:quality high|standard|256|64` sets the bitrate, `:quality` alone shows
the current format. `utuimp --apple` starts on the Apple Music tab.

## Spotify

Needs **Spotify Premium** and a separately installed
[librespot](https://github.com/librespot-org/librespot), which plays the
sound. utuimp's builds do not include librespot, and the installer may
offer your package manager's version. The first time, approve access on the
Spotify page that opens. The Spotify app doesn't need to be open.

On Linux utuimp takes the sound from librespot and plays it through its own
player, so pause is instant and the [visualizer](visualizer.md) sees the
real sound. `UTUIMP_SPOTIFY_OUTPUT=librespot` lets librespot play it itself
instead.

Without librespot utuimp can still control Spotify playing on your other
devices.

If librespot isn't on your `PATH`, set `librespot_path` in the
[config](configuration.md#config-file).
librespot 0.8.0 can reject track and playlist playback with HTTP 403.
Until an upstream release contains the fix, see the tested build and
workaround in [Spotify troubleshooting](troubleshooting.md#spotify).

**Your own Spotify app.** Spotify sometimes limits the app utuimp signs in
with by default. If it says it is blocking the shared sign-in, create your
own app (free):

1. open the [Spotify dashboard](https://developer.spotify.com/dashboard)
2. create an app with redirect URI `http://127.0.0.1:5588/login` and
   **Web API** ticked
3. run `utuimp setup` and paste its Client ID.

## YouTube Music

Needs [yt-dlp](https://github.com/yt-dlp/yt-dlp) and
[FFmpeg](https://ffmpeg.org/download.html). No sign-in: favorites (`f`) and
playlists are kept by utuimp in `~/.config/utuimp/library-youtube.json`, and
the feed (`F`) has a mix for each favorite.

Search reads songs, artists and albums from YouTube Music, including available
artist names, album names and durations. Playback still uses yt-dlp and
FFmpeg. Exported cookies can also show account likes and playlists in the
library; account items are read-only, while utuimp's favorites and playlists
are saved locally.

If YouTube asks to confirm you're not a bot, give yt-dlp your browser's
cookies, see [Troubleshooting](troubleshooting.md#youtube-music-confirm-youre-not-a-bot).

If yt-dlp or ffmpeg aren't on your `PATH`, set `ytdlp_path` / `ffmpeg_path`.

## SoundCloud

By default it plays through yt-dlp and FFmpeg, like YouTube Music. No
account and no subscription. Favorites and playlists are kept by utuimp in
`~/.config/utuimp/library-soundcloud.json`.

Tracks that SoundCloud only offers as 30-second previews (SoundCloud Go+)
are left out. Search takes several seconds, since yt-dlp asks SoundCloud
page by page.

**The official API.** If you have SoundCloud Artist Pro, you can use the
SoundCloud API instead, with your own likes and playlists:

1. open [your apps](https://soundcloud.com/you/apps)
2. create an app with redirect URI `http://127.0.0.1:5590/soundcloud`
3. open the settings (`,`) and paste its
   Client ID and Client Secret.

## Internet radio

The Radio tab searches Radio Browser by station name. Paste a direct stream
or supported station page into `/` or use `:link <url>`. Press `f` while a
station plays, or `Ctrl+F` on a search result, to save it.
The library's Songs and Favorites contain saved stations: select one and press
`r` to rename it without changing its stream URL. In the feed, `f` saves the
selected recent link. `F` shows recently played links. An HTML page must expose a direct stream in its player or metadata; Apple Music
station pages do not currently work here. The feed keeps recent played links
across restarts. On Linux, the player
updates the displayed title, artist and album when the stream supplies them;
not every station sends those fields.

## Local files

Plays the music in your `Music` folder, scanned with all its subfolders.
Set one or more folders in Settings → Services → Music folders, separated
by semicolons. Changing them triggers a rescan. `music_dirs` in the config
also accepts an array; the older `music_dir` remains supported. Run
`:refresh` from the Local files tab or press `Ctrl+R` in its library after
adding files. Playback and the current queue continue during the rescan.

To replace the folder list with one folder from the command line (saved):

```bash
utuimp --music-dir ~/some/folder
utuimp --local          # start on the local files tab
```

Lyrics: an `.lrc` file with the song's name beside it (`song.mp3` →
`song.lrc`) is shown in time with the song, a `.txt` as plain text. They come
before the lyrics sites.

Formats:

| System | Formats |
|---|---|
| Linux | MP3, FLAC, M4A, OGG, WAV, AIFF |
| macOS | MP3, FLAC, M4A, WAV, AIFF |
| Windows | MP3, FLAC, M4A |

On Linux FLAC, M4A and WAV need
[gst-plugins-good](https://gstreamer.freedesktop.org/documentation/installing/on-linux.html).

## Switching accounts and signing out

To sign out and sign in again, with the same or another account:

```bash
utuimp --relogin=yandex
utuimp --relogin=apple
utuimp --relogin=spotify
utuimp --relogin=soundcloud
utuimp --relogin            # all of them
```

`utuimp auth status` shows what you are signed in to, `utuimp auth logout`
clears the saved tokens.

## Last.fm

utuimp can scrobble what you play to Last.fm:

```bash
utuimp auth lastfm login    # connect your account
utuimp auth lastfm status
utuimp auth lastfm logout
```

Release builds come with Last.fm API keys. When you build from source
without them, set `lastfm_api_key` and `lastfm_api_secret` in the config
(get them at [last.fm/api](https://www.last.fm/api/account/create)).

## Discord status

utuimp shows the current song in your Discord status. Turn it off in
`utuimp setup discord`, with `"discord_rpc": false` in the config, or for
one run with `--no-discord`.

## System media controls

utuimp shows up in the system's media controls: MPRIS on Linux (media
keys, the desktop's player widget, `playerctl`), and the media overlay on
Windows. It always controls the service that is playing.
