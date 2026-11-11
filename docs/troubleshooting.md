# Troubleshooting

[← docs](README.md)

First, type `:debug-logs` inside utuimp. It shows what utuimp has been
doing, and the error is usually there. `utuimp --debug` logs in more detail.

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
M4A only.

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
config, or `utuimp setup interface`.

**The visualizer makes you dizzy.** Turn its strength down with `[`, or to
0 in `utuimp setup visualizer`, see [Visualizer](visualizer.md#strength).

**The start screen is in the way.** `"intro": false`.

## Reporting a bug

Open an [issue](https://github.com/tphx37/utuimp/issues) with what you did,
what happened, your system and terminal, and the relevant part of
`:debug-logs`.

Never paste tokens, cookies, client secrets, `config.json` or
`credentials.json`. Check the log for anything personal before posting it.
