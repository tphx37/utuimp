# Roadmap

This document tracks planned features and integrations for utuimp.  
Items are roughly ordered by expected implementation complexity.

---

## Done

- **Local files** — MP3, FLAC, M4A from your music folder (2.1)
- **Yandex Music** — search, library, My Wave, synced lyrics (2.1)
- **Spotify** — Web API library and search, playback through librespot
- **YouTube Music** — through yt-dlp and ffmpeg (2.2)
- **SoundCloud** — through yt-dlp, or the official API with your own app (2.2)
- **Cover view** — frameless fullscreen layout with fading lyrics (2.2)
- **One volume for all services** (2.2)
- **Demo mode** — `--demo` for credential-free UI development

---

## In progress

- **MPRIS icon** — resolve the GIO / GTK icon-theme lookup for the MPRIS notification
- **YouTube Music search metadata** — search results come without artist and
  length until a song plays (yt-dlp lists them by title only)

---

## Planned

### Larger lyrics in kitty

Use kitty's text sizing protocol for genuinely bigger lyrics in the cover
view. It has to work around Bubble Tea's renderer, which drops those escape
sequences, so it needs care with resizes, overlays and panels.

### YouTube Music account

Sign-in through browser cookies, to show your own YouTube Music likes and
playlists instead of utuimp's local ones.

### Deezer

Full-track streaming via Deezer's API.

- OAuth 2.0 authorisation code flow
- Search, library, playlists via Deezer REST API
- Requires a Deezer Premium subscription

### Tidal

Full-track streaming via Tidal's API.

- OAuth 2.0 authorisation code flow
- HiFi / MQA quality tiers where the subscription supports them
- Search, library, playlists, mixes via Tidal API v2

---

## Ideas / not yet scoped

- **mpd protocol bridge** — let `ncmpcpp` and other mpd clients control utuimp
- **Keyboard macro recorder** — record and replay complex key sequences

---

If you'd like to work on any of these, please open an issue first so we can coordinate.
