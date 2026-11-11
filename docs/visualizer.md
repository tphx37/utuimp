# Visualizer

[← docs](README.md)

`V` (or `:viz`) turns the whole terminal into a visualizer. `esc` leaves it.
Playback keys keep working inside.

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
