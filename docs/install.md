# Install

[← docs](README.md)

- [The install script](#the-install-script)
- [Building from source](#building-from-source)
- [Requirements](#requirements)
- [The application menu](#the-application-menu)
- [Updates](#updates)
- [Uninstalling](#uninstalling)

## The install script

**Linux and macOS:**

```bash
curl -fsSL https://raw.githubusercontent.com/tphx37/utuimp/main/scripts/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/tphx37/utuimp/main/scripts/install.ps1 | iex
```

The script:

1. looks for what utuimp needs and offers to install anything missing with
   your package manager (pacman, apt, dnf, zypper, Homebrew or winget). It
   asks before installing anything.
2. installs utuimp to `~/.local/bin`. It takes a published release when
   there is one for your system, otherwise it builds from source.

Options for `install.sh` (when piped from curl, pass them after `sh -s --`):

| Option | What it does |
|---|---|
| `--deps` | only check and offer to install dependencies |
| `--source` | build from source even when a release exists |
| `--yes` | don't ask, install what is missing |

Environment: `UTUIMP_INSTALL_DIR` changes the install directory (default
`~/.local/bin`), `UTUIMP_YES=1` is the same as `--yes`.

Without a terminal (in CI, for example) the script installs no dependencies
unless `--yes` is given.

Then start it in any terminal:

```bash
utuimp
```

## Building from source

```bash
git clone https://github.com/tphx37/utuimp
cd utuimp
make build
```

`make build` runs the same dependency check first and offers to install
what is missing. It builds `./utuimp`, copies it to `~/.local/bin` and adds
utuimp to the application menu.

| Variable | Effect |
|---|---|
| `NO_DEPS=1` | skip the dependency check |
| `NO_INSTALL=1` | only build `./utuimp`, don't install it or add the menu entry |
| `PREFIX=…` | install under another prefix (default `~/.local`) |

`make install` also builds in an Apple Music developer token when it finds
your own MusicKit key, see [Apple Music tokens](apple-music-tokens.md).

## Requirements

| | For | Get it |
|---|---|---|
| [Go](https://go.dev/doc/install) 1.26+ | building utuimp | [go.dev/dl](https://go.dev/dl/) |
| [Git](https://git-scm.com/downloads) | fetching the source | [git-scm.com](https://git-scm.com/downloads) |
| C compiler and [pkg-config](https://www.freedesktop.org/wiki/Software/pkg-config/) | building, Linux only | [GCC](https://gcc.gnu.org/install/binaries.html) |
| [GStreamer](https://gstreamer.freedesktop.org/) with the base, [good](https://gstreamer.freedesktop.org/modules/gst-plugins-good.html) and [libav](https://gstreamer.freedesktop.org/modules/gst-libav.html) plugins | the sound, Linux only | [installing on Linux](https://gstreamer.freedesktop.org/documentation/installing/on-linux.html) |
| [yt-dlp](https://github.com/yt-dlp/yt-dlp) | YouTube Music and SoundCloud (optional) | [yt-dlp installation](https://github.com/yt-dlp/yt-dlp/wiki/Installation) |
| [FFmpeg](https://ffmpeg.org/) | YouTube Music and SoundCloud (optional) | [ffmpeg.org/download](https://ffmpeg.org/download.html) |
| [librespot](https://github.com/librespot-org/librespot) | Spotify playback (optional, needs Premium) | [compiling librespot](https://github.com/librespot-org/librespot/wiki/Compiling), or your distribution's package |

Go, Git and the compiler are only needed to build from source. A release
installed by the script needs only the runtime parts (GStreamer on Linux,
plus yt-dlp, FFmpeg and librespot for the services that use them).

GStreamer packages by distribution:

```bash
# Arch
sudo pacman -S gstreamer gst-plugins-base gst-plugins-good gst-libav
# Debian / Ubuntu
sudo apt install gstreamer1.0-plugins-base gstreamer1.0-plugins-good gstreamer1.0-libav
```

macOS and Windows need nothing extra for the sound.

## The application menu

The install adds utuimp to your system's list of applications (on Linux:
`~/.local/share/applications/io.github.tphx37.utuimp.desktop`). utuimp is
still a terminal program: started from the menu, it opens in a terminal
window (kitty, foot, alacritty, wezterm, ghostty, konsole, or whatever
`xdg-terminal-exec` picks), and the task bar shows it as utuimp, with its
icon.

`utuimp --window` does the same from a terminal: it opens utuimp in a window
of its own.

## Updates

utuimp checks GitHub for a new release at most once a day. When there is
one, it downloads it, checks its SHA-256 checksum and replaces itself. The
new version starts next time. To turn this off for one run:

```bash
utuimp --no-update
```

`utuimp version` shows the installed version.

## Uninstalling

If you built with `make`:

```bash
make uninstall
```

Otherwise remove `~/.local/bin/utuimp`, and on Linux
`~/.local/share/applications/io.github.tphx37.utuimp.desktop` and
`~/.local/share/icons/hicolor/scalable/apps/io.github.tphx37.utuimp.svg`.

Your settings and sign-ins are in `~/.config/utuimp/` (Windows:
`%APPDATA%\utuimp\`) and the cache in `~/.cache/utuimp/`. Delete them too if
you want nothing left.
