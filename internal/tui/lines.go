package tui

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/gfx"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// The interface's lines. In a terminal that shows real pixels (see gfx) the
// progress line is not made of ━ and ─: it is a small picture laid under the
// text, a few fine strands in the song's colours that sway a little with
// the music and settle into one straight line when it stops. The volume
// line of the cover view is drawn the same way, and lies still until it is
// changed. The rules that part a panel's heading from its contents are one
// soft line each, and behind the cover view's lyrics run a few very faint
// ribbons, the visualizer's in a whisper. Nothing made of cells ever moves; only the picture under them does, by fractions of a
// pixel. Everywhere else, and with config.UIStyle "simple", the line is the
// cells it always was.

const (
	// The lines keep the screen's pace (screenFrame) whatever moves. (It
	// was half that at rest, with a faster pace only while something
	// travelled: the music's sway stepped, and a faster pace than the
	// screen's shows nothing more.) A line that is still is not drawn
	// again however often the clock ticks.
	//
	// linesWatch is the pace when nothing has moved for linesQuietAfter
	// frames: enough to notice something starting by itself, and anything
	// a key or the player starts brings the full pace back at once.
	linesWatch = time.Second / 15
	// linesHalfRate: what only sways or breathes (a frame's sides and
	// rules with the bass, the light behind the cover, the buttons with
	// the drum) is drawn every other frame, half of it on each: it moves
	// by a fraction of a pixel a frame, and the screen shows no step, while
	// the terminal has half the pictures to take in. What travels (the
	// cover, a frame on its way, the mark of the chosen row), a hand on a
	// line and the words are drawn every frame.
	linesHalfRate = 2
	// linesHaloScale: the light behind the cover is drawn at a third of
	// the screen's pixels each way.
	linesHaloScale = 3
	// linesCoverAltID: one cover and the next take turns at two pictures
	// (this and the cover strip's own), so that the old one can stay under
	// the new one as it comes up.
	linesCoverAltID = 69
	linesMenuID     = 68
	linesQuietAfter = 30
	linesImageID    = 72 // the first line's picture; the next ones follow
	linesRows       = 3  // the picture's height in cells: the line's row and one each side
	// linesSway is how far a strand may leave the middle, in cell heights,
	// at the loudest: a little.
	linesSway = 0.3
	// linesRuleSway is the same for a rule, which has one row to itself.
	linesRuleSway = 0.14
	linesMaxRules = 8
	// linesBackScale: the ribbons behind the lyrics are drawn at this part
	// of the screen's pixels (they are soft; the terminal smooths them out).
	// linesWaveRows is the height of the waves over and under the lyrics.
	linesWaveRows = 3
	// How long a picture takes to go, to come, and the cover to travel.
	linesGoFor   = 0.2
	linesComeFor = 0.28
	linesMoveFor = 0.6
	// linesCoverRound is the radius of the cover's corners, in cell heights.
	linesCoverRound = 0.45
)

type linesTickMsg struct{ gen int }

func linesTick(gen int, due *time.Time, pace time.Duration) tea.Cmd {
	return frameTick(due, pace, func() tea.Msg { return linesTickMsg{gen} })
}

// lineStrip is one line drawn as a picture.
type lineStrip struct {
	re   *regexp.Regexp // finds it on the screen: its cells are the first group
	sway bool           // its strands move with the music

	// Where it was in the last frame drawn, in cells of the whole
	// terminal; seen is false when it wasn't there.
	seen       bool
	col, row   int
	cols, rows int
	morphed    bool     // previous draw used intermediate geometry
	morph      boxMorph // a side of a frame: the frame's way to its place (see lines_frame.go)
	// The actual cropped picture and placement sent to the terminal. A side
	// can disappear halfway through a morph, when its cell geometry no longer
	// describes that picture.
	framePixels                                              []byte
	frameW, frameH, frameCol, frameRow, frameOffX, frameOffY int
	frac                                                     float64   // how much of it is filled
	want                                                     float64   // how much it should be (the progress and volume lines ease to it)
	born                                                     time.Time // when it came on screen (a rule draws itself in)
	// A side of a frame (see lines_frame.go).
	side, boxW, boxH int
	focused          bool
	mask             []bool
	pad              int     // cells of room before the line, across it
	lit              float64 // how much in focus it looks, 0..1: eased, so its colour never jumps

	img     gfx.Image
	shown   bool // its picture is on screen
	sentAt  time.Time
	sentKey [5]int // what the last picture was drawn for
	rgba    []byte
	pw, ph  int       // the picture's size in pixels
	goneAt  time.Time // when it left the screen: it fades out, not vanishes
	cameAt  time.Time // when it came

	// Its way to another place (see lines_glide.go); how large it is drawn
	// where it goes (0: as usual); the cell it goes to, while it is drawn
	// on its way (glided) before the screen is laid out again.
	glide  stripGlide
	scale  float64
	cell   [2]int
	glided bool
}

// easeFill brings the line's fill to where it should be: at once while it
// only moves on (the song playing) or has just come, and in a quarter of a
// second when it jumps (a seek, another song, a step of the volume).
// It reports whether the fill is still on its way.
func (st *lineStrip) easeFill(dt float64) bool {
	d := st.want - st.frac
	if !st.shown || math.Abs(d) < 0.004 {
		st.frac = st.want
		return false
	}
	st.frac += d * min(1, dt*12)
	return true
}

// The lines: the progress line ("0:39  ━━━━────  4:41"), the cover view's
// volume ("♪  ━━━━────  70%"), the ribbons behind the cover view's lyrics,
// and then the rules.
const (
	stripProgress = iota
	stripVolume
	stripBack
	stripCover                               // the cover itself
	stripHalo                                // the light behind it
	stripSel                                 // the mark of the chosen row
	stripCtl                                 // the player's buttons
	stripLyrics                              // the cover view's lyrics
	stripLabels                              // the player's words (see lines_label.go)
	stripBackLow                             // the wave under the lyrics (stripBack is the one over them)
	stripRule                                // the first of linesMaxRules
	stripFrame   = stripRule + linesMaxRules // the first side of the first frame
	stripCount   = stripFrame + 4*linesMaxBoxes
)

// uiLines is the state of the interface's lines.
type uiLines struct {
	due     time.Time // when the frame is due (see nextFrame)
	mode    gfx.Mode
	gen     int
	ticking bool
	strips  [stripCount]lineStrip

	// Where the ribbons behind the lyrics go, noted while the screen is
	// laid out (backWant: this screen has them).
	backWant                             bool
	backCol, backRow, backCols, backRows int
	// The rules of the last screen, by column and width, and when each
	// came: one that only moved up or down is the same rule.
	rules map[[2]int]time.Time
	boxes map[[4]int]time.Time // the frames, likewise
	// boxGeo is each frame's way to where it is (see boxMorph).
	boxGeo map[[4]int]boxMorph
	// boxSlot is which four strips each frame is drawn in.
	boxSlot map[[4]int]int
	// boxLit is whether each frame was in focus, for when its colour
	// cannot tell (see linesPlaceBoxes).
	boxLit      map[[4]int]bool
	baseFrames  string // layout under a small overlay, before it covers panel borders
	overlayRect fxRect
	// Where the cover goes, noted like the ribbons' place.
	coverWant                                bool
	coverCol, coverRow, coverCols, coverRows int
	coverKey                                 string // what the cover's picture was made of
	// The mark of the chosen row: where it is going (in pixels down the
	// screen) and where it is now. It flows from row to row.
	selTop, selBot float64
	selA, selB     float64
	selCol         int
	selOn          bool

	cellW    int
	cellH    int
	ear      *visualizer // listens to the music; draws nothing
	last     time.Time
	tick     int
	t        float64       // the strands' clock
	bt       float64       // the ribbons' clock: it stops with the music
	amp      float64       // how far they sway now, 0..1, eased
	bass     float64       // the lows alone, eased: the rules follow them
	kick     float64       // the bass drum: up at once, gone in a quarter of a second
	touches  []lineTouch   // what the hand has just done (see lines_touch.go)
	viewTook time.Duration // how long the last screen took to lay out (for UTUIMP_LINES_LOG)
	// viewParts and stripTook: where that time and a frame's went (for
	// UTUIMP_LINES_LOG).
	viewParts string
	stripTook map[int]time.Duration
	frameN    int  // frames drawn (see linesHalfRate)
	haloBlank bool // the light behind the cover was last drawn with nothing of it showing
	// coverOld is the cover a new one replaces, under it while it comes up
	// (see keepOldCover).
	coverOld      gfx.Image
	coverOldOn    bool
	menuImg       gfx.Image
	menuKey       [7]int
	menuShown     bool
	closingFrame  boxMorph
	closingSlot   int
	closingActive bool
	quiet         int  // frames in a row in which nothing moved (see linesWatch)
	changed       bool // something on screen changed in the last frame drawn
	views         int  // screens laid out (for UTUIMP_LINES_LOG)
	why           string
	sentPx        map[string]int // picture bytes sent, by kind (for UTUIMP_LINES_LOG)
	// The lyrics' picture goes to the terminal in bands (see sendBands).
	lyrBands []lyrBand
	// Or a line a picture (see sendLyricLines): the lines as lyr.frame
	// last gave them, whether it drew the whole picture instead, and a
	// line's pixels while they are drawn.
	lyrLines []lyrLine
	lyrItems []lyricItem
	lyrWhole bool
	lyrBuf   []byte
	colBuf   []byte // an upright side of a frame, without its clear columns
	halo     haloShape
	// The player's words as a picture: the labels of the screen being laid
	// out, the fonts, and when the words last changed.
	labels      []lineLabel
	fonts       map[string]*gfx.Font
	mask        []byte
	labelsOf    string
	labelsAt    time.Time
	titleOf     string
	songWordsOf string
	// The last song's words (title, artist, album) as they were last
	// drawn, and those going while another song's come (since labelsAt).
	songWords, goneWords []lineLabel
	// The song's block on its way (see lines_glide.go): each label's way
	// and where it is now, and the screen and cell size they were for.
	// songBig: the screen being laid out has the song beside the cover
	// alone, large (see Model.songBig).
	labelGlides map[int]*labelGlide
	labelCur    []labelGeom
	labelGen    int
	glideSize   [4]int
	songBig     bool
	// The cover view's lyrics as a picture (see lyrics_pic.go): where they
	// go, noted while the screen is laid out, and what draws them.
	lyrWant                          bool
	lyrCol, lyrRow, lyrCols, lyrRows int
	lyr                              lyricsPic
	lyrMoving                        bool
	lyrAt                            time.Time
	lyrSig                           string
	// Where the waves over and under the lyrics are, in pixels down the
	// lyrics' picture (their middles): eased towards the words' ends.
	waveTop, waveLow float64
	waveOK           bool
	ctl              ctlState // the buttons as last drawn, and when each changed
	ctlKnown         bool
	ctlAt            [4]time.Time
	// The cover travels to a new place: where it is now, in pixels (x, y,
	// width, height), where it set out from and when.
	covCur, covFrom, covTo [4]float64
	covAt                  time.Time
	covOK                  bool
	scratch, scaled        []byte
	covUp                  string    // the picture the terminal has for the travelling cover
	coverAt                time.Time // when the cover changed: the new one comes up softly
	coverFade              []byte
	pal                    [3]vcol // their colours, eased towards palTo
	palTo                  [3]vcol
	palURL                 string
	palStarted             bool
	song                   string  // the song the progress line is of
	vol                    float64 // the volume last seen
	volKick                float64 // a change of volume stirs its line for a moment
}

func newUILines() *uiLines {
	l := &uiLines{vol: -1}
	l.strips[stripProgress] = lineStrip{re: regexp.MustCompile(`(?:\d+:\d\d(?::\d\d)?|[\x{e100}-\x{e1ff}] *)  ([━╸─]{4,})  (?:-?\d+:\d\d|[\x{e100}-\x{e1ff}])`), sway: true}
	l.strips[stripVolume] = lineStrip{re: regexp.MustCompile(`(?:♪|[\x{e100}-\x{e1ff}] *)  ([━╸─]{4,})  (?:\d+%|[\x{e100}-\x{e1ff}])`)}
	for i := range l.strips {
		z := -3
		if i >= stripFrame {
			z = -1
		}
		l.strips[i].img = gfx.Image{ID: linesImageID + i, Z: z, Alpha: true}
	}
	l.strips[stripHalo].img.Z = -5 // under the cover and the one it replaces
	l.menuImg = gfx.Image{ID: linesMenuID, Z: -2, Alpha: true}
	return l
}

// linesRuleRe finds a rule: a run of ─ on its own.
var linesRuleRe = regexp.MustCompile(`─{5,}`)

// seen reports whether any line was on the last screen drawn.
func (l *uiLines) seen() bool {
	for i := range l.strips {
		if l.strips[i].seen {
			return true
		}
	}
	return false
}

// unsee forgets where the lines were: the screen shown has none.
func (l *uiLines) unsee() {
	for i := range l.strips {
		l.strips[i].seen = false
		l.strips[i].glided = false
	}
	l.backWant, l.coverWant, l.lyrWant = false, false, false
}

// resetLayout forgets geometry that belongs to the previous terminal size.
func (l *uiLines) resetLayout() {
	l.unsee()
	l.boxes, l.boxGeo, l.boxSlot = nil, nil, nil
	l.coverCol, l.coverRow, l.coverCols, l.coverRows = 0, 0, 0, 0
	l.coverKey, l.covUp = "", ""
}

// linesOn reports whether the interface draws its lines as pictures.
func (m *Model) linesOn() bool {
	return m.lines != nil && (m.lines.mode == gfx.Shm || m.lines.mode == gfx.File) && m.cfg != nil &&
		m.cfg.UIStyle != config.StyleSimple && m.cfg.Motion() != config.MotionOff && !styles.Console
}

// The parts of the interface's picture, each as the settings have it (see
// config.UICover): all of them only where the lines are pictures at all.
func (m *Model) pixCover() bool   { return m.linesOn() && m.cfg.UICover != config.PartCells }
func (m *Model) pixLines() bool   { return m.linesOn() && m.cfg.UILines != config.PartCells }
func (m *Model) pixButtons() bool { return m.linesOn() && m.cfg.UIButtons != config.PartCells }
func (m *Model) pixLight() bool   { return m.pixCover() && m.cfg.UILight != config.LightOff }

// linesCalm reports whether the lines are drawn still: no swaying with
// the music, no beat, no answer to keys.
func (m *Model) linesCalm() bool { return m.cfg != nil && m.cfg.UILines == config.LinesCalm }

// linesBackAt notes where the ribbons behind the lyrics go on the screen
// being laid out: cols×rows cells from (col, row).
func (m *Model) linesBackAt(col, row, cols, rows int) {
	if l := m.lines; l != nil && cols >= 12 && rows >= 4 {
		l.backWant = true
		l.backCol, l.backRow, l.backCols, l.backRows = col, row, cols, rows
	}
}

// linesRuleIn finds a rule in a row of plain text: a run of ─ with nothing
// but blanks or a frame's side next to it. A frame's own top and bottom are
// not rules.
func linesRuleIn(plain string) (from, to int, ok bool) {
	if !strings.Contains(plain, "─────") || strings.ContainsAny(plain, "╭╮╰╯┌┐└┘├┤┬┴┼━╸") {
		return 0, 0, false
	}
	side := func(r rune) bool { return r == ' ' || r == '│' }
	for _, loc := range linesRuleRe.FindAllStringIndex(plain, -1) {
		if loc[0] > 0 {
			if r := []rune(plain[:loc[0]]); !side(r[len(r)-1]) {
				continue
			}
		}
		if loc[1] < len(plain) {
			if r := []rune(plain[loc[1]:]); !side(r[0]) {
				continue
			}
		}
		return loc[0], loc[1], true
	}
	return 0, 0, false
}

// linesPlace finds the lines in the finished screen, notes where they are
// and empties their cells, for the pictures to show through.
func (m *Model) linesPlace(content string) string {
	l := m.lines
	cover, lyr := l.coverWant, l.lyrWant
	l.unsee()
	defer func() { l.songBig = false }() // (said again by every screen that has it)
	if !m.linesOn() {
		return content
	}
	now := time.Now()
	rows := strings.Split(content, "\n")
	changed := false
	empty := func(y int, line, plain string, from, to int) (x0, w int) {
		x0 = ansi.StringWidth(plain[:from])
		w = ansi.StringWidth(plain[from:to])
		rows[y] = ansi.Cut(line, 0, x0) + "\x1b[0m" + strings.Repeat(" ", w) + ansi.Cut(line, x0+w, ansi.StringWidth(plain))
		changed = true
		return x0, w
	}
	// The frames first: what is left of ─ after them is rules and lines.
	hasLabels := false
	for i := range l.labels {
		l.labels[i].found = false
		hasLabels = true
	}
	if hasLabels || l.closingActive || l.menuShown || strings.ContainsAny(content, "╭│▸"+string(linesCoverMark)+string(linesCtlMark)) {
		grid := make([][]rune, len(rows))
		for y, line := range rows {
			if hasLabels || strings.ContainsAny(line, "╭╮╰╯│▸"+string(linesCoverMark)+string(linesCtlMark)) {
				grid[y] = cellGrid(ansi.Strip(line))
			}
		}
		if !m.pixLines() {
			l.boxes, l.boxGeo = nil, nil // the frames stay the characters they are
		} else if m.linesPlaceBoxes(rows, grid, now) {
			changed = true
		}
		// What is left of │ after the frames is the mark of a chosen row.
		if m.linesPlaceMarks(rows, grid, &cover) {
			changed = true
		}
	} else {
		l.boxes, l.boxGeo = nil, nil
		l.selOn = false
	}
	rules := map[[2]int]time.Time{}
	nRules := 0
	for y, line := range rows {
		if !m.pixLines() || !strings.ContainsAny(line, "─━") {
			continue
		}
		plain := ansi.Strip(line)
		found := false
		for i := stripProgress; i <= stripVolume; i++ {
			st := &l.strips[i]
			if st.seen {
				continue
			}
			loc := st.re.FindStringSubmatchIndex(plain)
			if loc == nil {
				continue
			}
			// With the words drawn, both lines start with a label's mark,
			// and both patterns fit either: the kind of label tells them
			// apart (a time before the progress line, the loudspeaker
			// before the volume), whichever comes first on the screen.
			if k := l.markKind(plain[loc[0]:]); k >= 0 && (k == labelTime) != (i == stripProgress) {
				continue
			}
			x0, w := empty(y, line, plain, loc[2], loc[3])
			// A few cells of room each side, where the screen has them: the
			// light and the point are not cut off at the line's ends.
			pad := max(0, min(3, x0, m.width-x0-w))
			st.seen, st.col, st.row, st.cols, st.rows, st.pad = true, x0-pad, y+m.hostRows-1, w+2*pad, linesRows, pad
			st.scale = 1
			if l.songBig {
				st.scale = songBigLine
			}
			switch i {
			case stripVolume:
				st.want = min(1, max(0, m.playerState.Volume))
			default:
				st.want = 0
				if t := m.playerState.Track; t != nil && t.Duration > 0 {
					st.want = min(1, max(0, m.smoothPosition().Seconds()/t.Duration.Seconds()))
				}
			}
			found = true
			break // one line a row
		}
		if found || nRules >= linesMaxRules {
			continue
		}
		if from, to, ok := linesRuleIn(plain); ok {
			// The destination border stays as cells while a popup frame
			// returns. Its horizontal sides are not independent rules.
			x0 := ansi.StringWidth(plain[:from])
			x1 := x0 + ansi.StringWidth(plain[from:to])
			border := false
			for box := range l.boxGeo {
				if (y == box[1] || y == box[3]) && x0 <= box[2] && x1 > box[0] {
					border = true
					break
				}
			}
			if border {
				continue
			}
			x0, w := empty(y, line, plain, from, to)
			st := &l.strips[stripRule+nRules]
			nRules++
			key := [2]int{x0, w}
			born, old := l.rules[key]
			if !old {
				born = now
			}
			rules[key] = born
			st.seen, st.col, st.row, st.cols, st.rows, st.born = true, x0, y+m.hostRows, w, 1, born
		}
	}
	l.rules = rules
	// (The waves that ran over and under the lyrics are gone: beside words
	// they were in the way wherever they were put. Their strips are never
	// placed now.)
	// The labels: one picture over all that were found (and the last
	// song's words while they go).
	x0, y0, x1, y1 := 1<<30, 1<<30, -1, -1
	for _, lb := range l.labels {
		if lb.found {
			rows[lb.y] = blankCells(rows[lb.y], lb.x, 1)
			x0, y0 = min(x0, lb.x), min(y0, lb.y)
			x1, y1 = max(x1, lb.x+lb.cells+2), max(y1, lb.y+max(1, lb.rows)) // room for a number a little wider than its cells
		}
	}
	if x1 > x0 && now.Sub(l.labelsAt) < labelsOutFor {
		for _, lb := range l.goneWords {
			if lb.y < len(rows) {
				x0, y0 = min(x0, lb.x), min(y0, lb.y)
				x1, y1 = max(x1, lb.x+lb.cells+2), max(y1, lb.y+max(1, lb.rows))
			}
		}
	}
	if x1 > x0 {
		// A row more each way where there is one: the title is taller
		// than its row.
		y0, y1 = max(0, y0-1), min(len(rows), y1+1)
		st := &l.strips[stripLabels]
		x1 = min(x1, m.width)
		st.seen, st.col, st.row, st.cols, st.rows = true, x0, y0+m.hostRows, x1-x0, y1-y0
	}
	if lyr && m.playerState.Track != nil && m.songMenu == nil && m.linkOffer == nil && m.prompt == nil {
		st := &l.strips[stripLyrics]
		st.seen, st.col, st.row, st.cols, st.rows = true, l.lyrCol, l.lyrRow+m.hostRows, l.lyrCols, l.lyrRows
	}
	if cover && m.artwork.img != nil && l.coverCol >= 0 {
		st := &l.strips[stripCover]
		st.seen, st.col, st.row, st.cols, st.rows = true, l.coverCol, l.coverRow+m.hostRows, l.coverCols, l.coverRows
		// The light behind it: a few cells more each way, as far as the
		// screen goes.
		mx := min(7, st.col, m.width-st.col-st.cols)
		my := min(3, st.row, m.height+m.hostRows-st.row-st.rows)
		if mx > 0 && my > 0 && m.pixLight() {
			hs := &l.strips[stripHalo]
			hs.seen, hs.col, hs.row, hs.cols, hs.rows = true, st.col-mx, st.row-my, st.cols+2*mx, st.rows+2*my
			hs.boxW, hs.boxH = mx, my
		}
	}
	_ = changed
	// The terminal must redraw rows, never scroll them: a scroll takes the
	// pictures along, and they jump. The renderer scrolls when it finds
	// rows whose letters moved up or down as they were, so each row is
	// given its own (see linesRowMark).
	for y := range rows {
		rows[y] = linesRowMark(rows[y], y)
	}
	return strings.Join(rows, "\n")
}

// linesRowMarks are blanks that are different characters.
var linesRowMarks = [3]string{" ", "\u00a0", "\u2800"}

// linesRowMark makes the row y differ from the rows near it without showing
// anything: its last blank cells are written with other blank characters,
// in a pattern of its own.
func linesRowMark(line string, y int) string {
	total := ansi.StringWidth(line)
	if total < 3 {
		return line
	}
	// As many of its last three cells as are blank.
	tail := []rune(ansi.Strip(ansi.Cut(line, total-3, total)))
	k := 0
	for k < len(tail) && tail[len(tail)-1-k] == ' ' {
		k++
	}
	if k == 0 || len(tail) != 3 {
		return line // it ends in something of its own
	}
	n := y % 27
	marks := []string{linesRowMarks[n%3], linesRowMarks[n/3%3], linesRowMarks[n/9]}
	return ansi.Cut(line, 0, total-k) + "\x1b[0m" + strings.Join(marks[:k], "")
}

// linesWake brings the lines' clock back to the screen's pace at once when
// it has slowed to a watch: something may be about to move.
func (m *Model) linesWake() tea.Cmd {
	l := m.lines
	if l == nil || !l.ticking || l.quiet < linesQuietAfter {
		return nil
	}
	l.quiet = 0
	l.gen++ // the slow tick on its way stops when it comes
	l.due = time.Time{}
	return linesTick(l.gen, &l.due, screenFrame)
}

// linesTicking reports whether the lines' clock runs, so that the other
// clocks leave the screen's frames to it.
func (m *Model) linesTicking() bool {
	return m.lines != nil && m.lines.ticking && m.linesOn()
}

// linesEnsure starts the lines' clock when there is a line to draw.
func (m *Model) linesEnsure() tea.Cmd {
	l := m.lines
	if l == nil || l.ticking || !m.linesOn() || m.background || m.vizOpen() {
		return nil
	}
	l.ticking = true
	l.gen++
	l.last = time.Now()
	l.due, l.quiet = time.Time{}, 0
	return linesTick(l.gen, &l.due, screenFrame)
}

// linesFrameMsg draws the next picture, or takes the last one away when
// the line is no longer on screen.
func (m *Model) linesFrameMsg(msg linesTickMsg) tea.Cmd {
	l := m.lines
	if l == nil || msg.gen != l.gen {
		return nil
	}
	now := time.Now()
	if m.vizOpen() && m.viz.over {
		// The visualizer is opening over the interface: its pictures stay
		// as they are under it, and go once it covers them.
		l.last = now
		return linesTick(l.gen, &l.due, screenFrame)
	}
	if !m.linesOn() || !l.seen() || m.background || m.vizOpen() || m.introStep != introDone {
		// The glow's slower tick starts the clock again when it is back.
		l.ticking = false
		return m.linesHide()
	}
	if v := m.viz; v != nil && v.closing && v.closeN%2 == 1 {
		// The visualizer closing over the interface draws its scene this
		// frame: the interface under it is drawn on the frames it only
		// shrinks (see vizStep), or the two missed frames together.
		return linesTick(l.gen, &l.due, screenFrame)
	}
	gap := now.Sub(l.last)
	dt := min(0.1, max(0.001, gap.Seconds()))
	l.last = now
	// The one clock while it runs: the lyrics' and the progress line's
	// time go on with it (see the lyricsTickMsg case), and the text's
	// effects are drawn on its frames (see fxEnsure).
	if m.playerState.Track != nil && m.fastAnimating() {
		m.advanceLyricsClock()
	}
	logPath := os.Getenv("UTUIMP_LINES_LOG")
	if logPath != "" && l.sentPx == nil {
		l.sentPx = map[string]int{}
	}
	l.changed = false
	seq := m.linesDraw(dt, now)
	framesLog.note("lines", time.Since(now), strconv.Itoa(len(seq)))
	if path := logPath; path != "" {
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil { //nolint:gosec // the user's own path
			_, _ = fmt.Fprintf(f, "tick gap %5.1f ms  draw %5.1f ms  bytes %d  view %5.1f ms  views %d %s %v\n", gap.Seconds()*1000, time.Since(now).Seconds()*1000, len(seq), l.viewTook.Seconds()*1000, l.views, l.why, l.sentPx)
			if took := time.Since(now); took > 8*time.Millisecond || l.viewTook > 8*time.Millisecond {
				_, _ = fmt.Fprintf(f, "  slow: view [%s] strips", l.viewParts)
				for i, d := range l.stripTook {
					if d > time.Millisecond {
						_, _ = fmt.Fprintf(f, " %d:%.1f", i, d.Seconds()*1000)
					}
				}
				_, _ = fmt.Fprintln(f)
			}
			_ = f.Close()
		}
	}
	// Nothing playing, nothing sent for half a second and no words on
	// their way: nothing is going to move by itself, and the clock slows
	// to a watch (see linesWake). While a song plays the lines move with
	// it, and the clock keeps the screen's pace.
	playing := m.playerState.Playing && !m.playerState.Loading
	if !l.changed && !playing && (m.soft == nil || !m.soft.busy) && !m.fxBusy(now) && !l.touching(now) {
		l.quiet++
	} else {
		l.quiet = 0
	}
	pace := screenFrame
	if l.quiet >= linesQuietAfter {
		pace = linesWatch
	}
	next := linesTick(l.gen, &l.due, pace)
	if seq == "" {
		return next
	}
	return tea.Batch(m.sendRaw(seq), next)
}

// coverSpan is how a picture n pixels across, sent with a clear margin
// up to have pixels, is shown want pixels across, starting off pixels into
// a cell of cell pixels: over how many cells, and how much of the picture
// (the source) is stretched over them.
//
// The terminal stretches the source from the offset to the far edge of the
// last cell, not over the cells from the offset on: what the offset takes
// off the first cell the picture loses, squeezed. So the source is worked
// out for the cells less the offset; leaving the offset out, the cover
// grew and shrank by up to a cell with every frame as it travelled.
// Of the spans that hold the picture, the one whose source comes nearest
// a whole number of pixels is taken (the source is given in whole pixels):
// the picture is then shown within a fraction of a pixel of its size.
func coverSpan(want float64, n, have, cell, off int) (cells, src int) {
	first := int(math.Ceil((want + float64(off)) / float64(cell)))
	cells, src = first, have
	best := math.Inf(1)
	for c := first; c < first+3; c++ {
		f := float64(c*cell-off) * float64(n) / want
		r := int(f + 0.5)
		if r > have {
			break
		}
		// How far from want it is shown, in pixels.
		if e := math.Abs(float64(c*cell-off)*float64(n)/float64(r) - want); e < best {
			cells, src, best = c, r, e
		}
	}
	return cells, src
}

// keepOldCover keeps the cover on screen (st's picture) under the one that
// replaces it, which comes up over it: the one song's cover melts into the
// next instead of going and the next coming up out of nothing. Nothing is
// sent again for it: it is only put a layer down, and taken off once the
// new one is up (see dropOldCover). A cover on its way somewhere goes at
// once, as before.
func (l *uiLines) keepOldCover(st *lineStrip) string {
	drop := l.dropOldCover()
	if !st.shown || !l.covOK || st.img.Native || st.img.SrcW > 0 || l.covUp != "" {
		return drop
	}
	old := st.img
	old.Z = -4
	l.coverOld, l.coverOldOn = old, true
	id := linesCoverAltID
	if st.img.ID == linesCoverAltID {
		id = linesImageID + stripCover
	}
	st.img = gfx.Image{ID: id, Z: -3, Alpha: true}
	st.shown, st.sentKey = false, [5]int{}
	return drop + old.Place(st.col, st.row, st.cols, st.rows)
}

// dropOldCover takes the cover that was replaced off the screen.
func (l *uiLines) dropOldCover() string {
	if !l.coverOldOn {
		return ""
	}
	l.coverOldOn = false
	return l.coverOld.Delete()
}

// haloGain is how much of the light behind the cover shows: none while the
// cover travels, then coming up as a whole once it has arrived.
func haloGain(now, covAt, cameAt time.Time) float64 {
	return smooth(linesMoveFor, linesMoveFor+0.35, now.Sub(covAt).Seconds()) * smooth(0, linesComeFor+0.2, now.Sub(cameAt).Seconds())
}

// linesHide takes the pictures off the screen.
func (m *Model) linesHide() tea.Cmd {
	seq := m.linesHideSeq()
	if seq == "" {
		return nil
	}
	return tea.Raw(seq)
}

// linesHideSeq takes the interface's pictures off the screen. The visualizer
// uses the sequence directly so it can delete these layers before placing its
// own full-screen picture in the same terminal update.
func (m *Model) linesHideSeq() string {
	l := m.lines
	if l == nil {
		return ""
	}
	var seq strings.Builder
	for i := range l.strips {
		st := &l.strips[i]
		st.shown = false
		st.sentKey = [5]int{}
		// Delete even when our state says it is gone: Kitty may still have
		// a placement left from an interrupted redraw or an earlier session.
		seq.WriteString(st.img.Delete())
	}
	for k := range l.lyrBands {
		b := &l.lyrBands[k]
		b.shown, b.prev = false, b.prev[:0]
		seq.WriteString(b.img.Delete())
	}
	for k := range l.lyrLines {
		ln := &l.lyrLines[k]
		ln.shown, ln.what = false, ""
		seq.WriteString(ln.img.Delete())
	}
	if l.coverOldOn {
		seq.WriteString(l.coverOld.Delete())
	}
	l.coverOldOn = false
	coverAlt := gfx.Image{ID: linesCoverAltID}
	seq.WriteString(coverAlt.Delete())
	seq.WriteString(l.menuImg.Delete())
	l.menuShown = false
	return seq.String()
}

// lineWave is a wave that runs along a line: a new song sends one across
// the progress line, and they keep coming while a song loads.
type lineWave struct {
	at, amp float64 // where its crest is, 0..1 of the line; its height, in cell heights
}

// linesDraw makes the pictures of the lines as they are now and returns the
// sequence that shows them ("" when the ones on screen still hold).
func (m *Model) linesDraw(dt float64, now time.Time) string {
	l := m.lines
	if l.cellW <= 0 || l.cellH <= 0 {
		l.cellW, l.cellH = cellPixels()
		if l.cellW <= 0 || l.cellH <= 0 {
			l.cellW, l.cellH = 9, 18
		}
	}
	l.tick++
	playing := m.playerState.Playing && !m.playerState.Loading

	// The music: how loud the lows and the middle are right now.
	if l.ear == nil {
		l.ear = newVisualizer()
		l.ear.strength = 1
	}
	e := l.ear
	e.listen(playing, m.playerState.Position, dt)
	e.analyse(dt)
	want, bass := 0.0, 0.0
	if playing {
		want = 0.25 + 0.75*clamp01(0.6*e.voice[0]+0.4*e.voice[1])
		bass = 0.2 + 0.8*clamp01(e.voice[voiceBass])
	}
	l.amp += (want - l.amp) * min(1, dt*3) // a second or so to rise or settle
	l.bass += (bass - l.bass) * min(1, dt*2.5)
	l.kick *= math.Exp(-dt * 7)
	if playing {
		l.kick = math.Max(l.kick, 0.85*clamp01(e.kick))
	}
	// The breath: the lows, eased, with each drum beat on top. Everything
	// that breathes takes it from here.
	breath := clamp01(l.bass + 0.45*l.kick)
	l.t += dt * (0.35 + 0.9*l.amp)
	l.bt += dt * l.amp * 0.6

	// The progress and controls keep their appearance across track changes.
	var wave lineWave
	if t := m.playerState.Track; t != nil {
		if id := t.ID + "\x00" + t.Title; id != l.song {
			if l.song != "" {
				fresh := false
				for _, t := range l.touches {
					fresh = fresh || (t.kind == touchSong && now.Sub(t.at).Seconds() < 2)
				}
				if !fresh {
					l.touch(touchSong, 1) // it changed by itself
				}
			}
			l.song = id
		}
	}
	// A change of volume stirs its line.
	if v := m.playerState.Volume; v != l.vol {
		if l.vol >= 0 {
			l.volKick = 1
		}
		l.vol = v
	}
	l.volKick = max(0, l.volKick-dt*1.6)

	// The colours: the cover's, softened; they drift to a new song's.
	if m.artwork.img != nil && m.artwork.url != l.palURL {
		l.palURL = m.artwork.url
		l.palTo = vizPalette(m.artwork.img)
	} else if m.artwork.url == "" && l.palURL != "" {
		l.palURL, l.palTo = "", linesDefaultPal()
	}
	if !l.palStarted {
		l.palStarted = true
		if l.palURL == "" {
			l.palTo = linesDefaultPal()
		}
		l.pal = l.palTo
	}
	for i := range l.pal {
		l.pal[i] = l.pal[i].mix(l.palTo[i], min(1, dt*1.2))
	}
	fgc, _, _ := styles.ScreenColors()
	fg := vcol{float64(fgc.R) / 255, float64(fgc.G) / 255, float64(fgc.B) / 255}
	palKey := int(l.pal[0].r*40) + int(l.pal[0].g*40)<<8 + int(l.pal[0].b*40)<<16

	// The progress line's fill goes on with the song between layouts of
	// the screen (which a frame of the lines does not always make anew).
	if st := &l.strips[stripProgress]; st.seen {
		st.want = 0
		if t := m.playerState.Track; t != nil && t.Duration > 0 {
			st.want = min(1, max(0, m.smoothPosition().Seconds()/t.Duration.Seconds()))
		}
	}
	touching := l.touching(now)
	// Calm lines stand still: the music does not sway them nor the drum
	// thicken them, and keys do not bend them (the buttons and the light
	// behind the cover go on as they are).
	calm := m.linesCalm()
	lineAmp, lineBass, lineKick, lineTouch := l.amp, l.bass, l.kick, touching
	if calm {
		lineAmp, lineBass, lineKick, lineTouch = 0, 0, 0, false
		wave = lineWave{}
	}
	bend := func(focused bool) func(side int, u, s float64) float64 {
		if calm {
			return nil
		}
		return l.frameBend(now, focused)
	}
	ctl := m.linesCtlState()
	ctlPop, ctlLive := l.ctlPops(ctl, now)
	l.frameN++
	var seq strings.Builder
	var stripAt time.Time
	if l.sentPx != nil {
		l.stripTook = map[int]time.Duration{}
		stripAt = time.Now()
		defer func() { l.stripTook[len(l.strips)-1] += time.Since(stripAt) }()
	}
	carry := l.glideCarry(m) // (see lines_glide.go)
	for i := range l.strips {
		if l.sentPx != nil && i > 0 {
			t := time.Now()
			l.stripTook[i-1] += t.Sub(stripAt)
			stripAt = t
		}
		st := &l.strips[i]
		if !st.seen {
			if st.shown {
				// It goes softly: a few frames of what it last was,
				// fainter each time.
				if st.goneAt.IsZero() {
					st.goneAt = now
				}
				l.changed = true
				if i != stripCover && i < stripFrame {
					st.img.Native, st.img.OffX, st.img.OffY = false, 0, 0 // the whole picture, where its cells are
				}
				if i == stripLyrics {
					if l.lyrLinesShown() && st.pw > 0 && st.ph > 0 {
						// Shown a line a picture: it goes as one, drawn once.
						st.rgba = l.lyr.paintAll(st.rgba[:0], st.pw, st.ph, l.lyr.lastItems)
					}
					seq.WriteString(l.lyrLinesDelete())
					seq.WriteString(l.bandsDelete())
				}
				k := 1 - now.Sub(st.goneAt).Seconds()/linesGoFor
				if (i == stripLyrics || i == stripLabels) && l.covOK && now.Sub(l.covAt).Seconds() < linesMoveFor {
					k = 0 // the words go at once while the cover travels (see cameAt)
				}
				if k > 0 && !st.goneAt.Equal(now) && (l.frameN+i)%linesHalfRate != 0 {
					continue // it fades out every other frame (see linesHalfRate)
				}
				if i >= stripFrame && k > 0 && len(st.framePixels) == st.frameW*st.frameH*4 && len(st.framePixels) > 0 {
					st.img.Native, st.img.OffX, st.img.OffY = true, st.frameOffX, st.frameOffY
					if one, err := st.img.Frame(l.mode, l.faded(st.framePixels, k), st.frameW, st.frameH, st.frameCol, st.frameRow, 0, 0); err == nil && one != "" {
						seq.WriteString(one)
						continue
					}
				}
				if k > 0 && st.pw > 0 && len(st.rgba) == st.pw*st.ph*4 && (!st.img.Native || i != stripCover) {
					if one, err := st.img.Frame(l.mode, l.faded(st.rgba, k), st.pw, st.ph, st.col, st.row, st.cols, st.rows); err == nil && one != "" {
						seq.WriteString(one)
						continue
					}
				}
				st.shown, st.goneAt = false, time.Time{}
				st.sentKey = [5]int{}
				st.img.Native = false
				seq.WriteString(st.img.Delete())
				if i == stripCover {
					l.covOK = false
					seq.WriteString(l.dropOldCover())
				}
			}
			continue
		}
		if i == stripCover && (!st.shown || !st.goneAt.IsZero()) {
			// The same artwork can return after a layout that had no cover.
			// Its cached pixels are still valid, but its fade must start again.
			l.coverAt = now
		}
		if !st.goneAt.IsZero() {
			st.goneAt, st.sentKey = time.Time{}, [5]int{} // back before it had gone
		}
		if !st.shown && !now.Before(st.cameAt) {
			st.cameAt = now
			// The words that come with another view come once the cover
			// has all but got to its place, not while it travels: the
			// terminal takes in their pictures, a whole screen of
			// them a frame as they fade in, and late with that, it
			// shows the cover's way in jumps.
			if (i == stripLyrics || i == stripLabels) && l.covOK && now.Sub(l.covAt).Seconds() < linesMoveFor*0.7 {
				st.cameAt = l.covAt.Add(time.Duration(linesMoveFor * 0.7 * float64(time.Second)))
			}
		}
		if now.Before(st.cameAt) {
			continue // (not yet)
		}
		// What moves this line, and whether it moves at all right now.
		// slow: it only sways or breathes, by a little and slowly (see
		// linesHalfRate).
		amp, moving, slow := 0.0, false, false
		w, h := st.cols*l.cellW, st.rows*l.cellH
		if i == stripHalo {
			// The light is soft through and through: drawn at a third of
			// the screen's pixels each way and spread over its cells by the
			// terminal, which blends the pixels as it stretches them, it
			// differs from it drawn at full size by at most 3 of 255 where
			// it can be seen (as at a half), for a ninth of the work and
			// of what is sent (see TestHaloScale).
			w, h = (w+linesHaloScale-1)/linesHaloScale, (h+linesHaloScale-1)/linesHaloScale
		}
		// A side of a frame on its way to another place is placed by the
		// pixel, at its own size.
		var sideMask []bool
		morphing := i >= stripFrame && st.morph.moving(now)
		if i >= stripFrame {
			// A frame just finishing a pixel-positioned morph must be sent
			// once at its final cell geometry, even if its cache key matches.
			if !morphing && st.morphed {
				// morphSide updates col and row as it moves. Restore the
				// destination even if the text layout was cached this tick.
				target := st.morph.to
				switch st.side {
				case sideTop:
					st.col, st.row = int(target[0]), int(target[1])-st.pad
				case sideBottom:
					st.col, st.row = int(target[0]), int(target[3])-st.pad
				case sideLeft:
					st.col, st.row = int(target[0])-st.pad, int(target[1])+1
				case sideRight:
					st.col, st.row = int(target[2])-st.pad, int(target[1])+1
				}
				st.sentKey = [5]int{-1}
			}
			st.morphed = morphing
			st.img.Native, st.img.OffX, st.img.OffY = false, 0, 0
			sideMask = st.mask
			if morphing {
				var x, y int
				x, y, w, h, sideMask = l.morphSide(st, st.morph.at(now))
				if st.side == sideLeft || st.side == sideRight {
					sideMask = nil
				}
				st.col, st.row = x/l.cellW, y/l.cellH
				st.img.Native, st.img.OffX, st.img.OffY = true, x%l.cellW, y%l.cellH
			}
		}
		// The song's block on its way to another place (see
		// lines_glide.go).
		gliding, drawScale := false, 1.0
		switch i {
		case stripProgress, stripVolume, stripCtl:
			if i == stripCtl {
				st.img.Native, st.img.OffX, st.img.OffY = false, 0, 0
			}
			w, h, drawScale, gliding = l.glideStrip(st, w, h, now, carry)
		case stripLabels:
			w, h, gliding = l.glideLabelsStrip(st, w, h, now, m, carry)
		}
		reveal := 1.0
		switch {
		case i == stripProgress:
			amp = lineAmp
			moving = st.easeFill(dt) || amp >= 0.004 || wave.amp > 0
		case i == stripVolume:
			if !calm {
				amp = 0.7 * l.volKick * (2 - l.volKick) // rises at once, settles slowly
			}
			moving = st.easeFill(dt) || amp >= 0.004
		case i == stripBack || i == stripBackLow:
			amp = l.amp
			reveal = clamp01(now.Sub(st.born).Seconds() / 0.6)
			moving = reveal < 1 || amp >= 0.02
			st.img.Native = false
			if ly := &l.strips[stripLyrics]; ly.seen && l.lyr.font != nil {
				// With the lyrics a picture the waves keep to the words:
				// the same distance over the first row that can be read
				// and under the last, wherever those are, and they
				// follow when the words move.
				gap := 1.3 * float64(l.cellH)
				half := float64(linesWaveRows*l.cellH) / 2
				full := float64(ly.rows * l.cellH)
				// Never among the words: where those would fade in, the
				// wave keeps to the strip that is clear of them.
				wantTop, wantLow := l.lyr.extTop-gap, l.lyr.extBot+gap
				wantTop, wantLow = math.Max(half, wantTop), math.Min(full-half, wantLow)
				if !l.waveOK {
					l.waveTop, l.waveLow, l.waveOK = wantTop, wantLow, true
				}
				if i == stripBack {
					k := 1 - math.Exp(-dt*5)
					l.waveTop += (wantTop - l.waveTop) * k
					l.waveLow += (wantLow - l.waveLow) * k
				}
				mid := l.waveTop
				if i == stripBackLow {
					mid = l.waveLow
				}
				y0 := max(0, ly.row*l.cellH+int(mid-half+0.5))
				st.row, st.frac = y0/l.cellH, float64(y0%l.cellH)
				st.img.Native, st.img.OffX, st.img.OffY = true, 0, y0%l.cellH
				if math.Abs(wantTop-l.waveTop)+math.Abs(wantLow-l.waveLow) > 0.5 {
					moving = true
				}
			} else {
				l.waveOK = false
			}
		case i == stripCtl:
			moving = ctlLive || ctl.loading || l.kick > 0.03
			slow = !ctlLive && !ctl.loading
		case i == stripLabels:
			// Drawn anew when the words change; another song's come up
			// softly.
			title := ""
			var words strings.Builder
			for _, lb := range l.labels {
				if lb.found && lb.kind == labelTitle {
					title = lb.text
				}
				if lb.found && isSongWord(lb) {
					fmt.Fprintf(&words, "%d:%d:%s;", lb.kind, len(lb.text), lb.text)
				}
			}
			if title != "" && words.String() != l.songWordsOf {
				// The last song's words melt away as the new ones come.
				l.labelsAt = now
				l.goneWords = nil
				if l.titleOf != "" {
					l.goneWords = append(l.goneWords, l.songWords...)
				}
				l.titleOf = title
				l.songWordsOf = words.String()
			}
			sig := l.labelsSig() + fmt.Sprintf("%d %v", int(m.playerState.Volume*100), m.preMuteVol >= 0)
			moving = sig != l.labelsOf || now.Sub(l.labelsAt) < labelsInFor+50*time.Millisecond
			for _, lb := range l.labels {
				if lb.found && lb.scroll {
					if f := l.labelFont(lb.kind, lb.scale); f != nil && f.Width(lb.text) > lb.cells*l.cellW {
						moving = true
					}
				}
			}
			slow = false // keep the crossfade at the screen's full frame rate
			l.labelsOf = sig
		case i == stripLyrics:
			// Drawn anew when the lyrics change, while the text glides
			// or a line's light eases, and while the voice runs along
			// a line with word times.
			sc := m.lyricsP.m.CoverScene()
			sig := fmt.Sprintf("%d %d %d %d %q %q %d", sc.Focus, sc.Current, sc.Cursor, len(sc.Lines), sc.Message, sc.Status, int(sc.Lit*6))
			if n := len(sc.Lines); n > 0 {
				sig += sc.Lines[0].Text + sc.Lines[n-1].Translation
			}
			moving = l.lyrMoving || sig != l.lyrSig || (sc.Lit >= 0 && m.playerState.Playing)
			l.lyrSig = sig
		case i == stripCover:
			// Still, once it has come up and got to its place; made anew
			// for another cover or another size.
			moving = now.Sub(l.coverAt).Seconds() < 0.6 || now.Sub(l.covAt).Seconds() < linesMoveFor
			slow = now.Sub(l.coverAt).Seconds() >= 0.6 && now.Sub(l.covAt).Seconds() >= linesMoveFor
		case i == stripSel:
			amp = breath
			if calm {
				amp = 0
			}
			// It flows to where it is going, the end that leads faster
			// than the one that follows.
			if !l.selOn {
				l.selA, l.selB, l.selOn = l.selTop, l.selBot, true
			}
			lead, trail := min(1, dt*24), min(1, dt*11)
			ka, kb := lead, trail
			if l.selTop > l.selA {
				ka, kb = trail, lead
			}
			l.selA += (l.selTop - l.selA) * ka
			l.selB += (l.selBot - l.selB) * kb
			away := math.Abs(l.selA-l.selTop)+math.Abs(l.selB-l.selBot) > 0.4
			if !away {
				l.selA, l.selB = l.selTop, l.selBot
			}
			moving = away || amp >= 0.02
			st.frac = (l.selA*7 + l.selB) / float64(max(1, w))
		case i == stripHalo:
			amp = breath
			slow = true
			if now.Sub(l.covAt).Seconds() < linesMoveFor+0.4 || now.Sub(st.cameAt).Seconds() < linesComeFor+0.25 {
				moving, slow = true, false
			}
			moving = moving || amp >= 0.02
			if l.haloBlank && haloGain(now, l.covAt, st.cameAt) <= 0 {
				moving = false // it waits for the cover, and nothing of it shows: sent once
			}
		case i >= stripFrame:
			amp = lineBass
			reveal = clamp01(now.Sub(st.born).Seconds() / 0.5)
			moving = reveal < 1 || amp >= 0.004 || morphing
			want := 0.0
			if st.focused {
				want = 1
			}
			if now.Sub(st.born).Seconds() < 0.05 {
				st.lit = 0 // a new frame takes its colour as it draws itself in
			}
			slow = reveal >= 1 && !morphing
			if d := want - st.lit; math.Abs(d) > 0.004 {
				st.lit += d * min(1, dt*5)
				moving, slow = true, false
			} else {
				st.lit = want
			}
		default:
			amp = lineBass
			reveal = clamp01(now.Sub(st.born).Seconds() / 0.35)
			moving = reveal < 1 || amp >= 0.004
			slow = reveal >= 1
		}
		if (i == stripProgress || i == stripVolume || i == stripCtl || i == stripSel || i == stripLyrics || i == stripLabels) && now.Sub(st.cameAt).Seconds() < linesComeFor+0.05 {
			// Still coming up: a fade, every other frame, unless it moves too.
			slow = slow || !moving
			moving = true
		}
		if touching && i == stripCtl || lineTouch && (i == stripProgress || i == stripSel || i >= stripRule) {
			moving, slow = true, false // the hand is on it: every frame
		}
		if gliding {
			moving, slow = true, false // on its way: every frame
		}
		// Nothing moves while it is still: the picture on screen holds,
		// bar a refresh now and then (a cleared screen takes pictures
		// with it).
		key := [5]int{st.col<<16 | st.row, st.cols<<16 | st.rows, int(st.frac * float64(w)), 0, palKey}
		if i >= stripFrame {
			key[2], key[3] = maskKey(st.mask), st.side<<1
			if st.focused {
				key[3]++
			}
		}
		if i == stripCtl {
			bits := ctl.repeat
			for n, on := range []bool{ctl.shuffle, ctl.playing, ctl.loading, ctl.fav} {
				if on {
					bits |= 4 << n
				}
			}
			key[3] = bits
		}
		if i == stripCover {
			key[4] = 0
			if ck := fmt.Sprintf("%s %dx%d", m.artwork.url, w, h); ck != l.coverKey || len(st.rgba) != w*h*4 {
				if !strings.HasPrefix(l.coverKey, m.artwork.url+" ") {
					l.coverAt = now // another cover, not another size
					seq.WriteString(l.keepOldCover(st))
				}
				l.coverKey = ck
				st.rgba = make([]byte, w*h*4)
				drawCover(st.rgba, w, h, m.artwork.img, linesCoverRound*float64(l.cellH))
				st.sentKey = [5]int{}
			}
		}

		if moving || key != st.sentKey {
			l.changed = true // (a picture only sent again, now and then, is not)
		}
		switch {
		case !moving:
			if key == st.sentKey && now.Sub(st.sentAt) < time.Second {
				continue
			}
		case slow && st.shown && [3]int(key[:3]) == [3]int(st.sentKey[:3]) && (l.frameN+i)%linesHalfRate != 0:
			continue // drawn every other frame, half of them on each
		}
		if len(st.rgba) != w*h*4 {
			st.rgba = make([]byte, w*h*4)
		}
		switch {
		case i == stripProgress:
			waves := []lineWave{wave}
			if !calm {
				waves = append(l.progressWaves(now, st.frac), wave)
			}
			drawProgressPad(st.rgba, w, h, st.pad*l.cellW, st.frac, amp, l.t, float64(l.cellH)*drawScale, l.pal, fg, waves...)
		case i == stripVolume:
			drawProgressPad(st.rgba, w, h, st.pad*l.cellW, st.frac, amp, l.t, float64(l.cellH)*drawScale, l.pal, fg)
		case i == stripCtl:
			pop := ctlPop
			for _, t := range l.touches {
				if t.kind == touchPlay {
					pop[2] -= 0.6 * spring(now.Sub(t.at).Seconds()) // pressed
				}
			}
			drawControls(st.rgba, w, h, float64(l.cellH)*drawScale, ctlLook{ctlState: ctl, pop: pop, kick: l.kick,
				spin: float64(now.UnixMilli()%1200) / 1200 * 2 * math.Pi, pal: l.pal, fg: fg})
		case i == stripLyrics:
			white := vcol{1, 1, 1}
			// A line a picture once they have come up (while they come up
			// they fade as one).
			lines := now.Sub(st.cameAt).Seconds() >= linesComeFor+0.05
			l.lyrItems, l.lyrWhole, l.lyrMoving = l.lyr.frame(st.rgba, w, h, m.lyricsP.m.CoverScene(), min(0.1, max(0.001, now.Sub(l.lyrAt).Seconds())),
				lyricLook{fg: fg, sung: white.mix(l.pal[0], 0.22), pick: l.pal[1].mix(white, 0.35)}, lines)
			l.lyrAt = now
		case i == stripLabels:
			vol := min(1, max(0, m.playerState.Volume))
			if m.preMuteVol >= 0 {
				vol = -1
			}
			age := now.Sub(l.labelsAt).Seconds()
			if age > labelsOutFor.Seconds() {
				l.goneWords = nil
			}
			ox, oy := float64(st.col*l.cellW), float64(st.row*l.cellH)
			if st.img.Native {
				ox, oy = ox+float64(st.img.OffX), oy+float64(st.img.OffY)
			}
			l.drawLabels(st.rgba, w, h, ox, oy, m.hostRows, labelLook{fg: fg, accent: l.pal[0].mix(vcol{1, 1, 1}, 0.4),
				volume: vol, fade: smooth(labelsInAfter.Seconds(), labelsInFor.Seconds(), age),
				gone: 1 - smooth(0, labelsOutFor.Seconds(), age)})
			// What this song's words are, for when the next one comes.
			l.songWords = l.songWords[:0]
			for _, lb := range l.labels {
				if lb.found && isSongWord(lb) {
					l.songWords = append(l.songWords, lb)
				}
			}
		case i == stripCover:
			// Drawn when it changed, above.
		case i == stripSel:
			squeeze := 0.0
			if !calm {
				squeeze = l.selSqueeze(now) * (l.selB - l.selA) / 2
			}
			drawSel(st.rgba, w, h, l.selA+squeeze, l.selB-squeeze, float64(l.cellH), amp, l.pal[0])
		case i == stripHalo:
			// While the cover travels its light waits, and comes back
			// when it has arrived.
			// It comes up after the cover has arrived, and as a whole: its
			// strength is turned up, the finished picture is not thinned
			// (which leaves a grain of single steps).
			gain := haloGain(now, l.covAt, st.cameAt)
			l.haloBlank = gain <= 0
			l.halo.draw(st.rgba, w, h, st.boxW*l.cellW/linesHaloScale, st.boxH*l.cellH/linesHaloScale,
				linesCoverRound*float64(l.cellH)/linesHaloScale, l.t, amp, gain, l.pal)
		case i >= stripFrame:
			bw, bh := st.boxW, st.boxH
			if morphing {
				r := st.morph.at(now)
				bw, bh = max(2, int(math.Round(r[2]-r[0]+1))), max(2, int(math.Round(r[3]-r[1]+1)))
			}
			drawFrameSide(st.rgba, w, h, st.side, bw, bh, sideMask, float64(l.cellW), float64(l.cellH),
				frameLook{amp: amp, kick: lineKick, t: l.t, reveal: reveal, focused: st.focused, lit: st.lit, pal: l.pal, fg: fg,
					pad: st.pad, bend: bend(st.focused)})
		case i == stripBack:
			drawWave(st.rgba, w, h, l.bt, 0.25+0.75*l.bass, l.kick, reveal, 0, l.pal[0])
		case i == stripBackLow:
			drawWave(st.rgba, w, h, l.bt, 0.25+0.75*l.amp, l.kick, reveal, 1, l.pal[1])
		default:
			lift := 0.0
			if !calm {
				lift = l.ruleLift(now)
			}
			drawRule(st.rgba, w, h, amp, l.t, float64(l.cellH), reveal, lift, fg)
		}
		pix := st.rgba
		st.pw, st.ph = w, h
		if i >= stripFrame && l.closingActive && (i-stripFrame)/4 == l.closingSlot {
			pix = l.faded(pix, 1-smooth(0, linesMorphFor, now.Sub(l.closingFrame.start).Seconds()))
		}

		// What has just come comes softly (the frames, the rules and the
		// waves draw themselves in instead).
		if i == stripProgress || i == stripVolume || i == stripCtl || i == stripSel || i == stripLyrics || i == stripLabels {
			if k := smooth(0, linesComeFor, now.Sub(st.cameAt).Seconds()); k < 1 {
				pix = l.faded(pix, k)
				key[4] = -1 - int(k*1000) // not the finished picture yet
			}
		}
		if i == stripCover {
			// The cover travels to a new place and grows or shrinks on
			// the way, instead of jumping there.
			to := [4]float64{float64(st.col * l.cellW), float64(st.row * l.cellH), float64(w), float64(h)}
			if !l.covOK {
				l.covCur, l.covFrom, l.covTo, l.covOK = to, to, to, true
			} else if to != l.covTo {
				framesLog.note("cover-to", 0, fmt.Sprintf("%v -> %v at %v (%.0f ms into the last)", l.covTo, to, l.covCur, now.Sub(l.covAt).Seconds()*1000))
				l.covFrom, l.covTo, l.covAt = l.covCur, to, now
			}
			p := clamp01(now.Sub(l.covAt).Seconds() / linesMoveFor)
			p = p * p * (3 - 2*p)
			for n := range l.covCur {
				l.covCur[n] = l.covFrom[n] + (l.covTo[n]-l.covFrom[n])*p
			}
			st.img.Native = false
			if p < 1 {
				// The picture is sent once, at the size it is going to,
				// with a clear margin; after that only its place and how
				// much of it to show are, each frame. Showing a little of
				// the margin too, over whole cells, shrinks the picture
				// itself to any size by the pixel: the terminal does the
				// scaling, at the screen's own pace.
				mx, my := w/6+2*l.cellW, h/6+2*l.cellH
				cw2, ch2 := w+mx, h+my
				seqUp := ""
				if l.covUp != l.coverKey {
					if cap(l.scaled) < cw2*ch2*4 {
						l.scaled = make([]byte, cw2*ch2*4)
					}
					l.scaled = l.scaled[:cw2*ch2*4]
					clear(l.scaled)
					for yy := range h {
						copy(l.scaled[yy*cw2*4:yy*cw2*4+w*4], st.rgba[yy*w*4:(yy+1)*w*4])
					}
					up, ok := st.img.Upload(l.mode, l.scaled, cw2, ch2)
					if ok {
						seqUp, l.covUp = up, l.coverKey
					}
				}
				if l.covUp == l.coverKey {
					x, y := int(l.covCur[0]+0.5), int(l.covCur[1]+0.5)
					wd, hd := math.Max(8, l.covCur[2]), math.Max(8, l.covCur[3])
					st.img.OffX, st.img.OffY = x%l.cellW, y%l.cellH
					var cols, rows int
					cols, st.img.SrcW = coverSpan(wd, w, cw2, l.cellW, st.img.OffX)
					rows, st.img.SrcH = coverSpan(hd, h, ch2, l.cellH, st.img.OffY)
					st.shown, st.sentAt, st.sentKey = true, now, [5]int{-1}
					seq.WriteString(seqUp)
					seq.WriteString(st.img.Place(x/l.cellW, y/l.cellH, cols, rows))
					continue
				}
			}
			st.img.SrcW, st.img.SrcH = 0, 0
			st.img.OffX, st.img.OffY, l.covUp = 0, 0, ""
		}
		if i == stripCover {
			// A new cover comes up out of nothing in half a second.
			if k := smooth(0, 0.5, now.Sub(l.coverAt).Seconds()); k < 1 {
				if len(l.coverFade) != len(pix) {
					l.coverFade = make([]byte, len(pix))
				}
				copy(l.coverFade, pix)
				for j := 3; j < len(pix); j += 4 {
					l.coverFade[j] = byte(float64(pix[j]) * k)
				}
				pix = l.coverFade
				key[3] = 1 + int(k*1000) // not the finished picture yet
			} else {
				l.coverFade = nil
				seq.WriteString(l.dropOldCover()) // the new one is up: the old one under it goes
			}
		}
		if i == stripLyrics {
			if l.lyrWhole {
				// While a word's light runs along its line only that line
				// changes: the lyrics go in bands, and only the bands that
				// changed are sent (all of them now and then, as every
				// picture is: a cleared screen takes pictures with it).
				seq.WriteString(l.lyrLinesDelete())
				seq.WriteString(l.sendBands(pix, w, h, st, now, !moving))
			} else {
				seq.WriteString(l.bandsDelete())
				seq.WriteString(l.sendLyricLines(st, w, h, now, false))
			}
			st.shown, st.sentAt, st.sentKey = true, now, key
			continue
		}
		// A thin line is sent without the clear rows round it: they are
		// most of its picture (the room it has to sway and bend in), and
		// what is sent sixty times a second is what the terminal works
		// for. The rest is placed by the pixel where it was.
		sendH, col, row := h, st.col, st.row
		if i == stripProgress || i == stripVolume || i == stripSel || (i >= stripRule && i < stripFrame) ||
			i >= stripFrame && (st.side == sideTop || st.side == sideBottom) {
			bx, by := st.col*l.cellW, st.row*l.cellH
			if (i >= stripFrame || gliding) && st.img.Native {
				bx, by = bx+st.img.OffX, by+st.img.OffY
			}
			y0, y1 := alphaRows(pix, w, h)
			if y1 <= y0 {
				y0, y1 = 0, 1 // nothing to show: one clear row takes the last picture's place
			}
			pix, sendH = pix[y0*w*4:y1*w*4], y1-y0
			col, row = bx/l.cellW, (by+y0)/l.cellH
			st.img.Native, st.img.OffX, st.img.OffY = true, bx%l.cellW, (by+y0)%l.cellH
		}
		sendW := w
		if i >= stripFrame && (st.side == sideLeft || st.side == sideRight) {
			// An upright side: without the clear columns round it.
			bx, by := st.col*l.cellW, st.row*l.cellH
			if st.img.Native {
				bx, by = bx+st.img.OffX, by+st.img.OffY
			}
			x0, x1 := alphaCols(pix, w, h)
			if x1 <= x0 {
				x0, x1 = 0, 1
			}
			if cap(l.colBuf) < (x1-x0)*h*4 {
				l.colBuf = make([]byte, (x1-x0)*h*4)
			}
			out := l.colBuf[:(x1-x0)*h*4]
			for y := range h {
				copy(out[y*(x1-x0)*4:(y+1)*(x1-x0)*4], pix[(y*w+x0)*4:(y*w+x1)*4])
			}
			pix, sendW = out, x1-x0
			col, row = (bx+x0)/l.cellW, by/l.cellH
			st.img.Native, st.img.OffX, st.img.OffY = true, (bx+x0)%l.cellW, by%l.cellH
		}
		if l.sentPx != nil { // only with UTUIMP_LINES_LOG
			kind := map[int]string{stripProgress: "progress", stripVolume: "volume", stripCover: "cover", stripHalo: "halo", stripSel: "sel", stripCtl: "buttons", stripLyrics: "lyrics", stripLabels: "labels"}[i]
			if kind == "" {
				kind = "frames/rules"
			}
			l.sentPx[kind] += len(pix)
			l.sentPx[kind+"#"]++
		}
		one, err := st.img.Frame(l.mode, pix, sendW, sendH, col, row, st.cols, st.rows)
		if err != nil || one == "" {
			l.mode = gfx.Off // no pictures after all: the cells come back
			return seq.String()
		}
		if i >= stripFrame {
			st.framePixels = append(st.framePixels[:0], pix...)
			st.frameW, st.frameH, st.frameCol, st.frameRow = sendW, sendH, col, row
			st.frameOffX, st.frameOffY = st.img.OffX, st.img.OffY
		}
		st.shown, st.sentAt, st.sentKey = true, now, key
		seq.WriteString(one)
	}
	seq.WriteString(m.songMenuBackdrop())
	return seq.String()
}

// drawRule draws a rule into buf (w×h pixels, one row of cells): one soft
// line that fades out towards its ends and sways with the lows, by amp of
// the little it may (see linesRuleSway). reveal below 1 is a rule still
// drawing itself in, from the left.
func drawRule(buf []byte, w, h int, amp, t, cell, reveal, lift float64, fg vcol) {
	clear(buf)
	if w <= 0 || h <= 0 {
		return
	}
	mid := float64(h)/2 - 0.5
	core := math.Max(0.8, cell/22)
	sway := amp * linesRuleSway * cell
	ends := math.Min(1.5*cell, float64(w)/5)
	c := fg.mul(0.55)
	r, g, b := byte(255*clamp01(c.r)), byte(255*clamp01(c.g)), byte(255*clamp01(c.b))
	upTo := reveal * float64(w+int(ends))
	for x := range w {
		fx := float64(x)
		k := 0.75 * smooth(0, ends, fx) * smooth(0, ends, float64(w-1)-fx) * smooth(0, ends, upTo-fx)
		if k < 0.004 {
			continue
		}
		u := fx / cell
		// Still at both ends, like a string.
		bow := math.Sin(math.Pi * fx / float64(w))
		y := mid + sway*bow*(0.7*math.Sin(u*0.16-t*1.1)+0.3*math.Sin(u*0.37+t*0.7)) + lift*cell*bow
		for py := max(0, int(y-3*core)); py <= min(h-1, int(y+3*core)+1); py++ {
			a := k * math.Exp(-sq((float64(py)-y)/core))
			if a < 0.004 {
				continue
			}
			i := (py*w + x) * 4
			buf[i], buf[i+1], buf[i+2], buf[i+3] = r, g, b, byte(255*a)
		}
	}
}

// linesDefaultPal is the lines' colours with no cover to take them from: the
// theme's.
func linesDefaultPal() [3]vcol {
	fgc, _, _ := styles.ScreenColors()
	fg := vcol{float64(fgc.R) / 255, float64(fgc.G) / 255, float64(fgc.B) / 255}
	return [3]vcol{fg, fg.mul(0.85), fg.mul(0.7)}
}

// drawProgress draws the progress line into buf (w×h pixels, four bytes
// each, see-through where nothing is drawn): three fine strands over the
// played part, swaying up to amp of what they may (see linesSway) and
// meeting at both ends; a thin still line over the rest; a bright point
// where the song is. cell is a cell's height in pixels. wave, if it has a
// height, lifts the whole line where its crest is.
func drawProgress(buf []byte, w, h int, frac, amp, t, cell float64, pal [3]vcol, fg vcol, waves ...lineWave) {
	drawProgressPad(buf, w, h, 0, frac, amp, t, cell, pal, fg, waves...)
}

// drawProgressPad is drawProgress with pad pixels of room before the line
// and after it: the line's light and the point where the song is run out
// into that room instead of being cut off at the line's ends.
func drawProgressPad(buf []byte, w, h, pad int, frac, amp, t, cell float64, pal [3]vcol, fg vcol, waves ...lineWave) {
	clear(buf)
	if w <= 0 || h <= 0 {
		return
	}
	mid := float64(h)/2 - 0.5
	bw := w - 2*pad // the line itself
	if bw <= 0 {
		return
	}
	head := frac * float64(bw-1)
	sway := amp * linesSway * cell
	core := math.Max(0.8, cell/22)  // a strand's half width
	glow := math.Max(2.5, cell/4.5) // how far its light reaches
	rest := fg.mul(0.55)            // the part still to play
	white := vcol{1, 1, 1}
	const strands = 3
	// How much light a row may carry: none at the picture's top and bottom.
	edge := make([]float64, h)
	for y := range h {
		edge[y] = smooth(0, float64(h)/4, math.Min(float64(y), float64(h-1-y)))
	}
	kc, kg := math.Exp(-2/(core*core)), math.Exp(-2/(glow*glow))
	par(w, func(x0, x1 int) {
		var ys [strands]float64
		// Each strand's line and light down the column, and the plain
		// line's (see gaussRow).
		var gc, gg [strands][]float64
		for i := range strands {
			gc[i], gg[i] = make([]float64, h), make([]float64, h)
		}
		mc, mg := make([]float64, h), make([]float64, h)
		for x := x0; x < x1; x++ {
			fx := float64(x - pad)
			played := fx >= 0 && fx <= head
			onLine := fx >= 0 && fx <= float64(bw-1)
			// The strands leave the middle after the start and come back
			// to it at the point where the song is.
			env := 0.0
			if played && head > 1 {
				env = smooth(0, 0.08*float64(bw)+12, fx) * smooth(0, 14, head-fx)
			}
			// The strands' light has no edges: it fades out towards both
			// ends of the played part and towards the picture's own rows.
			// It runs a little way past them, thinning out, so that it has
			// no ends either.
			lit := smooth(-3*glow, 3*glow, fx) * smooth(-3*glow, 3*glow, head-fx) * (0.4 + 0.6*amp)
			u := fx / cell
			// The wave: a crest a few cells wide, and none at the ends.
			lift := 0.0
			for _, wave := range waves {
				if wave.amp > 0 {
					d := (fx - wave.at*float64(bw)) / (2.5 * cell)
					lift -= wave.amp * cell * math.Exp(-d*d) * smooth(0, 2*cell, fx) * smooth(0, 2*cell, float64(bw-1)-fx)
				}
			}
			lift = math.Max(-0.42*cell, lift)
			for i := range strands {
				fi := float64(i)
				ys[i] = mid + env*sway*(0.62*math.Sin(u*(0.21+0.05*fi)-t*(1.3+0.35*fi)+fi*2.1)+
					0.38*math.Sin(u*0.47+t*0.9+fi*4.4)) + lift
			}
			mid := mid + lift
			if played {
				for i := range strands {
					gaussRow(gc[i], ys[i], core, kc)
					gaussRow(gg[i], ys[i], glow, kg)
				}
			} else {
				gaussRow(mc, mid, core, kc)
				gaussRow(mg, mid, glow, kg)
			}
			// Only the rows the light reaches: three widths of it past
			// the strands (and the point where the song is), beyond which
			// nothing would show.
			lo, hi := mid, mid
			if played {
				for _, y := range ys {
					lo, hi = math.Min(lo, y), math.Max(hi, y)
				}
			}
			reach := 3*math.Max(glow, 2.2*core) + 2
			for y := max(0, int(lo-reach)); y <= min(h-1, int(hi+reach)+1); y++ {
				fy := float64(y)
				var c vcol
				a := 0.0
				if played {
					// Strands that lie together look like one: as solid as
					// the most solid of them, in the mix of their colours.
					var sum vcol
					total := 0.0
					for i := range strands {
						k := gc[i][y] + 0.12*lit*gg[i][y]*edge[y]
						sum = sum.add(pal[i].mix(white, 0.18).mul(k))
						total += k
						a = math.Max(a, k)
					}
					if total > 0 {
						c = sum.mul(a / total)
					}
				} else {
					if onLine {
						k := math.Min(1, 0.75+math.Abs(lift)/cell*2) * mc[y]
						c, a = rest.mul(k), k
					}
					// The played part's light, where it runs past its ends.
					if k := 0.12 * lit * mg[y] * edge[y]; k > a {
						c, a = c.add(pal[0].mix(white, 0.18).mul(k)), k
					}
				}
				// The point where the song is.
				if dx := fx - head; math.Abs(dx) < 4*core+3 {
					r := math.Hypot(dx, fy-mid)
					k := math.Exp(-sq(r / (2.2 * core)))
					c = c.add(pal[0].mix(white, 0.6).mul(k))
					a = math.Max(a, k)
				}
				if a < 0.004 {
					continue
				}
				a = math.Min(1, a)
				// Straight alpha: the colour as it is where it is solid.
				inv := 1 / math.Max(a, 1e-3)
				i := (y*w + x) * 4
				buf[i] = byte(255 * clamp01(c.r*inv))
				buf[i+1] = byte(255 * clamp01(c.g*inv))
				buf[i+2] = byte(255 * clamp01(c.b*inv))
				buf[i+3] = byte(255 * a)
			}
		}
	})
}

// lyrBand is a band of the lyrics' picture as last sent.
type lyrBand struct {
	img            gfx.Image
	prev           []byte
	shown          bool
	col, row, w, h int
	sentAt         time.Time
}

// The lyrics' bands are lyricBandRows rows of cells high, their pictures
// numbered from lyricBandID.
const (
	lyricBandRows = 2
	lyricBandID   = 400
)

// sendBands sends the lyrics' picture pix (w×h, placed as st is) in bands,
// each only when it changed or has not been sent for a second (or all,
// with force).
func (l *uiLines) sendBands(pix []byte, w, h int, st *lineStrip, now time.Time, force bool) string {
	band := lyricBandRows * l.cellH
	n := (h + band - 1) / band
	var sb strings.Builder
	for len(l.lyrBands) > n {
		last := l.lyrBands[len(l.lyrBands)-1]
		if last.shown {
			sb.WriteString(last.img.Delete())
		}
		l.lyrBands = l.lyrBands[:len(l.lyrBands)-1]
	}
	for len(l.lyrBands) < n {
		l.lyrBands = append(l.lyrBands, lyrBand{img: gfx.Image{ID: lyricBandID + len(l.lyrBands), Z: st.img.Z, Alpha: true}})
	}
	for k := range n {
		b := &l.lyrBands[k]
		y0, y1 := k*band, min(h, (k+1)*band)
		part := pix[y0*w*4 : y1*w*4]
		row, rows := st.row+k*lyricBandRows, (y1-y0+l.cellH-1)/l.cellH
		if !force && b.shown && b.col == st.col && b.row == row && b.w == w && b.h == y1-y0 &&
			now.Sub(b.sentAt) < time.Second && bytes.Equal(part, b.prev) {
			continue
		}
		one, err := b.img.Frame(l.mode, part, w, y1-y0, st.col, row, st.cols, rows)
		if err != nil || one == "" {
			continue
		}
		sb.WriteString(one)
		if l.sentPx != nil {
			l.sentPx["lyrics"] += len(part)
			l.sentPx["lyrics#"]++
		}
		b.prev = append(b.prev[:0], part...)
		b.shown, b.col, b.row, b.w, b.h, b.sentAt = true, st.col, row, w, y1-y0, now
	}
	return sb.String()
}

// lyrLine is a line of the lyrics on screen as a picture of its own.
type lyrLine struct {
	img     gfx.Image
	key     int    // the line it shows (see lyricItem)
	what    string // what its picture is: the line's words and how they are cut
	itemSig string // how the line looked when it was drawn
	drawnY  int    // where the line's top was then, down the lyrics' picture
	at      [2]int // where its picture was put, in pixels on the screen
	shown   bool
	used    bool
	sentAt  time.Time
}

const (
	// lyricLineID numbers the lines' pictures.
	lyricLineID = 440
	// lyricMeltStep is how far, in pixels, a line in the edges' melting
	// glides before it is drawn again: its fade is spread over a seventh
	// of the height, two pixels' worth of it do not show.
	lyricMeltStep = 2
)

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// sendLyricLines shows the lyrics (l.lyrItems, see lyricsPic.frame) a line
// a picture, under the strip st (w×h pixels). A line that only glides is
// moved, not sent again: of the lyrics only what changes how a line looks
// goes to the terminal (the light running along the line sung, a line
// taking or losing its light, and in the edges' melting, how far down it
// is), drawn again every other frame and moved on the others; a line in
// the melting only once it has moved a few pixels. New words are drawn at
// once, and force draws every line again.
func (l *uiLines) sendLyricLines(st *lineStrip, w, h int, now time.Time, force bool) string {
	var sb strings.Builder
	fh := float64(h)
	fullTop, fullBot := 0.3*fh, fh-1-0.3*fh
	for k := range l.lyrLines {
		l.lyrLines[k].used = false
	}
	slot := func(key int) int {
		for k := range l.lyrLines {
			if !l.lyrLines[k].used && l.lyrLines[k].key == key && l.lyrLines[k].shown {
				return k
			}
		}
		for k := range l.lyrLines {
			if !l.lyrLines[k].used && !l.lyrLines[k].shown {
				return k
			}
		}
		l.lyrLines = append(l.lyrLines, lyrLine{img: gfx.Image{ID: lyricLineID + len(l.lyrLines), Alpha: true}})
		return len(l.lyrLines) - 1
	}
	for n := range l.lyrItems {
		it := &l.lyrItems[n]
		y0, y1, iw := it.crop(w, h)
		if y1 <= y0 || iw <= 0 {
			continue
		}
		k := slot(it.key)
		ln := &l.lyrLines[k]
		if ln.key != it.key {
			ln.shown, ln.what = false, "" // another line's picture: drawn anew
		}
		ln.used, ln.key = true, it.key
		what := fmt.Sprintf("%s|%d %d %d", it.text, y0-it.y, y1-y0, iw)
		inFade := it.key >= 0 && (float64(it.y) < fullTop || float64(it.y+it.h-1) > fullBot)
		px, py := st.col*l.cellW, st.row*l.cellH+y0
		ln.img.Z, ln.img.Native = st.img.Z, true
		ln.img.OffX, ln.img.OffY = px%l.cellW, py%l.cellH
		col, row := px/l.cellW, py/l.cellH
		at := [2]int{px, py}
		look := ln.itemSig != it.sig || inFade && absInt(it.y-ln.drawnY) >= lyricMeltStep
		var send bool
		switch {
		case force || !ln.shown || ln.what != what || now.Sub(ln.sentAt) > time.Second:
			send = true
		case look:
			send = (l.frameN+k)%linesHalfRate == 0
		}
		if !send {
			if ln.at != at {
				sb.WriteString(ln.img.Place(col, row, 0, 0))
				ln.at = at
			}
			continue
		}
		hh := y1 - y0
		if cap(l.lyrBuf) < iw*hh*4 {
			l.lyrBuf = make([]byte, iw*hh*4)
		}
		buf := l.lyrBuf[:iw*hh*4]
		clear(buf)
		it.paint(buf, iw, hh, it.y-y0, y0, h)
		one, err := ln.img.Frame(l.mode, buf, iw, hh, col, row, 0, 0)
		if err != nil || one == "" {
			continue
		}
		sb.WriteString(one)
		if l.sentPx != nil {
			l.sentPx["lyrics"] += len(buf)
			l.sentPx["lyrics#"]++
		}
		ln.shown, ln.what, ln.itemSig, ln.drawnY, ln.at, ln.sentAt = true, what, it.sig, it.y, at, now
	}
	for k := range l.lyrLines {
		if ln := &l.lyrLines[k]; !ln.used && ln.shown {
			sb.WriteString(ln.img.Delete())
			ln.shown, ln.what = false, ""
		}
	}
	return sb.String()
}

// lyrLinesShown reports whether any of the lyrics' lines is on screen as a
// picture of its own.
func (l *uiLines) lyrLinesShown() bool {
	for k := range l.lyrLines {
		if l.lyrLines[k].shown {
			return true
		}
	}
	return false
}

// lyrLinesDelete takes the lyrics' lines off the screen.
func (l *uiLines) lyrLinesDelete() string {
	var sb strings.Builder
	for k := range l.lyrLines {
		if ln := &l.lyrLines[k]; ln.shown {
			sb.WriteString(ln.img.Delete())
			ln.shown, ln.what = false, ""
		}
	}
	return sb.String()
}

// bandsDelete takes the lyrics' bands off the screen.
func (l *uiLines) bandsDelete() string {
	var sb strings.Builder
	for k := range l.lyrBands {
		if b := &l.lyrBands[k]; b.shown {
			sb.WriteString(b.img.Delete())
			b.shown, b.prev = false, b.prev[:0]
		}
	}
	return sb.String()
}

// alphaRows is the first and the last-plus-one row of pix (w×h, four bytes
// a pixel) with anything in it.
func alphaRows(pix []byte, w, h int) (y0, y1 int) {
	row := func(y int) bool {
		for i := y*w*4 + 3; i < (y+1)*w*4; i += 4 {
			if pix[i] != 0 {
				return true
			}
		}
		return false
	}
	for y0 < h && !row(y0) {
		y0++
	}
	y1 = h
	for y1 > y0 && !row(y1-1) {
		y1--
	}
	return y0, y1
}

// alphaCols is the first and the last-plus-one column of pix (w×h, four
// bytes a pixel) with anything in it.
func alphaCols(pix []byte, w, h int) (x0, x1 int) {
	x0, x1 = w, 0
	for y := range h {
		r := pix[y*w*4 : (y+1)*w*4]
		for x := 0; x < x0; x++ {
			if r[x*4+3] != 0 {
				x0 = x
				break
			}
		}
		for x := w - 1; x >= x1; x-- {
			if r[x*4+3] != 0 {
				x1 = x + 1
				break
			}
		}
	}
	return x0, x1
}

// gaussRow fills g[y] with e^(-((y-c)/s)²), k being e^(-2/s²): from the row
// nearest c each way, every row's value is the last one's times a ratio
// that is itself the last ratio times k. Three e^x for the whole column
// instead of one a row, the same values to many digits (far rows, too
// small to matter, may come out nothing).
func gaussRow(g []float64, c, s, k float64) {
	n := len(g)
	if n == 0 {
		return
	}
	is2 := 1 / (s * s)
	y0 := min(n-1, max(0, int(math.Round(c))))
	d := float64(y0) - c
	v0 := math.Exp(-d * d * is2)
	g[y0] = v0
	v, r := v0, math.Exp(-(2*d+1)*is2)
	for y := y0 + 1; y < n; y++ {
		v *= r
		r *= k
		g[y] = v
	}
	v, r = v0, math.Exp((2*d-1)*is2)
	for y := y0 - 1; y >= 0; y-- {
		v *= r
		r *= k
		g[y] = v
	}
}

// linesCoverAt notes where the cover goes on the screen being laid out and
// reports whether it will be drawn as a picture (then its cells stay
// empty).
func (m *Model) linesCoverAt(col, row, cols, rows int) bool {
	if !m.pixCover() || m.artwork.img == nil || cols < 4 || rows < 2 {
		return false
	}
	l := m.lines
	l.coverWant = true
	l.coverCol, l.coverRow, l.coverCols, l.coverRows = col, row, cols, rows
	return true
}

// coverSize notes the size of a cover whose place a mark will tell (see
// linesCoverMark).
func (l *uiLines) coverSize(cols, rows int) bool {
	if cols < 4 || rows < 2 {
		return false
	}
	l.coverWant = true
	l.coverCol, l.coverRow, l.coverCols, l.coverRows = -1, 0, cols, rows
	return true
}

// faded is pix with every pixel k (0..1) as solid as it was, in a buffer of
// the lines' own.
func (l *uiLines) faded(pix []byte, k float64) []byte {
	if len(l.scratch) < len(pix) {
		l.scratch = make([]byte, len(pix))
	}
	out := l.scratch[:len(pix)]
	copy(out, pix)
	k = clamp01(k)
	for j := 3; j < len(out); j += 4 {
		out[j] = byte(float64(out[j]) * k)
	}
	return out
}

// lyricsPicOn reports whether the cover view's lyrics are drawn as a
// picture: where the lines are pictures and a font was found.
func (m *Model) lyricsPicOn() bool {
	if !m.linesOn() || m.cfg.UIText == config.PartCells {
		return false
	}
	l := m.lines
	if l.cellW <= 0 || l.cellH <= 0 {
		l.cellW, l.cellH = cellPixels()
		if l.cellW <= 0 || l.cellH <= 0 {
			l.cellW, l.cellH = 9, 18
		}
	}
	return l.lyr.ready(l.cellH, m.cfg.LyricsScale())
}

// linesLyricsAt notes where the lyrics' picture goes on the screen being
// laid out.
func (m *Model) linesLyricsAt(col, row, cols, rows int) {
	if l := m.lines; l != nil && cols >= 12 && rows >= 4 {
		l.lyrWant = true
		l.lyrCol, l.lyrRow, l.lyrCols, l.lyrRows = col, row, cols, rows
	}
}
