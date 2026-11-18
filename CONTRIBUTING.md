# Contributing to utuimp

[← docs](docs/README.md)

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
  [the roadmap](docs/roadmap.md) to see what is planned or already being worked on.

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

`scripts/install.sh` (Linux, macOS) and `scripts/install.ps1` (Windows)
find what is missing and offer to install it with the system's package
manager. `sh scripts/install.sh --deps` only does that part, which is also
what `make build` runs first.

Only for the service you work on:

| Service | Needs |
|---|---|
| Spotify | Spotify Premium and [librespot](https://github.com/librespot-org/librespot) |
| YouTube Music, SoundCloud | [yt-dlp](https://github.com/yt-dlp/yt-dlp) and ffmpeg |
| Apple Music | an Apple Music subscription (see [Apple Music tokens](docs/apple-music-tokens.md)) |
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
services. `go run . setup` asks again.

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

`make build` first runs `scripts/install.sh --deps`, which checks for
GStreamer and the services' programs and offers to install what is
missing. It only asks on a terminal, `NO_DEPS=1` skips it. `make build`
also adds utuimp to the desktop's application list (`utuimp
setup-desktop`).

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

Some things only show in a real terminal: the cover view, the `system`
theme (which asks the terminal for its palette), the fades, the
visualizer. Check them by running the binary, not only in tests. Handy
switches:

- `UTUIMP_CONSOLE=0` / `=1` turns the Linux console mode off or forces it
  on.
- `UTUIMP_SPOTIFY_OUTPUT=librespot` lets librespot play the sound itself
  instead of utuimp.
- `UTUIMP_ICON=1 go test ./internal/tui -run TestRenderIcon` draws the
  app icon again.
- `"intro": false` in the config skips the start screen.

## Code style

- `gofmt` and `goimports`. `go vet ./...` and `golangci-lint run` (config in
  `.golangci.yml`) must be clean.
  The version is the one in `.pre-commit-config.yaml` (an older one can't
  read the packages of a newer Go):
  `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0`.
- [pre-commit](https://pre-commit.com) runs build, vet, tests, `go mod tidy`
  and the linter: `pre-commit install` once, and it runs on every commit.
- Return errors with context (`fmt.Errorf("loading the playlist: %w", err)`)
  and don't panic.
- Comments explain *why*, not *what*. A comment on a type or function says
  what it is for and anything surprising about it.
- `View()` runs many times a second: no network, disk or heavy work in it.
  Fetch in a `tea.Cmd` and send the result back as a message.
- The interface doesn't move and adds no colours of its own: no sliding or
  blinking. The only effect is a soft fade-in of what opens (`fx.go`),
  which `ui_motion: off` turns off. Colours come from the theme or the
  cover.

## How the code works

The layout of the code, adding a music service and adding a language are in
[Architecture](docs/architecture.md).

## Documentation

The front-page `README.md` is short on purpose: install, first start, the
main keys, and links. Everything else lives in `docs/`:

| Change | Page to update |
|---|---|
| a key, a panel, a command | [usage.md](docs/usage.md) (and the README's key table if it's a main key) |
| a service, its sign-in or requirements | [services.md](docs/services.md), [install.md](docs/install.md) |
| a config field, flag or environment variable | [configuration.md](docs/configuration.md) |
| the visualizer | [visualizer.md](docs/visualizer.md) |
| a known problem and its fix | [troubleshooting.md](docs/troubleshooting.md) |
| code layout, interfaces | [architecture.md](docs/architecture.md) |

Update the docs in the same pull request as a change people will notice,
and add a line to [CHANGELOG.md](CHANGELOG.md). Keep them plain: short
sentences, no jargon a listener wouldn't know.

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
- Report security problems privately as described in the [security policy](SECURITY.md),
  not in a public issue.

## License

By contributing you agree that your work is released under the
[GNU General Public License v3.0 or later](LICENSE).
