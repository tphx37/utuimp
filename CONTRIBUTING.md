# Contributing to utuimp

utuimp is a music player for the terminal, written in Go. It plays Yandex
Music, Apple Music, Spotify, YouTube Music, SoundCloud and local files, each
in its own tab. Bug fixes, new features, translations and documentation are
all welcome. This guide covers what you need to work on it.

## Before you start

- **Small fixes** (a bug, a typo, a missing translation): just open a pull
  request.
- **Bigger changes** (a new service, a new view, a change to how playback
  works): open an issue first and describe the idea, so we can agree on the
  approach before you spend time on it.
- Check the [issues](https://github.com/tphx37/utuimp/issues) and
  [ROADMAP.md](ROADMAP.md) to see what is planned or already being worked on.

## Setting up

You need:

- **Go 1.26** or newer.
- **Linux:** GStreamer with its base and good plugins and the libav plugin.
  Arch: `sudo pacman -S gstreamer gst-plugins-base gst-plugins-good gst-libav`.
  Debian/Ubuntu: `sudo apt install libgstreamer1.0-dev
  libgstreamer-plugins-base1.0-dev gstreamer1.0-plugins-good
  gstreamer1.0-libav`.
- **macOS / Windows:** nothing extra to build. Windows needs no CGo
  toolchain.

Only for the service you work on:

| Service | Needs |
|---|---|
| Spotify | Spotify Premium and [librespot](https://github.com/librespot-org/librespot) |
| YouTube Music, SoundCloud | [yt-dlp](https://github.com/yt-dlp/yt-dlp) and ffmpeg |
| Apple Music | an Apple Music subscription (see [Apple Music tokens](README.md#apple-music-tokens)) |
| Yandex Music | a Yandex account |

Then:

```sh
git clone https://github.com/tphx37/utuimp
cd utuimp
go run . --demo
```

### Demo mode: no accounts needed

`--demo` runs utuimp with a built-in fake player and ten fake tracks. Search,
queue, lyrics layout, keys and both views work as usual, and nothing goes to
the network. Use it for any work on the interface.

```sh
go run . --demo
```

To try a real service, run `go run .`. The first start asks for language and
services; `go run . setup` asks again.

**Your real config is at risk while you develop.** utuimp saves
`~/.config/utuimp/config.json` as you use it. To keep a development config
apart from your everyday one, pass `--config`:

```sh
go run . --config /tmp/utuimp-dev.json
```

## Building

```sh
make build          # builds ./utuimp and also installs it to ~/.local/bin
NO_INSTALL=1 make build   # builds ./utuimp only
make install        # also builds in your Apple MusicKit token if you have one
```

`make build` checks for GStreamer first on Linux.

## Tests

```sh
go test ./...
```

All tests run without accounts or network. They fake the services with
`httptest` servers and the external programs (yt-dlp, ffmpeg) with small
scripts or recorded output. A few tests that need ffmpeg skip themselves
when it is missing.

Tests never write to your real files. Each package that saves anything has a
`TestMain` that points `HOME` and the XDG directories at a temporary
directory, and `config.Save` refuses to write the default config from a test
binary. If you add a test package that can save the config, history or a
library file, give it the same `TestMain` (copy `internal/tui/main_test.go`).

Write tests for what you change. Bugs get a test that fails before the fix.
TUI tests drive the model directly. Key presses use the Bubble Tea v2 form:

```go
m.handleNormalKey(tea.KeyPressMsg{Code: 'v', Text: "v"}, "v")
m.handleNormalKey(tea.KeyPressMsg{Code: tea.KeyEscape}, "esc")
```

and the rendered screen is `ansi.Strip(m.View().Content)`. See
`internal/tui/cover_test.go` for an example.

## Code style

- `gofmt` and `goimports`. `go vet ./...` and `golangci-lint run` (config in
  `.golangci.yml`) must be clean.
- [pre-commit](https://pre-commit.com) runs build, vet, tests, `go mod tidy`
  and the linter: `pre-commit install` once, and it runs on every commit.
- Return errors with context (`fmt.Errorf("loading the playlist: %w", err)`);
  don't panic.
- Comments explain *why*, not *what*. A comment on a type or function says
  what it is for and anything surprising about it.
- `View()` runs many times a second: no network, disk or heavy work in it.
  Fetch in a `tea.Cmd` and send the result back as a message.

## How the code is laid out

```
cmd/                     command line: flags, setup, starting each service
  services.go            the list of tabs (serviceDefs)
  setup.go               `utuimp setup`
  *_flow.go              sign-in and start-up of each service
internal/
  config/                config.json (and credentials.json, see keep.go)
  i18n/                  translations (ru.go)
  provider/              music services: search, library, albums, radio…
    provider.go          the Provider interface every service implements
    apple/ yandex/ spotify/ ytmusic/ soundcloud/ local/ demo/
    ytdlp/               running yt-dlp and streaming through ffmpeg
    localstore/          favourites and playlists kept on disk
  player/                playback
    player.go            the Player interface
    ystream/             plays MP3/AAC streams (Yandex, YouTube, SoundCloud)
    browserless/         Apple Music (Widevine) and the audio outputs
    spotifyconnect/      Spotify through librespot
    local/ gst/          local files
    mpris/ smtc/ mux/    system media controls
  tui/                   the interface (Bubble Tea v2)
    model.go             state, keys and the boxed view
    cover.go             the cover view (v)
    views/               panels: library, search, lyrics, queue, feed…
  lyrics/ lastfm/ discord/ auth/ updater/
```

### Adding a music service

1. Implement `provider.Provider` in `internal/provider/<name>/`, plus
   `provider.SongDetailer` for the song menu (`i`).
2. Playback: if the service gives MP3 or AAC (a link or a stream), use
   `ystream`. Implement `StreamURL` for a link, or `ystream.Opener` when you
   produce the stream yourself (see `ytmusic/stream.go`). Otherwise write a
   `player.Player`.
3. Start it in `cmd/<name>_flow.go` and add it to `serviceDefs` in
   `cmd/services.go`, before local files, which stay last.
4. Add it to `setupServices` in `cmd/setup.go`, and to `signOut` in
   `cmd/root.go` if it signs in.
5. New config fields go in `internal/config/config.go`. Add any ID, secret
   or token to `credentialKeys` in `internal/config/keep.go`.
6. Translate every new user-facing string (next section) and describe the
   service in README.md.

## Translations

Interface text is written in English in the code, wrapped in `i18n.T("…")`.
Translations live in a table per language; Russian is `internal/i18n/ru.go`.
The key is the English text with surrounding spaces trimmed, so `" search"`
and `"search"` share one entry. Text without a translation shows in English.

To add a language, add a table like `ru.go`, list it in `i18n.Languages` and
handle it in `i18n.Set`.

## Documentation

README.md is for people who use utuimp: what to install, how to sign in,
which keys do what, how to fix common problems. Update it in the same pull
request as a change people will notice. Keep it plain: short sentences, no
jargon a listener wouldn't know.

## Pull requests

1. Fork the repository and create a branch from `main`.
2. Make the change, with tests.
3. Run `go build ./...`, `go vet ./...`, `go test ./...` and
   `golangci-lint run`.
4. Open the pull request. Say what you changed and why, how you tested it,
   and which services you tried it with. Add a screenshot for visible
   changes.

`main` is protected. A pull request needs one approving review, all review
threads resolved and a linear history, so rebase on `main` instead of
merging it in.

## Security and privacy

- Never paste tokens, cookies, client secrets or your `config.json` /
  `credentials.json` into issues or logs. `:debug-logs` output is safe to
  share once you have checked it for anything personal.
- Report security problems privately to the maintainer, not in a public
  issue.

## License

By contributing you agree that your work is released under the
[MIT license](LICENSE).
