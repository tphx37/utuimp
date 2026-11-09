<p align="center">
  <img src="assets/utuimp_logo.png" alt="utuimp" width="128">
</p>

<h1 align="center">utuimp</h1>

<p align="center">universal terminal user interface media player</p>

A music player for the terminal. It plays Yandex Music, Apple Music, Spotify,
YouTube Music, SoundCloud and the music files on your computer, and
everything is done from the keyboard.

## Install

You need [Go](https://go.dev/dl/) and, on Linux, GStreamer.

```bash
git clone https://github.com/tphx37/utuimp
cd utuimp
make build
```

Now run `utuimp` in any terminal.

On first start utuimp asks for the language, whether to show the current
song in your Discord status, and which services you use. To change these
later, run:

```bash
utuimp setup
```

## Services

Each service has its own tab. Switch with the number keys: `1`, `2`, `3`…
The tab bar at the top shows which number is which.

- **Yandex Music.** The first time, utuimp shows a link and a code. Open the
  link, enter the code, and you are signed in.
- **Apple Music.** The first time, sign in in the browser window that opens.
  How utuimp gets the tokens Apple asks for is described in
  [Apple Music tokens](#apple-music-tokens).
- **Spotify.** Needs Spotify Premium and
  [librespot](https://github.com/librespot-org/librespot), which plays the
  sound (Arch: `sudo pacman -S librespot`). The first time, approve access on
  the Spotify page that opens. The Spotify app doesn't need to be open.
- **YouTube Music.** Needs [yt-dlp](https://github.com/yt-dlp/yt-dlp) and
  ffmpeg (Arch: `sudo pacman -S yt-dlp ffmpeg`). No sign-in: favorites (`f`)
  and playlists are kept by utuimp in `~/.config/utuimp/library-youtube.json`,
  and the feed has a mix for each favorite. If YouTube asks to confirm you're
  not a bot, see [Problems](#problems).
- **SoundCloud.** By default it plays through yt-dlp and ffmpeg, like
  YouTube Music: no account and no subscription. Favorites and playlists
  stay in utuimp (`~/.config/utuimp/library-soundcloud.json`), and the feed
  shows tracks like each favorite. Tracks that SoundCloud only offers as
  30-second previews (SoundCloud Go+) are left out. Search takes several
  seconds, since yt-dlp asks SoundCloud page by page.

  If you have SoundCloud Artist Pro, you can use the official API instead,
  with your own likes and playlists: open
  [your apps](https://soundcloud.com/you/apps), create an app with redirect URI
  `http://127.0.0.1:5590/soundcloud`, run `utuimp setup`, answer yes to the
  API app question and paste its Client ID and Client Secret.
- **Local files.** Plays the music in your `Music` folder. You can pick another
  folder in `utuimp setup`.

To sign in with a different account:

```bash
utuimp --relogin=yandex
utuimp --relogin=apple
utuimp --relogin=spotify
utuimp --relogin=soundcloud
```

## Keys

| Key | Action |
|-----|--------|
| `space` | play / pause |
| `n` / `p` | next / previous song |
| `←` / `→` | rewind / forward 10 seconds |
| `+` / `-` | volume |
| `f` | add to favorites (press twice to remove) |
| `s` | shuffle |
| `r` | repeat |
| `R` | radio from the current song (Yandex Music: My Wave by this song) |
| `v` | switch between the boxed view and the cover view |
| `/` | search |
| `l` | your library |
| `q` | recently played |
| `y` | lyrics |
| `F` | recommendations (Yandex Music: My Wave) |
| `e` | equalizer |
| `i` | song menu: go to the album or an artist, copy the link |
| `?` | about |
| `esc` | back |

**Volume** is one for all services: `+`/`-`, `:vol` or `:mute` in any tab
sets every player.

**Radio and My Wave** keep at least four songs queued after the current one,
so you always see what plays next.

**Cover view.** `v` switches to a view without frames, like a fullscreen
player: the cover with the song, controls and volume under it, and the
lyrics beside it. The lyrics fill the
height of the cover; the line being sung sits near the top, the lines already
sung fade away above it, and the text moves up with the song. Search (`/`), the
queue (`esc` on the lyrics), the feed, the library and every other panel open
in a frame in place of the lyrics; closing them brings the lyrics back. While
nothing plays, a frame holds the cover's place. `v` again returns to the
boxed view. utuimp remembers which one you used.

**Queue.** Pick a song with the arrow keys. `enter` plays it, `d` removes it,
`K` and `J` move it up and down.

**Lyrics.** Word-by-word if available or line-by-line. `t` shows a translation next
to each line, `T` chooses the language, `c` chooses where the lyrics come
from if the text is wrong.

**Commands.** Type `:` and then a command, for example `:vol 50`, `:mute`,
`:save My playlist` or `:q` to quit.

## Settings

Most settings are in `utuimp setup`. Everything else is in
`~/.config/utuimp/config.json`, for example:

- `theme`: colors. `system` uses your terminal's colors; others are
  `classic`, `dracula`, `gruvbox`, `nord`.
- `translate_lang`: the language lyrics are translated into, e.g. `"en"`.
- `layout`: `"cover"` starts in the cover view (`v` sets it).
- `youtube_cookies`: where yt-dlp takes YouTube cookies from, a browser name
  (`"firefox"`, `"chrome"`…) or the path of a `cookies.txt`.
- `ytdlp_path`, `ffmpeg_path`: where yt-dlp and ffmpeg are, if not on `PATH`.

Client IDs, secrets and sign-ins are also copied to
`~/.config/utuimp/credentials.json`. If an older utuimp rewrites the config
without them (one that doesn't know SoundCloud yet, say), the next start
takes them back from there. Settings an older build doesn't know are also
kept when it saves. To really remove a secret, clear it in `utuimp setup`.

## Apple Music tokens

Apple Music needs two tokens:

- a **developer token**, which identifies the app to Apple (the same for
  everyone, valid for a few days or weeks);
- a **user token**, which is your Apple ID's permission for that app. You
  get it by signing in in the browser window utuimp opens the first time. It
  is saved as `apple_user_token` in the config.

utuimp looks for the developer token in this order and takes the first one
that is still valid for at least three days:

1. A token built into the binary. `make install` builds one in when it finds
   your own MusicKit key (`APPLE_KEY_ID`, `APPLE_TEAM_ID`,
   `APPLE_PRIVATE_KEY` or `APPLE_PRIVATE_KEY_FILE`, in the environment or a
   `.env` file). Release builds instead ask the project's token service.
2. `apple_developer_token` in the config (`make refresh-token` writes one
   from your own key).
3. The token saved by the last refresh, in `~/.cache/utuimp/devtoken.json`.
4. A fresh one from [vibez](https://github.com/simonepelosi/vibez), the
   project utuimp started from, whose releases can fetch a developer token.
   utuimp downloads the latest vibez release for your system from GitHub
   (once a week at most), checks it against the release's `checksums.txt`,
   and keeps only the executable, in `~/.cache/utuimp/upstream/`. It runs it
   in a throwaway home directory with no access to your config, your browser
   or other programs. As soon as vibez saves its token, utuimp stops it,
   takes the token and saves it in `devtoken.json`. If GitHub is briefly
   unavailable, utuimp retries a couple of times.

If every step fails, the Apple Music tab says why. Usually it is the network
or GitHub, and restarting utuimp later helps.

## Problems

- **FLAC, M4A or WAV files do not play (Linux).** Install `gst-plugins-good`
  (Arch: `sudo pacman -S gst-plugins-good`, Debian/Ubuntu:
  `sudo apt install gstreamer1.0-plugins-good`).
- **Spotify says to install librespot.** Arch: `sudo pacman -S librespot`;
  elsewhere `cargo install librespot`. Without it utuimp can still control
  Spotify playing on your other devices.
- **Spotify says it is blocking the shared sign-in.** Spotify sometimes limits
  the app utuimp signs in with by default. Create your own (free):
  open the [Spotify dashboard](https://developer.spotify.com/dashboard),
  create an app with redirect URI `http://127.0.0.1:5588/login` and
  Web API ticked, then run `utuimp setup` and paste its Client ID.
- **YouTube Music: "confirm you're not a bot".** YouTube sometimes refuses
  yt-dlp without a signed-in browser. Open YouTube in your browser, then run
  `utuimp setup` and name that browser for the cookies (or set
  `youtube_cookies` in the config). yt-dlp reads the browser's YouTube
  cookies itself; they stay on your computer.
- **YouTube Music search shows songs without the artist.** yt-dlp lists
  search results by title only; the artist, album and cover appear once the
  song plays. Albums and mixes show everything at once.
- **SoundCloud says "only 30-second previews".** Those tracks are SoundCloud
  Go+ only; utuimp can play only what SoundCloud streams in full for free.
- **Spotify feed or playlists say "Forbidden (HTTP 403)".** Update utuimp:
  Spotify changed its API in 2026 and older versions use the retired paths.
  Spotify's radio (`R`) is built from songs by the same artist, since its
  recommendations aren't open to new apps.
- **Something went wrong.** Type `:debug-logs` to see what happened.

## Credits

utuimp started as a fork of [vibez](https://github.com/simonepelosi/vibez) by
Simone Pelosi.

Yandex Music support is based on the unofficial API documented by
[MarshalX/yandex-music-api](https://github.com/MarshalX/yandex-music-api) and
[oklookat/goym](https://github.com/oklookat/goym). Spotify support uses the
[Spotify Web API](https://developer.spotify.com/documentation/web-api) and
plays through [librespot](https://github.com/librespot-org/librespot).
YouTube Music and SoundCloud play through
[yt-dlp](https://github.com/yt-dlp/yt-dlp) and [FFmpeg](https://ffmpeg.org);
SoundCloud can also use the
[SoundCloud API](https://developers.soundcloud.com/docs/api/guide).
utuimp is not affiliated with Yandex, Apple, Spotify, Google or SoundCloud.

Lyrics come from [LRCLIB](https://lrclib.net), NetEase Cloud Music,
[lyrics.ovh](https://lyrics.ovh) and Yandex Music. The interface is built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).

## License

MIT, see [LICENSE](LICENSE).
