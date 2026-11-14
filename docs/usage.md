# Usage

[← docs](README.md)

- [Tabs](#tabs)
- [Keys](#keys)
- [Views](#views)
- [The look](#the-look)
- [Queue](#queue)
- [Search](#search)
- [Library and playlists](#library-and-playlists)
- [Favorites](#favorites)
- [Lyrics](#lyrics)
- [Radio and My Wave](#radio-and-my-wave)
- [Song menu](#song-menu)
- [Equalizer](#equalizer)
- [Volume](#volume)
- [Commands](#commands)

## Tabs

Every service you turned on in `utuimp setup` is a tab. Number keys switch
between them in the order they are shown (`1` is the first tab, `2` the
second…). Local files are always last. Keys can be rebound with
`service_keys`, see [Configuration](configuration.md#config-file). For
example `` {"`": "local"} `` puts local files on the key left of `1`
(`` ` `` and `ё` count as the same key, so it works on a Russian layout
too).

The music keeps playing when you switch tabs: the tab bar marks the service
that is playing, and the playback keys always control it.

## Keys

| Key | Action |
|-----|--------|
| `space` | play / pause |
| `n` / `p` | next / previous song |
| `←` / `→` | rewind / forward 10 seconds (Settings, Playback, Seek step) |
| `+` / `-` | volume |
| `f` | add to favorites (press twice to remove) |
| `s` | shuffle |
| `r` | repeat: off → all → one |
| `R` | radio from the current song (Yandex Music: My Wave by this song) |
| `v` | switch between the boxed view and the cover view |
| `V` | fullscreen [visualizer](visualizer.md) |
| `/` | search |
| `l` | your library (`L` jumps back to its top) |
| `q` | recently played |
| `y` | lyrics |
| `F` | recommendations (Yandex Music: My Wave) |
| `e` | equalizer |
| `i` | song menu: go to the album or an artist, copy the link |
| `P` | add the selected (or playing) song to a playlist |
| `,` | settings (also `:settings`) |
| `H` | hide the key hints at the bottom: they fade away from the top row down, then the panels (or the cover view) grow into their rows; a quiet "H show hints" stays on one row at the bottom. `H` again brings them back. Remembered |
| `?` | about |
| `:` | command line, see [Commands](#commands) |
| `esc` | back |
| `ctrl+c` twice | quit |

A panel key opens its panel, pressing it again closes it. `j`/`k` work
everywhere the arrows do.

## Views

**Boxed view.** The default: the song, the queue and the open panel each
sit in a frame.

**Cover view.** `v` switches to a frameless fullscreen player: the cover on
one side, the lyrics fading in and out on the other. `y` hides the lyrics,
and the song and its controls take their place next to the cover. `v` again
returns to the boxed view.

utuimp remembers the view, and whether the lyrics are hidden, for every tab.
`"layout": "cover"` in the config starts in the cover view.

**Album art in the boxed view.** `:art` shows the cover and the song info
instead of the progress bar. It needs a terminal with at least 256 colours.

**Visualizer.** `V`, see [Visualizer](visualizer.md).

## The look

In kitty and Ghostty the interface is drawn in real pixels under the text,
in the colours of the song's cover; everywhere else (and in tmux, over ssh,
on the Linux console) it is the terminal's characters.

- **Frames and lines.** The panels' frames, the rules under headings, the
  progress and volume lines and the mark of the chosen row are fine lines.
  They sway a little with the music, flash on the drum and answer keys:
  `enter` presses a frame in, moving through a list sloshes it, a new song
  sends a wave round it. A frame that changes its size or place goes over
  to the new one instead of being drawn again.
- **The cover** is the picture itself, with round corners and a light
  behind it that breathes with the bass.
- **Text.** The cover view's lyrics are drawn large in the system's font
  (it needs fontconfig), the line being sung lit, word by word where the
  lyrics have word times. The player's title, artist, album, times and
  volume are drawn the same way.
- **Buttons.** Repeat, shuffle, play and favourite are drawn in fine
  strokes and spring when they change.
- **Nothing changes at a stroke.** New words come in a quick sweep and old
  ones are swept away, a colour goes over to another, the cover moves to
  its new place, another song's words melt in, the visualizer opens out of
  the middle of the screen and closes back into it. All of it at sixty
  frames a second; with nothing playing and nothing moving, utuimp all but
  stops drawing.

Settings (`,`), section **Graphics**, sets each part on its own:

| Item | Choices |
|---|---|
| Pixel interface | on, off (all of it in characters) |
| Cover | picture, characters |
| Frames and lines | moving, still (the same lines, standing still), characters |
| Text | drawn, terminal (the terminal's letters) |
| Buttons | strokes, characters |
| Light behind the cover | on, off |

Animation off (section Interface) is the interface in characters too: its
pictures and lines move. For lines that stand still, choose Frames and
lines: still. In the config these are `ui_style`, `ui_cover`, `ui_lines`,
`ui_text`, `ui_buttons` and `ui_light`, see
[Configuration](configuration.md#look).

## Queue

The queue is on the main screen. Move into it with the arrow keys, then:

| Key | Action |
|-----|--------|
| `enter` | play from this song |
| `d` / `x` / `delete` | remove it |
| `K` / `J` | move it up / down |
| `shift+tab` | move it to play next |
| `p` | add it to a playlist |
| `R` | radio from it |
| `c` | clear the queue |
| `s` | save the queue as a playlist (`:save <name>`) |
| `esc` | leave the queue |

## Search

`/` opens search. Type, and the results come as you type: songs, albums,
artists and playlists.

| Key | Action |
|-----|--------|
| `↑` / `↓` | choose |
| `enter` | play the song, open the album, artist or playlist |
| `tab` / `ctrl+q` | play next |
| `shift+tab` | add to the end of the queue |
| `ctrl+p` | add to a playlist |
| `ctrl+r` | radio from the song |
| `ctrl+w` / `ctrl+u` | delete a word / everything before the cursor |
| `esc` | back (out of an artist, then out of search) |

**Links.** Paste a Spotify, Apple Music, Yandex Music, YouTube or SoundCloud
link into search (or `:link <url>`) to play it or add it to the queue. A
link to another service opens on that service's tab.

## Library and playlists

`l` opens your library: liked songs, playlists, and on services that have
them, saved albums and followed artists. `enter` opens a playlist or album
as a list of its songs. In it, `enter` plays from a song, `tab` plays it
next, `shift+tab` adds it to the end, `ctrl+q` puts the whole list next.

In the list of playlists:

| Key | Action |
|-----|--------|
| `c` | new playlist |
| `r` | rename (needs `library_edit`) |
| `d` `d` | delete (needs `library_delete`) |

In an open playlist, `d` `d` takes the song out of it (also needs
`library_delete`).

Deleting and renaming are off by default so nothing in your account goes
away by accident. Turn them on in `utuimp setup library`, or with
`library_delete` / `library_edit` in the [config](configuration.md#config-file).
Adding is always allowed.

YouTube Music and SoundCloud (without the API) have no account library:
utuimp keeps favorites and playlists itself, in
`~/.config/utuimp/library-youtube.json` and
`~/.config/utuimp/library-soundcloud.json`.

## Favorites

`f` adds the song to your favorites (likes) on its service. Removing is easy
to do by accident, so it takes a second `f` right after the first. Taking a
like back counts as deleting from the library and needs `library_delete`.

## Lyrics

`y` shows the lyrics: word by word when available, line by line otherwise,
plain text as the last resort.

| Key | Action |
|-----|--------|
| `t` | show a translation next to each line |
| `T` | choose the translation language |
| `c` | choose where the lyrics come from, if the text is wrong |

Sources: the service itself (Yandex Music; for local files an `.lrc` or
`.txt` beside the song, see [Services](services.md#local-files)), NetEase Cloud Music (word sync),
[LRCLIB](https://lrclib.net) (line sync) and [lyrics.ovh](https://lyrics.ovh)
(plain text). The source you pick is remembered for that song.

The translation language is `translate_lang` in the config. If it's unset, it comes
from your system's locale.

## Radio and My Wave

`R` starts a radio from the song that is playing or selected. Whatever was
queued after that song is removed. Press `R` again on the same song to stop
it.

On **Yandex Music** `R` is "My Wave" by this song, and `F` opens the feed,
where **My Wave** clears the queue and plays your personal wave. Each time,
the wave starts fresh. My Wave learns from utuimp the same way it does from
the official apps: from songs started, finished and skipped.

On **Spotify** the radio is built from songs by the same artist, since
Spotify's recommendations aren't open to new apps.

On **YouTube Music** the feed has a mix for each favorite.

## Song menu

`i` opens a menu for the selected or playing song: go to its album or one
of its artists, add it to the queue or a playlist, copy its link.

## Equalizer

`e` opens a 10-band equalizer.

| Key | Action |
|-----|--------|
| `←` / `→` | choose a band |
| `↑` / `↓` | gain |
| `0` | reset the band |
| `r` | reset all |
| `e` | close |

The bands are saved in the config (`eq_bands`).

## Volume

One volume for all service tabs: `+` / `-`, `:vol 50`, `:vol +10`,
`:vol -5`, or `:mute` (again to unmute). It is remembered between runs.

## Commands

Type `:` and then a command. `tab` completes, `↑` / `↓` go through the
suggestions.

| Command | What it does |
|---|---|
| `:vol <0-100\|+n\|-n>` | set, raise or lower the volume |
| `:mute` | mute / unmute |
| `:seek <seconds>` | jump to a position in the song |
| `:save <name>` | save the queue as a playlist |
| `:link <url>` | open a song, album or playlist link |
| `:quality [high\|standard\|256\|64]` | show the audio format, or set the Apple Music bitrate |
| `:art` | album art in the boxed view |
| `:viz` | fullscreen visualizer |
| `:about` | about utuimp |
| `:debug-logs` | what utuimp has been doing (`esc` closes) |
| `:q`, `:quit` | quit |
