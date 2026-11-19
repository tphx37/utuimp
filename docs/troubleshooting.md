# Troubleshooting

[← docs](README.md)

First, type `:debug-logs` inside utuimp. It shows what utuimp has been
doing, and the error is usually there. Run `utuimp --debug` to also save
diagnostics and crash reports to `~/.cache/utuimp/debug.log` (on Windows,
`utuimp/debug.log` under the user cache directory). Review the log for private
information before sharing it.

- [Playback](#playback)
- [Spotify](#spotify)
- [YouTube Music](#youtube-music-confirm-youre-not-a-bot)
- [SoundCloud](#soundcloud)
- [Apple Music](#apple-music)
- [Interface](#interface)
- [Reporting a bug](#reporting-a-bug)

## Playback

**FLAC, M4A or WAV files don't play (Linux).** Install
[gst-plugins-good](https://gstreamer.freedesktop.org/documentation/installing/on-linux.html)
(and `gst-libav`). `sh scripts/install.sh --deps` finds what is missing.

**A local file isn't listed.** Check the format is supported on your system,
see [Local files](services.md#local-files). Windows plays MP3, FLAC and
M4A only. Check all configured paths under Settings → Services → Music
folders; press `Ctrl+R` in the Local files library or run `:refresh` on
that tab after adding files.

**A radio link doesn't play.** Use a direct stream URL, an M3U/PLS playlist,
or a page that exposes the stream in its audio player or metadata. Some pages,
including Apple Music station pages, don't provide a direct playable URL.
Available codecs and HLS support depend on the audio backend and installed
plugins. The station may not supply song metadata.

## Spotify

**Spotify says to install librespot.** utuimp plays Spotify through
[librespot](https://github.com/librespot-org/librespot)
([compiling it](https://github.com/librespot-org/librespot/wiki/Compiling),
or your distribution's package). If it is installed somewhere outside your
`PATH`, set `librespot_path`. Without it, utuimp can still control Spotify
playing on your other devices.

**Spotify says it is blocking the shared sign-in.** Spotify sometimes
limits the app utuimp signs in with by default. Create your own (free):
open the [Spotify dashboard](https://developer.spotify.com/dashboard),
create an app with redirect URI `http://127.0.0.1:5588/login` and Web API
ticked, then run `utuimp setup` and paste its Client ID.

**Feed or playlists say "Forbidden (HTTP 403)".** Update utuimp: Spotify
changed its API in 2026 and older versions use the retired paths.

**Playing a search result or playlist fails with "Restriction violated" (HTTP 403).**
If Spotify's app can play on the utuimp device but utuimp cannot start a
track, librespot 0.8.0 may be affected by a change in Spotify's playback
requests. Its log is `~/.cache/utuimp/librespot.log` on Linux; the errors
`Client specified an invalid argument` and `context is not available`
at the same time as the 403 match this problem. Other restrictions can
also produce a 403.

An [upstream fix](https://github.com/librespot-org/librespot/pull/1870)
is available but has not yet been released. Until a release containing it
is available, a Linux user can build the tested revision (requires Git,
Rust, Cargo, a C compiler, pkg-config and ALSA development headers):

```sh
git clone https://github.com/stefanbd/librespot.git
cd librespot
git checkout afb2e6aa6ead6206089141b46e7b88134657e87d
cargo build --release --locked --no-default-features --features rustls-tls-native-roots,rodio-backend
```

Set `librespot_path` in the [config](configuration.md#config-file) to the
absolute path of `target/release/librespot`, then restart utuimp. This is
an unreleased build; replace it with a released version containing the fix
when available. The patch does not resolve every possible Spotify 403.

**Radio (`R`) plays only the same artist.** That's expected. Spotify's
recommendations aren't open to new apps, so the radio is built from songs
by the same artist.

## YouTube Music: "confirm you're not a bot"

YouTube sometimes refuses yt-dlp without a signed-in browser. Open YouTube
in your browser, then run `utuimp setup services` and name that browser
for the cookies (or set `youtube_cookies` in the config to a browser name
or a `cookies.txt` path). yt-dlp reads the browser's YouTube cookies
itself, and they stay on your computer.

Keeping yt-dlp up to date helps too. YouTube changes often and old
versions stop working.

**Search shows songs without the artist.** yt-dlp lists search results by
title only. The artist, album and cover appear once the song plays. Albums
and mixes show everything at once.

## SoundCloud

**"Only 30-second previews".** Those tracks are SoundCloud Go+ only. utuimp
can play only what SoundCloud streams in full for free.

**Search is slow.** yt-dlp asks SoundCloud page by page, so it takes a few
seconds.

## Apple Music

**The tab says it has no developer token.** Usually the network or GitHub
is down. Restart utuimp later. The details are in
[Apple Music tokens](apple-music-tokens.md).

**Signed in with the wrong Apple ID.** `utuimp --relogin=apple`.

## Interface

**Colours look wrong.** Try another `theme`. `system` follows your
terminal's palette, the others use fixed colours. See
[Themes](configuration.md#themes).

**The cover is missing or blocky.** Covers need a terminal with at least
256 colours (true colour looks best).

**Something fades in and you'd rather it didn't.** `ui_motion: "off"` in the
config, or `utuimp setup interface`. That puts the whole interface in
characters: for the pixel lines standing still instead, choose Settings →
Graphics → Frames and lines: still.

**The pixel interface is gone (lines, the cover as a picture).** It needs
kitty or Ghostty, and is off with Animation off or Graphics → Pixel
interface off. Not in tmux nor over ssh.

**The lyrics are in the terminal's letters, not drawn large.** Drawn text
needs fontconfig (`fc-match` on the path) and a font it finds; otherwise, or
with Graphics → Text: terminal, they stay letters.

**utuimp uses more processor than you'd like.** While a song plays the
interface keeps a fraction of one core busy (it draws as many frames a
second as fit the screen, 60 or 72, see
[Environment variables](configuration.md#environment-variables)); paused,
next to nothing. `UTUIMP_FPS=60` on a 144 Hz screen is a little cheaper and
a little less smooth. The visualizer is the heavy part, see
[Visualizer](visualizer.md#real-pixels). On battery: Graphics → Frames and
lines: still, Light behind the cover: off, or Pixel interface: off for the
least.

**Animations stutter.** Note where (a view change, the lyrics, opening a
panel, a visualizer scene) and run `UTUIMP_FRAMES=/tmp/frames.log utuimp`
while it happens: every frame of every animation is in it, with how long it
took. Attach the log to the [issue](#reporting-a-bug), and say how often
your screen refreshes.

**A picture is in the wrong place, or one is left behind.** Run
`UTUIMP_LINES_LOG=/tmp/lines.log utuimp`, show it happening, and attach the
log (it holds no songs nor accounts) to the [issue](#reporting-a-bug), with
your terminal and its version.

**The visualizer makes you dizzy.** Turn its strength down with `[`, or to
0 in `utuimp setup visualizer`, see [Visualizer](visualizer.md#strength).

**The start screen is in the way.** `"intro": false`.

## Reporting a bug

Open an [issue](https://github.com/tphx37/utuimp/issues) with what you did,
what happened, your system and terminal, and the relevant part of
`:debug-logs`.

Never paste tokens, cookies, client secrets, `config.json` or
`credentials.json`. Check the log for anything personal before posting it.
