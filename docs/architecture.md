# Architecture

[← docs](README.md)

How utuimp is put together, for anyone changing the code. Setting up,
tests and pull requests are in [Contributing](../CONTRIBUTING.md).

- [How the code is laid out](#how-the-code-is-laid-out)
- [Adding a music service](#adding-a-music-service)
- [Translations](#translations)

## How the code is laid out

```
cmd/                     command line: flags, setup, starting each service
  services.go            the list of tabs (serviceDefs)
  setup.go               `utuimp setup` and its menu of sections
  *_flow.go              sign-in and start-up of each service
  discord.go             starting Discord Rich Presence
internal/
  config/                config.json (and credentials.json, see keep.go)
  i18n/                  translations (ru.go)
  signin/                the browser sign-in page all services share
  provider/              music services: search, library, albums, radio…
    provider.go          the Provider interface, and the optional ones
                         (PlaylistEditor, LibraryCollections, CollectionSaver…)
    link.go              parsing the services' share links
    apple/ yandex/ spotify/ ytmusic/ soundcloud/ local/ demo/
    ytdlp/               running yt-dlp and streaming through ffmpeg
    localstore/          favourites and playlists kept on disk
  player/                playback
    player.go            the Player interface
    broadcast.go         fanning player state out to subscribers
    ystream/             plays MP3/AAC streams (Yandex, YouTube, SoundCloud)
    browserless/         Apple Music (Widevine) and the audio outputs
    spotifyconnect/      Spotify through librespot. On Linux utuimp plays
                         librespot's sound itself (capture.go, output_linux.go)
    gst/                 GStreamer playback, also of raw PCM (Spotify on Linux)
    local/               local files
    tap/                 hands the sound being played to the visualizer
    mpris/ smtc/ mux/    system media controls. mux follows the playing service
  tui/                   the interface (Bubble Tea v2)
    model.go             state, keys and the boxed view
    cover.go             the cover view (v)
    visualizer*.go       the fullscreen visualizer (V): scenes, beat finding,
                         drawing
    fx.go                fade-ins of what opens
    intro*.go            the start screen
    link.go              share links in search and :link
    playlists.go         making, renaming, deleting and filling playlists
    preview.go           an album or playlist opened as a list of its songs
    songmenu.go          the song menu (i)
    styles/              themes, the terminal's palette, the Linux console
    art/                 covers drawn in the terminal (and on the console)
    views/               panels: library, search, lyrics, queue, feed…
  lyrics/ lastfm/ discord/ auth/ updater/
assets/                  logos. services/ holds each service's icon for README
scripts/                 install.sh, install.ps1
flatpak/                 the Flatpak manifest and its icon
```

## Adding a music service

1. Implement `provider.Provider` in `internal/provider/<name>/`, plus
   `provider.SongDetailer` for the song menu (`i`). Implement the optional
   interfaces the service can do: `PlaylistEditor` (remove, rename,
   delete), `LibraryCollections` and `CollectionSaver` (saved albums,
   followed artists), `LibraryAdder`. A service without playlists of its
   own can keep them in `localstore`.
2. Playback: if the service gives MP3 or AAC (a link or a stream), use
   `ystream`. Implement `StreamURL` for a link, or `ystream.Opener` when you
   produce the stream yourself (see `ytmusic/stream.go`). Otherwise write a
   `player.Player`.
3. Start it in `cmd/<name>_flow.go` and add it to `serviceDefs` in
   `cmd/services.go`, before local files, which stay last.
4. Add it to `setupServices` in `cmd/setup.go`, and to `signOut` in
   `cmd/root.go` if it signs in.
5. New config fields go in `internal/config/config.go` and in
   [configuration.md](configuration.md#config-file). Add any ID, secret
   or token to `credentialKeys` in `internal/config/keep.go`.
6. Teach `ParseLink` in `internal/provider/link.go` the service's share
   links (with a case in `link_test.go`).
7. If it signs in through the browser, use the shared page in
   `internal/signin` and add the service's colours there.
8. Translate every new user-facing string ([Translations](#translations)),
   describe the service in [services.md](services.md) and add its icon to
   `assets/services/`.

## Translations

Interface text is written in English in the code, wrapped in `i18n.T("…")`.
Translations live in a table per language. Russian is `internal/i18n/ru.go`.
The key is the English text with surrounding spaces trimmed, so `" search"`
and `"search"` share one entry. Text without a translation shows in English.

To add a language, add a table like `ru.go`, list it in `i18n.Languages` and
handle it in `i18n.Set`.

