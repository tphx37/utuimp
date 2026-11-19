# Changelog

## 4.0.3

- Improved mini menus (such as `i` menu) animations.
- Read YouTube Music cover thumbnails and decode WebP artwork.
- Show Spotify playlists after recently played items, in the order returned by Spotify.
- Give yt-dlp a temporary cookies copy to keep the exported file intact.
- Read YouTube Music liked songs and playlists directly with exported cookies instead of the YouTube video library.
- Show errors when Spotify feed cannot load saved albums or followed artists.
- Accept cookies.txt paths including ~/ in YouTube cookies settings.
- Retry loading the feed after a service finishes connecting.
- Include the playback command in Spotify error messages to make restrictions easier to diagnose.
- In cover view now q opens the queue, Esc returns to the lyrics, h opens history.
- Shortcuts also work on the corresponding keys of the Cyrrilic keyboard layout.
- Spotify volume is applied locally when utuimp plays the audio, avoiding volume API errors.

## 4.0.2

- Added an optional pure black AMOLED background in Interface settings.
- Smoothed word-by-word lyric highlighting, including slow passages.
- Smoothed artwork, title, artist and album transitions when changing tracks,
  including rapid changes and metadata that arrives too late.
- Kept the progress and volume bars and playback buttons visible when tracks
  change, without restarting their appearance animation.
- Made the cover fade in when returning to a view that has room for it.
- Fixed long titles scrolling unevenly or being cut off in narrow windows.
- Added an opaque background to the song menu so player graphics underneath it
  do not overlap its text.
- Added [SECURITY.md](SECURITY.md).

## 4.0.1

- The macOS and Windows audio outputs now share device-clock PCM with the
  visualizer. Spotify also uses native PCM output on those systems, so
  beats, spectrum and waveform follow the sound rather than a generated rhythm.
- Replaced the black-and-white README card with word-by-word lyrics.
- Added a signed pacman package repository and uninstall
  commands for each platform and package manager.

- Removed downloading/running another project's release to obtain Apple
  Music credentials, including the settings option and cache fallback.
  Own MusicKit tokens and local Music media on macOS remain supported.
- Added Arch release packages, AUR recipes, a Homebrew tap and generated
  winget manifests, with installation and removal instructions.
- Reduced pixel lyric rendering work during word sync: reuse static scene
  text, calculate highlight colors once per column, and avoid resending
  unchanged lyric lines when playback is between updates.

## 4.0

Follow-up fixes
- The start screen keeps its pixel ribbons with Pixel interface both on
  and off. With interface pictures off, it uses solid terminal half-blocks -
  the ribbons stay vivid, the status text is easier to read, and the intro
  no longer shows through to or overlaps the screen behind it.
- On macOS, compatible terminals receive graphics through private temporary
  files when POSIX shared memory is unavailable. The pixel interface and
  picture visualizer no longer depend on Linux's `/dev/shm`.
- If Apple Music sign-in is needed after the start screen has already
  finished, utuimp brings up the visible waiting screen again. Sign-in
  progress and the fallback link are shown inside the interface instead
  of being overlapped.

Interface
 - In kitty and Ghostty the interface is drawn in real pixels under the
  text, in the song's colours, and moves a little with the music. Nothing
made of cells moves. In Settings under Interface and Lines, "simple"
keeps the plain cells, and animation "off" does too.
  - The progress line is fine strands that sway and settle into one line
    when the music stops. A new song sends a wave along it, and waves keep
    coming while a song loads. The cover view's volume line stirs when the
    volume changes.
  - The frames of the panels are lines held at their round corners that
    sway between them, the colours run round the panel in focus, and a new
    frame draws itself in. The rules under headings are soft lines.
  - The cover is the picture itself instead of coloured cells, with a
    light behind it that breathes with the bass.
  - The mark of the chosen row flows from row to row.
  - The cover view's lyrics are drawn large, in the system's bold sans,
    the way a fullscreen player shows them, down the whole column. A line
    change glides the text up, the line being sung brightens, and with
    word times the light runs along it with the voice. (Needs fontconfig,
    without it they stay in the terminal's letters.)
  - The player's own words are drawn too, the title large and bold, the
    artist in the song's colour, the album, the times and the volume in
    the system's sans, and a small loudspeaker in place of the note sign,
    with more waves the louder it is.
  - The boxed view's Now Playing panel is laid out as a player's bar, the
    cover small on the left with the song beside it, the buttons in the
    middle, the volume on the right, the progress line under them from
    the cover to the panel's edge.
  - The drawn words look the same in any language, letters the font lacks
    come from a font of its own family where the system has one (Noto
    Sans CJK for Noto Sans), upright and as heavy.
  - The player's buttons (repeat, shuffle, play or pause, favourite) are
    drawn in the same fine strokes, larger than a character, one that
    changes springs for a moment, and a ring round the play button
    breathes with the drum.
  - Nothing changes at a stroke. Changing the view moves the cover to its
    new place, growing or shrinking on the way, pictures come and go
    softly, new words arrive in a quick sweep and old ones are swept
    away (the key hints at the bottom too), each letter whole and in
    its own colour, a frame takes and loses the colour of focus
    gradually.
  - The bass drum shows, the frames, the light behind the cover and the
    mark of the chosen row brighten for a moment on each beat, in place.
  - The start screen's ribbons are a picture too, smooth instead of made
    of cells, with a soft round opening as the interface comes in.
  - The lines answer the hand. Enter presses a frame in and it springs
    back, esc lets it out, moving through a list sloshes the frame that
    way and sends a swell down its sides, tab and the arrows send one
    along its top and bottom, a new song sends one all the way round.
    Play and pause ripple out of the song's point on the progress line, a
    seek sends a wave the way it goes, and a press squeezes the mark of
    the chosen row. It is all over in under a second.
  - A held key is one steady push, not a shake, the frame leans the way
    of the move for as long as the key is down and springs back once when
    it is let go, words, a new cover and the wave of a new song under way
    run on instead of beginning again with every repeat.
  - The progress line's light and the point where the song is run a
    little past the line's ends instead of being cut off there.
  - A new cover comes up softly instead of replacing the old one at once,
    and on leaving the visualizer the frames draw themselves in again.
  - In the boxed view's player bar the volume line and the progress line
    no longer swap places (the progress line showed the volume, and the
    other way round).
  - A frame keeps the colour of focus while the theme is being changed
    (a key held on Theme in the settings), instead of flashing plain.
  - As the start screen gives way, the interface's pictures (the cover,
    frames, the lyrics) come as its opening reaches them, instead of all
    at once after it, with an empty hole where they belong until then,
    the logo's letters stay whole until the interface is over them.
  - The start screen runs at the screen's own sixty frames a second, each
    frame is made and sent on one tick, and a frame was now and then
    shown twice.
  - Everything that moves runs at sixty frames a second, the lines (thirty
    before, while nothing travelled), the words' effects (thirty), the
    lyrics' and the progress line's clock (twenty-five), the visualizer
    (fifty). They keep to one steady beat instead of waiting a frame after
    each one, which made some 57 a second, and while the lines' clock runs
    the others draw on its frames instead of drawing the screen again.
  - Light on the machine, at the same sixty frames a second and looking
    the same, about a third of the processor it took while a song plays,
    a quarter of it paused, and half the terminal's. The screen is laid
    out anew only when something in its cells changed, not on every frame
    of the lines (nor a second time for every picture sent), with nothing
    playing and nothing moving the lines' clock slows to a watch, and a
    key brings it back at once. The pictures sent are smaller, thin lines
    without the clear rows round them, the light behind the cover at half
    its pixels (it is soft through and through), the lyrics in bands of
    which only those that changed go again. Lines and glows are drawn with
    a few e^x a column instead of one a pixel.
  - A frame that changes its size or place (switching views with a panel
    open, a panel growing as the key hints under it shrink) goes over to
    its new place, its sides sliding and stretching by the pixel and a
    touch past before they settle, instead of going and drawing itself in
    anew. Each new frame goes over from the old one that shared most of
    its cells.
  - More of it changes softly. Words that stay but change colour (the
    chosen row of a list, a title taking focus, a whole theme) go from the
    one colour to the other in a fifth of a second. The progress and
    volume lines run to where they should be after a seek, another song or
    a step of the volume, instead of jumping. Another song's title, artist
    and album melt in as the last one's melt away, the times and the
    volume staying as they are (and they are no longer typed out letter by
    letter under that). The lyrics melt from one song's into "fetching
    lyrics…" and into the next song's, and as the translation is turned
    on or off.
  - Settings, Graphics, each part of the pixel interface has its own item
    besides "Pixel interface" (which turns them all off), the cover
    (picture or characters), frames and lines (moving, still or
    characters), text (drawn or the terminal's), buttons (strokes or
    characters) and the light behind the cover. In the config, `ui_cover`,
    `ui_lines`, `ui_text`, `ui_buttons`, `ui_light`.
  - With the lyrics hidden, the song beside the cover is set large, the
    title twice its size, the artist, the album and the times larger
    too, in proportion to the cover instead of small beside it.
  - The cover keeps its shape on its way to another place (another view,
    the lyrics opened or closed), kitty stretches a picture from the
    pixel it starts at to the edge of the last cell, and it was sent as
    if from the cell's edge, so it grew and shrank by up to a cell with
    every frame as it moved.
  - On changing the view the cover travels more slowly and evenly, the
    lyrics and the song's words come once it has all but got to its
    place, and the old ones go at once, so the terminal is not taking in
    their pictures while it moves the cover (it showed the cover's way in
    jumps).
  - Frames going over to a new place and size no longer break, each keeps
    its own sides while it is there (a frame that came or went before it
    handed its sides, with their colour and picture, to another), and none
    goes a touch past where it goes and back.
  - With a key held (a list scrolled) the words no longer flash in another
    colour and back, they change at once while it is down. Play and pause
    pressed again and again ripple the progress line on every press.
  - Opening the visualizer no longer jumps the interface up first (the tab
    row went away under it).
  - The lyrics glide from line to line more slowly and evenly, gathering
    speed and settling instead of leaping.

Visualizer
- In kitty and Ghostty the visualizer is drawn in real pixels (the kitty
  graphics protocol) instead of two coloured halves a cell. Settings,
  section Graphics, on/off/auto and how fine the picture is.
- Frames keep their pace however long one takes to draw.
- The picture is drawn with several times more pixels than before (as
  many as the machine draws in time).
- The picture no longer stays coarse after a busy moment, its detail comes
  back once frames are quick again, and is worked out anew when the window
  changes size.
- It opens out of the interface's lines, for a moment every scene is the
  ribbons, thin and flat, which swell and give way to the scene.
- It opens over the interface, which stays where it is under it, the
  picture grows out of the middle of the screen and covers it, instead of
  the screen going black at once and the picture coming out of the dark.
- It closes the way it opens, the picture goes back into the middle of
  the screen while the interface comes back under it. A new scene melts
  out of the last one, another song's colours, the brightness, the
  strength and the background come over in a moment instead of at once.
- A frame takes about a third less to draw (the tone curve and the glows
  read off tables, the tunnel's squeeze without a sine), so the picture
  keeps its full detail at sixty frames a second.
- The backdrop's stars and sparks are soft round points that move by the
  pixel and trail along their way, under the scene, instead of square dots
  stepping from one place to the next over it, the scenes' particles trail
  the same way.
- 60 frames a second instead of 30. The scenes draw faster (the tunnel by
  about a third, the ribbons twice as fast), and each scene remembers the
  detail it kept up with, so it opens at that detail instead of at the
  finest, stuttering for a second or two until it found out again, when a
  frame is far too slow the detail goes straight down to what keeps up.

Settings
- `,` (or `:settings`) opens the settings inside utuimp, services and their
  keys, language, theme, animation, start screen, visualizer, Discord
  status, Last.fm, Apple Music quality, lyrics translation, library.
- More in the settings, all taking effect at once and in every tab
  - Interface, the view (boxed or with the cover), the lyrics in the cover
    view, the album art in the boxed view, the key hints, and the frame
    rate (auto, 60, or 30 for the battery). Changing the view there leaves
    the settings open.
  - Playback, a new section (`utuimp setup playback`), how far `←` `→` seek
    (5, 10, 15, 30 s, the key hints say it) and `+` `-` turn the volume
    (2, 5, 10%), and song notifications, the song, its artist and album and
    its cover on the desktop when the next one starts, always or only while
    you are in another window (the terminal says when it is not in use).
    One replaces the last. Offered where `notify-send` or `osascript` is.
  - Lyrics, the size of the cover view's lyrics (small, medium, large).
  - Visualizer, the scene it opens with, and its brightness.
- Turning a service on or off shows or hides its tab at once. The theme,
  language, animation, visualizer and Discord status also apply without a
  restart. What does need one says so, and `R` restarts utuimp in place.
- Last.fm is connected from the settings (no `utuimp lastfm login` needed).
- The key hints at the bottom show `,` for the settings.
- A song's cover melts into the next one's, the old cover stays under the
  new one as it comes up, instead of going and the new one coming up out
  of nothing. Nothing more is sent for it.
- `H` hides the key hints at the bottom, they fade away from the top row
  down, and come back from the bottom up with `H` again. Once they have
  gone their rows go to the rest of the screen (on a small terminal they
  take several), the panels grow into them and the cover view spreads
  down, a quiet "H show hints" is left on one row at the bottom. utuimp
  remembers it.
- A second logo, black and white, its ribbons drawn as the interface's
  fine strands, `assets/utuimp_logo_lines.*` (and a square copy). The old
  logo stays. `scripts/art/` draws it and the social previews.
- A black and white theme, `mono` (Settings, Theme, black and white), the
  interface in greys, white for what matters most. What takes the song's
  colours (the progress line, the title and artist, the lines, the
  visualizer) keeps them, as with every theme. A theme in greys gives the
  start screen grey ribbons, not tinted ones.
- Animation off says what it does, the interface in characters (its pixel
  lines and pictures move). The Graphics items it leaves nothing to set
  for can't be changed then, and say why.
- The first start opens the settings instead of asking questions in the
  terminal. `utuimp setup [section]` now opens utuimp on its settings, the
  question-by-question setup is gone.

Speed
- The start screen shows once, on the tab utuimp opens on. A tab opened
  while it was up danced a start screen of its own over the others until
  its service was ready, for good if it never was, and kept the processor
  busy all the while.
- Animations keep a pace that fits the screen, a frame lasts a whole number
  of its refreshes, or what moves jerks (60 frames a second showed for two
  refreshes, then three, on a 144 Hz screen). 72 frames a second at 144 Hz,
  60 at 60, 120 and 240 Hz, utuimp asks Hyprland, sway or X11.
  `UTUIMP_FPS` sets it.
- A frame goes to the terminal as soon as it is drawn, Bubble Tea wrote to
  it on a clock of its own, sixty times a second, that beat against the
  animations' and sent two frames together now and then.
- No more stall on the first cover view and on a view change, the fonts the
  words are drawn in are found and read once, in the background, and the
  cover's pixels are kept instead of read again for every size.
- What only sways, breathes or fades (the frames with the bass, the light
  behind the cover, the buttons with the drum, a picture coming or going)
  is drawn every other frame, the light behind the cover is not sent while
  the cover travels and none of it shows. Half the pictures for the
  terminal to take in, for no step that shows.
- The cover view's lyrics go to the terminal a line a picture, a line
  that glides to its new place is moved there, not drawn and sent again.
  Only what changes how a line looks is (the light running along the line
  sung, a line taking or losing its light, a line in the edges' melting),
  every other frame. A line change sent some 70 MB a second, now a tenth.
- The start screen is sharper, its ribbons are drawn at about three times
  the pixels on a full HD screen (as many as keep a frame at the same cost
  on any screen), their thin threads no longer come out as a string of
  beads at a coarse size (the visualizer's ribbons neither), and their
  colours have no steps. It takes more processor than it did, for the
  second or two it shows (some 1.3 cores of a laptop's against 0.6).
- With the pixel interface the text's effects (a panel's words coming up,
  a new song's) had stopped showing, they show again.
- Frames come on a steady beat, a frame's clock waits for the moment it
  is due, not for a span from when Bubble Tea got round to starting it
  (after a heavy frame, a little later). Every animation keeps its pace
  within a couple of milliseconds now, at 60, 72 and 90 frames a second
  alike.
- The visualizer keeps up at any pace, its detail goes to what fits in
  85% of a frame (95% left the rest of the frame no room and it missed
  frames), down in a few frames when a scene is too heavy, not over half a
  second, a scene not shown yet starts at the detail the last one had,
  weighed by how heavy each is, tab draws the next scene on the next frame
  only, not twice. Closing, it draws its scene every other frame and the
  interface coming back under it on the others.
- No missed frame when a song changes, the new cover's pixels are read out
  as it loads. The lyrics' karaoke clock stops while the visualizer covers
  them.
- A frame's pictures go to the terminal the moment the frame is drawn,
  written straight to it (each write whole, one at a time with Bubble
  Tea's), Bubble Tea sent them on its own clock, up to 8 ms late, by a
  different amount every frame against the animations' pace.
- The light behind the cover is drawn at a third of the screen's pixels
  (a half before), as the terminal shows it, stretched, it differs from it
  drawn at full size by at most 3 of 255 where it shows, as before, for
  less than half of what is sent, it has its colour also where none of it
  shows, so the edges of it no longer darken as the terminal stretches it.
- What remains is the terminal's, on a 144 Hz screen kitty shows even a
  lone picture moving 72 times a second for two refreshes, then three,
  whatever its settings.
- `UTUIMP_FRAMES=<file>` notes every frame of every animation, for a bug
  report about stutter, `UTUIMP_CPUPROFILE=<file>` records where the
  processor's time goes, and a slow frame in `UTUIMP_LINES_LOG` says what
  took the time.

Local files
- Lyrics from beside the song, `song.lrc` (in time with the song) or
  `song.txt` (plain) next to `song.mp3`. They are not counted as files the
  scan skipped.
- The volume line showed 0 and a muted speaker on Linux while the song
  played, and `+`/`-` seemed to do nothing, the player never said its
  volume. It does now.



Signing in
- A sign-in page opens as a new tab of the browser already running, not a
  window of another browser (the one the system names is not always the
  one in use), and the browser no longer closes with the terminal.
Apple Music
- utuimp says plainly that its Apple Music developer token is borrowed from
  vibez. `utuimp setup services` asks before using it (or takes your own
  MusicKit key instead) and saves the answer as `apple_token_source`. A
  token that is already saved keeps working, a new one is only fetched from
  vibez after you agree.
- The About panel and `utuimp auth status` show whose token is in use and
  how long it has left.

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
