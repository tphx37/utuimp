<p align="center">
  <img src="assets/utuimp_logo.png" alt="utuimp" width="128">
</p>

<h1 align="center">utuimp</h1>

<p align="center">universal terminal user interface media player</p>

A music player for the terminal. It plays Yandex Music, Apple Music and the
music files on your computer, and everything is done from the keyboard.

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
- **Spotify.** Needs Spotify Premium and
  [librespot](https://github.com/librespot-org/librespot), which plays the
  sound (Arch: `sudo pacman -S librespot`). The first time, approve access on
  the Spotify page that opens. The Spotify app doesn't need to be open.
- **Local files.** Plays the music in your `Music` folder. You can pick another
  folder in `utuimp setup`.

To sign in with a different account:

```bash
utuimp --relogin=yandex
utuimp --relogin=apple
utuimp --relogin=spotify
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
| `/` | search |
| `l` | your library |
| `q` | recently played |
| `y` | lyrics |
| `F` | recommendations (Yandex Music: My Wave) |
| `e` | equalizer |
| `i` | song menu: go to the album or an artist, copy the link |
| `?` | about |
| `esc` | back |

**Queue.** Pick a song with the arrow keys. `enter` plays it, `d` removes it,
`K` and `J` move it up and down.

**Lyrics.** The words light up as they are sung. `t` shows a translation next
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
- **Something went wrong.** Type `:debug-logs` to see what happened.

## Credits

utuimp started as a fork of [vibez](https://github.com/simonepelosi/vibez) by
Simone Pelosi.

Yandex Music support is based on the unofficial API documented by
[MarshalX/yandex-music-api](https://github.com/MarshalX/yandex-music-api) and
[oklookat/goym](https://github.com/oklookat/goym). Spotify support uses the
[Spotify Web API](https://developer.spotify.com/documentation/web-api) and
plays through [librespot](https://github.com/librespot-org/librespot).
utuimp is not affiliated with Yandex, Apple or Spotify.

Lyrics come from [LRCLIB](https://lrclib.net), NetEase Cloud Music,
[lyrics.ovh](https://lyrics.ovh) and Yandex Music. The interface is built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).

## License

MIT, see [LICENSE](LICENSE).
