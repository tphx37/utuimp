# Changelog

## 2.2

New services
- YouTube Music, through yt-dlp and ffmpeg. No sign-in: favorites and
  playlists are kept by utuimp, and the feed has a mix for each favorite.
- SoundCloud. By default through yt-dlp, with no account or subscription.
  With an Artist Pro app, the official API with your own likes and
  playlists.

Cover view
- `v` switches to cover view: the cover, the song,
  controls and a volume slider on the left, the lyrics on the right.
- The lyrics fill the height of the cover. The sung line sits near the top,
  earlier lines fade away above it, and line changes cross-fade.
- Search, queue, feed, library and the lyrics' source and language pickers
  open in a frame in place of the lyrics.
- The hints list every key that works in the view.

Playback
- One volume for all services: a change (or `:mute`) in one tab applies to
  every player.
- Radio and My Wave keep at least four songs queued after the current one.
- `/` opens search from any panel.

Fixes
- Spotify: playlists, the feed, search, artists' albums, likes and radio
  work again after Spotify's 2026 Web API changes.
- Spotify volume: +/- no longer jumps back or drifts down; a burst of
  presses is sent as one change.
- Apple Music: the developer-token download retries when GitHub has a
  hiccup, and the error says what failed.
- Settings survive older builds: unknown config keys are kept, and IDs,
  secrets and sign-ins are mirrored in `credentials.json`.
- Running the tests can no longer overwrite your real config.


## 2.1

- Tabs for Yandex Music, Apple Music, Spotify (placeholder) and local files.
- Yandex Music: search, library, playlists, likes, My Wave, synced lyrics.
- Russian interface.
- `utuimp setup`: language, Discord status, services.
- `--relogin` per service.
- Local MP3 files play without extra GStreamer plugins.
