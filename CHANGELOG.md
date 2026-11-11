# Changelog

## 3.0.1

Start screen
- With the system theme the ribbons use the real colours of the terminal's
  palette (as it reported them), not xterm's standard ones, so they follow
  the terminal's colour scheme.
Install
- `install.sh` finds the latest release from the redirect of
  `github.com/<repo>/releases/latest` instead of the GitHub API. The API's
  limit for anonymous calls made the lookup fail, the script took that for
  "no release" and built from source, and a source build has no Last.fm
  keys, so scrobbling did not work. Now the release binary is installed.

## 3.0

Setup
- `utuimp setup` shows a menu of sections (language, RP,
  services, visualizer, interface).

Visualizer
- `V` (or `:viz`) opens a fullscreen visualizer. Six scenes, cycled
  with `tab` and remembered: tunnel, terrain, nebula, spectrum, orb,
  ribbons. It reacts to the real sound (spectrum and waveform) on Linux, 
  for every service, and to a
  beat of its own elsewhere. Playback keys keep working inside.
- Strength, against motion sickness: `[` and `]` (or setup) turn the
  motion down or up in steps of 25%, 0–200%: the shake, the push and turn
  of beats, the speed of flying and scrolling, the zoom of the trails.
  Below 50% nothing shakes.
- A backdrop behind every scene (`b`, or setup): still, twinkling "stars"
  (something steady for the eye) or "sparks" drifting up as behind the
  ribbons. They show only where the scene is dark.
- Its colours are the cover's, in every view (the cover is fetched even
  when it isn't shown). A cover of one hue gives shades of it, a black-and-white one gives greys, a song with no cover goes back to the default colours.
- It shows the sound being heard, not the sound being decoded - when each
 sample comes out of the speakers is worked out from the player's clock. The player hands the visualizer the sound a little before it is played, so a hit is found ahead of time and shown at the moment it sounds.
- `i` inside it shows some info - where the sound comes from, how far
  ahead of the speakers it is read, and how many hits a second it shows.
- In every scene the parts listen to different voices of the sound (bass,
  body, vocal, bright, air).
- On Linux utuimp plays the sound through the same player as the other services.
  Pause is instant, and the visualizer is as exact as elsewhere.
  `UTUIMP_SPOTIFY_OUTPUT=librespot` goes back to librespot playing it.
- The service tabs are hidden while it is open, and can't be switched.
- It fills the terminal to the window's edge: the margin some terminals
  keep around the text (kitty's `window_padding_width`) takes the colour
  of the picture's edges while it is open, instead of framing it.
- On the Linux console it sets all 16 colours to tones of the cover's hues
  and mixes them with `░▒▓`, so the picture stays smooth there too; the
  palette comes back on `esc`.

Everything
- A panel that opens brings its text up out of the background row after
  row, and a new song's text comes up from
  the frame it is shown. `ui_motion: off` (also in setup)
  turns it off. Not on the Linux console.
- Visualizer: `{` and `}` change the picture's brightness (10–150%). It's visualizer_brightness` in the config.
- Install: one script for Linux and macOS (`scripts/install.sh`) and one
  for Windows (`scripts/install.ps1`): they find what is missing, offer to
  install it with the system's package manager, then install a release or
  build from source. `make build` runs the same check.
- Every service that signs in through the browser now has
  utuimp's own page.
- Setup: Enter at the background question keeps the marked choice.
- With the `system` theme the terminal is asked for its 16 palette colours
  at start (OSC 4), so fades blend its real colours instead of xterm's.
- Cover view: the status rows always take as many rows as the tallest set
  of hints, so the cover and the song no longer jump (and the cover no
  longer comes and goes) on esc, q or a step into the queue.
- A service's tab keeps the note's place, so the tabs don't change size
  when music starts or stops.

Discord
- A pause hides the status, and play brings it back at the song's current
  playtime.

Links
- A share link of any service (a song, album, playlist or artist from
  Spotify, Apple Music, Yandex Music, YouTube Music or SoundCloud) pasted
  into search (`/`) opens in that service's tab, parses and tries to find it. `:link <url>` offers to play it or add it to the queue.

Playlists
- In the library's playlists `c` makes one, `r` renames it and `d d`
  deletes it; `d d` in an open playlist deletes a song.
- `P` (or "Add to playlist…" in the song menu) adds the selected or playing
  song to a playlist.

Library
- Where the service keeps saved albums and followed artists (Spotify,
  Yandex Music, YouTube Music, SoundCloud), Albums and Artists show those
  instead of grouping the liked songs. An album opens with all its songs,
  an artist with their albums.
- The song menu saves the song's album and follows its artist there, and
  on Apple Music adds the song to the library.

Yandex Music
- My Wave learns from utuimp like from the official apps - it is told when
  a song starts, ends and is skipped.

Cover colours
- The song's title, artist, time and progress line take the cover's
  colours on every service. When the service sends
  no palette, the colours are picked from the cover itself.

Cover view
- `esc` between the lyrics and the queue no longer moves the cover by a row.
- `y` hides the lyrics: a bigger cover, with the song, controls and volume
  on the right. `esc` shows the queue there.
- The view (and hidden lyrics) is one for all tabs and is remembered.
- A failed translation or if the current lyrics are the same language shows "no translation".
- Playing a song (from a list, search, history) or My Wave goes to the
  lyrics, or to the song when they're hidden.

Feed and search
- An album or playlist opens as a list of its songs first.

Start screen
- While "♪ utuimp" types itself out and the services connect, the
  visualizer's ribbons dance behind it: they come in with a splash and sway to a beat of their own as long as loading takes.
- On the Linux console the logo stands alone.
- Set `"intro": false` in the config to turn it off.

Desktop
- A new icon: a frame of the ribbons scene drawn by the visualizer itself,
  pixel for pixel as a terminal shows it, with a note (centred on its
  stem) in the middle of the ribbons. `UTUIMP_ICON=1 go test ./internal/tui -run TestRenderIcon` draws
  it again.
- The About panel shows a small copy of the icon on top (when there's
  room), says "keyboard first", and credits vibez to simonepelosi.
- The install script and the Flatpak manifest get their icon: the
  `logo.png` both pointed to was missing.
- `make build` adds utuimp to the application list too (launcher and
  icon), not only `make install` and the first start.

Linux console
- On the console without a desktop utuimp sets the
  console's palette to the theme's colours (and restores it on exit),
  drops the italic, underline and bold the console shows as odd colours,
  and draws only glyphs the console font has.
- Covers show there too, dithered to the console's colours; six palette
  entries follow each cover. `UTUIMP_CONSOLE=0` turns all this off, `=1`
  forces it on.

Local files
- Seeking works in MP3s that start with an ID3 tag.
- Covers embedded in the files' tags are shown.

Player
- The album line no longer shows the codec and the catalog's
  formats. And now `:quality` without
  an argument shows them.
- `ctrl+c` quits when pressed twice within two seconds; the first press
  only says so.
- The explicit mark has an extra spacing before the "—" in terminals that draw it
  two cells wide.
- The progress bar shows the elapsed time on its left and the song's length
  on its right, instead of in the album line.

Volume
- Spotify uses the same linear volume scale as the other services and no
  longer normalises loudness, so a level sounds the same on every tab.

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
