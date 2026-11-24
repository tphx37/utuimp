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
2. downloads and verifies a published release when available, otherwise
   builds from source. It installs to `~/.local/bin` on Linux and macOS,
   or the directory of the existing `utuimp.exe` on Windows (otherwise
   `%LOCALAPPDATA%\Programs\utuimp`). The current PowerShell session is updated
   too. An alias or function named `utuimp` takes precedence over the executable;
   use `Get-Command utuimp -All` to inspect conflicting commands.

Options for `install.sh` (when piped from curl, pass them after `sh -s --`):

| Option | What it does |
|---|---|
| `--deps` | only check and offer to install dependencies |
| `--source` | build from source even when a release exists |
| `--yes` | don't ask, install what is missing |

Environment: `UTUIMP_INSTALL_DIR` changes the install directory (default
`~/.local/bin` on Unix; on Windows it overrides the directory above), `UTUIMP_YES=1` is the same as `--yes`.

Without a terminal (in CI, for example) the script installs no dependencies
unless `--yes` is given.

Then start it in any terminal:

```bash
utuimp
```

## Package managers

### pacman (Arch Linux, x86_64)

Configure the signing key and repository using the
[package repository instructions](https://github.com/tphx37/utuimp-repo#arch-linux-x86_64), then:

```bash
sudo pacman -Syu utuimp
```

pacman downloads the package and installs GStreamer and fontconfig dependencies.
The repository supports releases starting with 4.0.1.

### yay / paru and AUR

After configuring the signed repository above, install the release by name:

```bash
yay -Syu utuimp
# Or:
paru -Syu utuimp
```

This downloads the signed package automatically.


The AUR recipe is included in this repository. Build it now with:

```bash
sudo pacman -S --needed base-devel git
git clone https://github.com/tphx37/utuimp.git
cd utuimp/packaging/aur/utuimp-git
makepkg -si
```

After the recipe is published to AUR, `yay -S utuimp-git` or
`paru -S utuimp-git` installs it directly. It is not yet registered there.

### Homebrew (macOS and Linux)

```bash
brew tap --custom-remote tphx37/utuimp-repo https://github.com/tphx37/utuimp-repo.git
brew install tphx37/utuimp-repo/utuimp
# Update:
brew upgrade tphx37/utuimp-repo/utuimp
```

This installs a release binary on macOS (Intel or Apple Silicon) or Linux x86_64.
For YouTube Music / SoundCloud, also run `brew install yt-dlp ffmpeg`;
for Spotify Premium, `brew install librespot`.

### winget (Windows x64)

Release builds provide `packaging.zip` with winget manifests. Until the
package is accepted into the winget community catalog, use a local manifest
in PowerShell (run the settings command in an administrator terminal):

```powershell
winget settings --enable LocalManifestFiles
Invoke-WebRequest https://github.com/tphx37/utuimp/releases/latest/download/packaging.zip -OutFile packaging.zip
Expand-Archive .\packaging.zip -DestinationPath .\utuimp-packaging -Force
$manifest = Get-ChildItem .\utuimp-packaging\winget\manifests\t\tphx37\utuimp -Directory | Select-Object -First 1
winget install --manifest $manifest.FullName
```

The [community catalog submission](https://github.com/microsoft/winget-pkgs/pull/483188)
is pending. After acceptance, use `winget install --id tphx37.utuimp --exact`.



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

Use the package manager that installed utuimp:

```bash
# Arch package / signed pacman repository
sudo pacman -Rns utuimp
# yay / paru with the signed repository
yay -Rns utuimp
# AUR recipes: choose the installed package (paru accepts the same commands)
yay -Rns utuimp-git
yay -Rns utuimp-bin
# macOS or Linux / Homebrew
brew uninstall utuimp
brew untap tphx37/utuimp-repo
# Built with make, from the source directory
make uninstall
# For a custom prefix, repeat the one used for installation:
make uninstall PREFIX=/usr/local
```

Windows / winget:

```powershell
winget uninstall --id tphx37.utuimp --exact
```

Linux / macOS / install script, with the default directory:

```bash
rm -f "$HOME/.local/bin/utuimp"
# Linux desktop entries, if installed:
rm -f "$HOME/.local/share/applications/io.github.tphx37.utuimp.desktop"
rm -f "$HOME/.local/share/icons/hicolor/scalable/apps/io.github.tphx37.utuimp.svg"
```

If `UTUIMP_INSTALL_DIR` was set, remove the binary from that directory.
Windows / PowerShell install script (also removes its user PATH entry):

```powershell
$dir = Join-Path $env:LOCALAPPDATA "Programs\utuimp"
# If installed with UTUIMP_INSTALL_DIR, use that directory instead.
Remove-Item (Join-Path $dir "utuimp.exe") -ErrorAction SilentlyContinue
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
[Environment]::SetEnvironmentVariable("Path", (($userPath -split ";" | Where-Object { $_.TrimEnd("\") -ne $dir.TrimEnd("\") }) -join ";"), "User")
```

Close utuimp first. Removal keeps your settings and sign-ins. For a full
reset, optionally delete these directories too:

| Platform | Configuration | Cache |
|---|---|---|
| Linux | `~/.config/utuimp` | `~/.cache/utuimp` |
| macOS | `~/.config/utuimp` | `~/Library/Caches/utuimp` |
| Windows | `%APPDATA%\utuimp` | `%LOCALAPPDATA%\utuimp` |

`XDG_CONFIG_HOME` overrides the configuration root on Linux/macOS;
`XDG_CACHE_HOME` overrides the Linux cache root. Local music is stored
separately.
