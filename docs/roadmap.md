# Roadmap

[← docs](README.md)

This document tracks the current state of utuimp and possible future work.
Planned items are proposals, not release commitments.

## Available

- **Local files**: recursive scanning across several folders, manual refresh,
  local playlists and sidecar lyrics. See [Services](services.md#local-files).
- **Yandex Music**: search, library, My Wave and service lyrics.
- **Spotify**: Web API library and search, with librespot playback.
- **YouTube Music**: search with song, artist and album metadata, playback
  through yt-dlp and FFmpeg, and account likes/playlists with exported cookies.
- **SoundCloud**: yt-dlp playback or the official API with an app of your own.
- **Internet radio**: Radio Browser search, direct stream and supported HTML
  links, favorites, playlists, recent links and Linux stream metadata. Some
  websites do not expose a direct audio stream.
- **Interface**: cover and boxed views, large pixel lyrics on supported
  terminals, visualizer, shared volume, MPRIS and other system controls.
- **Diagnostics**: `--debug` writes a log; `:debug-logs` shows the in-app log.
- **Demo mode**: `--demo` for a credential-free interface.

## Needs investigation

- **MPRIS icon**: the desktop entry and icon are installed and the MPRIS
  `DesktopEntry` is set; verify icon-theme lookup in desktop environments
  where the notification icon still does not appear.
- **Internet radio**: improve stream format and metadata support across
  platforms, and investigate pages that obtain streams dynamically.
- **Local files**: consider automatic filesystem watching and richer folder
  management; currently a manual rescan updates the library.
- **Lyrics**: explore terminal-native text sizing where graphics are not
  available; large rendered lyrics already work with the pixel interface.

## Planned / exploratory

- **Discord voice bot**: play utuimp output into a voice channel.
- **Deezer**: investigate authentication, catalog and full-track playback.
- **Tidal**: investigate authentication, catalog and full-track playback.

Service support depends on available APIs, playback rights and dependencies;
no specific account or quality tier is promised yet.

## Ideas / not yet scoped

- **mpd protocol bridge**: let `ncmpcpp` and other mpd clients control utuimp.
- **Keyboard macro recorder**: record and replay key sequences.

If you'd like to work on an item, please open an issue first so we can coordinate.
