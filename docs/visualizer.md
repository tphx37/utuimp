# Visualizer

[← docs](README.md)

`V` (or `:viz`) turns the whole terminal into a visualizer. `esc` (or `V`)
leaves it. Playback keys keep working inside.

Where it is drawn as a picture it opens out of the middle of the screen over
the interface, which stays where it is under it, and closes back into the
middle the same way. A new scene melts out of the last one; another song's
colours, the brightness, the strength and the background come over in a
moment instead of at once.

## Scenes

Six scenes: **tunnel**, **terrain**, **nebula**, **spectrum**, **orb** and
**ribbons**. `tab` goes to the next one (`shift+tab` back), and the last one
is remembered.

In every scene the parts listen to different voices of the sound: bass,
body, vocal, bright and air.

## Keys

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | next / previous scene |
| `[` / `]` | strength down / up |
| `{` / `}` | brightness down / up |
| `b` | background: none → stars → sparks |
| `i` | info: where the sound comes from, how far ahead it is read, hits a second |
| `esc` | back |

## Strength

If the motion makes you dizzy, turn it down. `[` and `]` change the strength
in steps of 25%, from 0 to 200%: the shake, the push and turn on beats, the
speed of flying and scrolling, the zoom of the trails. Below 50% nothing
shakes.

Also in `utuimp setup visualizer`, or `visualizer_strength` in the
[config](configuration.md#config-file).

## Brightness

`{` and `}` change the picture's brightness, 10-150%.
`visualizer_brightness` in the config.

## Background

A backdrop behind every scene, shown only where the scene is dark:

- **stars**: still, slightly twinkling. Something steady for the eye.
- **sparks**: drifting up, as behind the ribbons.
- none.

`b` cycles them. In the config it's `visualizer_background`.

## Real pixels

In kitty and Ghostty the visualizer is drawn as a picture (the kitty
graphics protocol) instead of coloured cells: several times the detail,
with no cell grid showing. Everywhere else, in tmux and over ssh it is
drawn in cells as before.

Settings (`,`), section Graphics:

- **Visualizer as a picture**: auto (the default), on (try in a terminal
  utuimp doesn't know) or off. `graphics` in the config.
- **Visualizer detail**: normal or fine (several times the pixels).
  `visualizer_detail` in the config.

It runs at sixty frames a second. The detail is as fine as the setting
allows and the machine draws in time: if frames take too long the picture
gets coarser by itself, and finer again once there is room. Each scene
remembers the detail it kept up with, so it opens at it instead of
stuttering for a moment to find out again. The scenes are not equally
heavy: the tunnel is the dearest. The visualizer is the one part of utuimp
that keeps several processor cores busy (normal detail in a large window:
three or four); the rest of the interface takes a fraction of one. For less,
choose cells (Visualizer as a picture: off) or a smaller window.

## Colours

The colours come from the cover of the song that is playing. A cover of one
hue gives shades of it, a black-and-white one gives greys, a song with no
cover goes back to the default colours.

The visualizer fills the terminal to the window's edge: the margin some
terminals keep around the text (kitty's `window_padding_width`) takes the
colour of the picture's edges while it is open.

On the Linux console it sets the 16 console colours to tones of the cover
and mixes them with `░▒▓`. The palette comes back on `esc`.

## The sound it sees

On Linux it reacts to the real sound of every service (spectrum and
waveform). It shows the sound being heard, not the sound being decoded. The
player hands it the sound a little ahead of time, so a hit is found early
and shown at the moment it sounds.

On macOS and Windows it follows a beat of its own.
