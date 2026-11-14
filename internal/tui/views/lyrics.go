package views

import (
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/i18n"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/tphx37/utuimp/internal/lyrics"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// LyricsSeekMsg is emitted when the user presses Enter on a synced lyric line
// to seek playback to that line's timestamp.
type LyricsSeekMsg struct {
	Position time.Duration
}

// LyricsSourceSelectedMsg is emitted when the user picks a lyrics service in
// the source picker; the model refetches the lyrics from it.
type LyricsSourceSelectedMsg struct {
	Source lyrics.Source
}

// LyricsLangSelectedMsg is emitted when the user picks a translation
// language; the model saves it and fetches a fresh translation.
type LyricsLangSelectedMsg struct {
	Code string
}

// TranslateLang is a language offered in the translation-language picker.
type TranslateLang struct {
	Code string // Google Translate code
	Name string // native name
}

// popularLangs are listed first in the language picker (keys 1–6).
var popularLangs = []TranslateLang{
	{"en", "English"},
	{"ru", "Русский"},
	{"es", "Español"},
	{"fr", "Français"},
	{"de", "Deutsch"},
	{"ja", "日本語"},
}

// otherLangs follow the popular ones in the picker.
var otherLangs = []TranslateLang{
	{"it", "Italiano"},
	{"pt", "Português"},
	{"uk", "Українська"},
	{"pl", "Polski"},
	{"tr", "Türkçe"},
	{"zh-CN", "中文 (简体)"},
	{"ko", "한국어"},
	{"ar", "العربية"},
	{"hi", "हिन्दी"},
	{"nl", "Nederlands"},
	{"sv", "Svenska"},
	{"cs", "Čeština"},
	{"el", "Ελληνικά"},
	{"he", "עברית"},
	{"vi", "Tiếng Việt"},
	{"id", "Bahasa Indonesia"},
	{"th", "ไทย"},
	{"fi", "Suomi"},
	{"no", "Norsk"},
	{"da", "Dansk"},
	{"hu", "Magyar"},
	{"ro", "Română"},
	{"kk", "Қазақ тілі"},
	{"be", "Беларуская"},
}

// TranslateLangs is every language in picker order.
var TranslateLangs = append(append([]TranslateLang{}, popularLangs...), otherLangs...)

// LyricsTranslateRequestMsg asks the model to fetch a translation of the
// current lyrics (emitted when translation is switched on and none is cached).
type LyricsTranslateRequestMsg struct{}

// LyricsModel renders a scrollable lyrics panel.
// Call SetLoading when a fetch starts, SetLyrics when it completes, and
// SetPosition on every player-state update so the current line is highlighted.
type LyricsModel struct {
	// bare drops the "Lyrics · source" header (the cover view shows the
	// words alone).
	bare bool

	lines      []lyrics.Line
	synced     bool
	loading    bool
	errMsg     string
	pos        time.Duration
	currentIdx int  // index of the currently active line (-1 = none)
	cursor     int  // index of the user-selected line (-1 = none)
	follow     bool // true when viewport/cursor follows playback automatically
	scroll     int  // index of the top visible line
	width      int
	height     int

	source     lyrics.Source // service the shown lyrics came from
	requested  lyrics.Source // service the user asked for (auto by default)
	wordSynced bool

	showTranslation bool
	translations    []string // one per line; nil until fetched
	translating     bool
	translateErr    string
	translateErrAt  time.Time // when translateErr was set (the cover view lets it fade)
	translateLang   string
	translateSrc    string // detected language of the song

	// Line-change animation: when the active line changes, the new one fades
	// in and the previous one fades out (see lineFadeIn / lineFadeOut).
	prevIdx      int
	changedAt    time.Time
	scrollTarget int              // where follow-scrolling is heading
	now          func() time.Time // injectable clock for tests

	picker       bool // a picker is open (see pickerKind)
	pickerKind   pickerKind
	pickerIdx    int
	pickerScroll int
}

type pickerKind int

const (
	pickSource pickerKind = iota
	pickLang
)

func NewLyrics() *LyricsModel {
	return &LyricsModel{currentIdx: -1, cursor: -1, prevIdx: -1, follow: true, now: time.Now}
}

// SetLoading transitions the panel into a fetching state.
func (l *LyricsModel) SetLoading() {
	l.loading = true
	l.lines = nil
	l.errMsg = ""
	l.pos = 0
	l.currentIdx = -1
	l.cursor = -1
	l.follow = true
	l.scroll = 0
	l.scrollTarget = 0
	l.prevIdx = -1
	l.changedAt = time.Time{}
	l.translations = nil
	l.translating = false
	l.translateErr = ""
}

// SetRequestedSource records which service the user picked (shown in the
// header and pre-selected in the picker).
func (l *LyricsModel) SetRequestedSource(src lyrics.Source) { l.requested = src }

// RequestedSource returns the service the user picked.
func (l *LyricsModel) RequestedSource() lyrics.Source {
	if l.requested == "" {
		return lyrics.SourceAuto
	}
	return l.requested
}

// Lines returns the currently loaded lyric lines.
func (l *LyricsModel) Lines() []lyrics.Line { return l.lines }

// TranslationOn reports whether translation display is switched on.
func (l *LyricsModel) TranslationOn() bool { return l.showTranslation }

// NeedsTranslation reports whether translation is on but not yet fetched.
func (l *LyricsModel) NeedsTranslation() bool {
	return l.showTranslation && l.translations == nil && !l.translating && l.translateErr == "" && len(l.lines) > 0
}

// SetTranslating marks a translation fetch as in flight.
func (l *LyricsModel) SetTranslating(lang string) {
	l.translating = true
	l.translateErr = ""
	l.translateLang = lang
}

// SetTranslation stores a fetched translation (one entry per line) or the
// error that prevented it.
func (l *LyricsModel) SetTranslation(tr []string, err error) {
	l.translating = false
	if err != nil {
		l.translateErr = err.Error()
		l.translateErrAt = l.now()
		l.translations = nil
		return
	}
	l.translateErr = ""
	l.translations = tr
	if l.follow && l.currentIdx >= 0 {
		l.centerOn(l.currentIdx)
	} else {
		l.ensureCursorVisible()
	}
}

// SetTranslationSource records the song's detected language for the header.
func (l *LyricsModel) SetTranslationSource(lang string) { l.translateSrc = lang }

// SetTranslateLang records the current translation language (shown in the
// header and pre-selected in the language picker).
func (l *LyricsModel) SetTranslateLang(code string) { l.translateLang = code }

// PickerOpen reports whether the source or language picker is shown.
func (l *LyricsModel) PickerOpen() bool { return l.picker }

// SetLyrics transitions the panel to the loaded (or error) state.
func (l *LyricsModel) SetLyrics(res *lyrics.Result, err error) {
	l.loading = false
	if err != nil {
		l.errMsg = err.Error()
		l.lines = nil
		return
	}
	l.lines = res.Lines
	l.synced = res.Synced
	l.source = res.Source
	l.wordSynced = res.WordSynced
	l.translations = nil
	l.translating = false
	l.translateErr = ""
	l.errMsg = ""
	l.pos = 0
	l.currentIdx = -1
	l.cursor = -1
	l.follow = true
	l.scroll = 0
	l.scrollTarget = 0
	l.prevIdx = -1
	l.changedAt = time.Time{}
}

// SetPosition updates which line (and word) is highlighted based on the
// playback position. No-op for plain (unsynced) lyrics.
func (l *LyricsModel) SetPosition(pos time.Duration) {
	if !l.synced || len(l.lines) == 0 {
		return
	}
	l.pos = pos
	// Walk forward until a line starts after pos; the one before it is current.
	idx := -1
	for i, line := range l.lines {
		if line.Start > pos {
			break
		}
		idx = i
	}
	if idx != l.currentIdx {
		l.prevIdx = l.currentIdx
		l.currentIdx = idx
		l.changedAt = l.now()
		if !l.follow && l.cursor >= 0 && idx == l.cursor {
			l.follow = true
		}
		if l.follow {
			l.cursor = idx
			l.scrollTarget = l.centerTarget(idx)
			// A far jump (seek, first load) moves at once; the usual
			// next-line advance glides.
			if d := l.scrollTarget - l.scroll; d > maxGlideRows || d < -maxGlideRows {
				l.scroll = l.scrollTarget
			}
		}
	}
	l.stepScroll()
}

// maxGlideRows is the largest scroll change animated row by row; bigger
// jumps happen at once.
const maxGlideRows = 4

// Line-change fade durations.
const (
	lineFadeIn  = 260 * time.Millisecond
	lineFadeOut = 420 * time.Millisecond
)

// stepScroll moves the viewport one row toward scrollTarget per frame while
// following playback, so a multi-row advance reads as motion.
func (l *LyricsModel) stepScroll() {
	if !l.follow || l.scroll == l.scrollTarget {
		return
	}
	if l.scroll < l.scrollTarget {
		l.scroll++
	} else {
		l.scroll--
	}
}

// fade returns animation progress 0…1 for something that started at t0.
func (l *LyricsModel) fade(d time.Duration) float64 {
	if l.changedAt.IsZero() {
		return 1
	}
	return min(max(float64(l.now().Sub(l.changedAt))/float64(d), 0), 1)
}

// easeOut softens the end of a fade.
func easeOut(t float64) float64 { return 1 - (1-t)*(1-t) }

// Back resumes auto-following the active lyric line when the user has scrolled
// away; returns false when already following so the caller can close the panel.
func (l *LyricsModel) Back() bool {
	if l.picker {
		l.picker = false
		return true
	}
	if !l.synced || l.follow || len(l.lines) == 0 {
		return false
	}
	l.follow = true
	if l.currentIdx >= 0 {
		l.cursor = l.currentIdx
		l.centerOn(l.currentIdx)
	} else {
		l.cursor = 0
		l.scroll = 0
	}
	return true
}

// translationGap is the minimum space between a lyric and its translation
// when both share a row.
const translationGap = 3

// translationFor returns the translation shown for line i, or "".
func (l *LyricsModel) translationFor(i int) string {
	if !l.showTranslation || i >= len(l.translations) {
		return ""
	}
	return l.translations[i]
}

// selPrefix is the "▸ " marker width taken by the selected line.
func (l *LyricsModel) selPrefix(i int) int {
	if !l.follow && l.synced && i == l.cursor {
		return 2
	}
	return 0
}

// translationInline reports whether line i's translation fits on the same
// row, pushed to the right edge; otherwise it goes on the row below.
func (l *LyricsModel) translationInline(i int) bool {
	tr := l.translationFor(i)
	text := l.lines[i].Text
	if text == "" {
		return true
	}
	return l.selPrefix(i)+lipgloss.Width(text)+translationGap+lipgloss.Width(tr) <= l.width
}

// lineRows is how many rows line i takes: 2 when its translation wraps
// below it, else 1.
func (l *LyricsModel) lineRows(i int) int {
	if l.translationFor(i) == "" || l.translationInline(i) {
		return 1
	}
	return 2
}

// availRows is the number of rows below the two-row header.
func (l *LyricsModel) availRows() int {
	if l.bare {
		return max(l.height, 1)
	}
	return max(l.height-2, 1)
}

// SetBare shows the lyrics without their header line.
func (l *LyricsModel) SetBare(on bool) { l.bare = on }

// visibleEnd returns the index after the last line that fits from scroll.
func (l *LyricsModel) visibleEnd(scroll int) int {
	used, i := 0, scroll
	for ; i < len(l.lines); i++ {
		r := l.lineRows(i)
		if used+r > l.availRows() {
			break
		}
		used += r
	}
	return max(i, min(scroll+1, len(l.lines)))
}

// maxScroll is the largest scroll that still fills the panel to the end.
func (l *LyricsModel) maxScroll() int {
	used, s := 0, len(l.lines)
	for s > 0 && used+l.lineRows(s-1) <= l.availRows() {
		used += l.lineRows(s - 1)
		s--
	}
	return s
}

func (l *LyricsModel) centerOn(idx int) {
	if idx >= 0 && l.height > 0 {
		l.scroll = l.centerTarget(idx)
		l.scrollTarget = l.scroll
	}
}

// centerTarget is the scroll offset that puts line idx mid-panel.
func (l *LyricsModel) centerTarget(idx int) int {
	if idx < 0 || l.height <= 0 || idx >= len(l.lines) {
		return l.scroll
	}
	above := (l.availRows() - l.lineRows(idx)) / 2
	s, used := idx, 0
	for s > 0 && used+l.lineRows(s-1) <= above {
		used += l.lineRows(s - 1)
		s--
	}
	return max(0, min(s, l.maxScroll()))
}

func (l *LyricsModel) ensureCursorVisible() {
	if l.cursor < 0 {
		return
	}
	if l.cursor < l.scroll {
		l.scroll = l.cursor
	}
	for l.scroll < l.cursor && l.visibleEnd(l.scroll) <= l.cursor {
		l.scroll++
	}
	l.scroll = max(0, min(l.scroll, l.maxScroll()))
	l.scrollTarget = l.scroll
}

// SetSize updates the panel dimensions (called on window resize).
func (l *LyricsModel) SetSize(w, h int) {
	l.width = w
	l.height = h
}

// Update handles j/k scroll/cursor input and Enter to seek when the lyrics
// panel is active.
func (l *LyricsModel) Update(msg tea.KeyPressMsg) tea.Cmd {
	if l.picker {
		return l.updatePicker(msg)
	}
	switch msg.String() {
	case "c":
		l.picker = true
		l.pickerKind = pickSource
		l.pickerIdx = 0
		l.pickerScroll = 0
		for i, src := range lyrics.Sources {
			if src == l.RequestedSource() {
				l.pickerIdx = i
			}
		}
		return nil
	case "T":
		l.picker = true
		l.pickerKind = pickLang
		l.pickerIdx = 0
		l.pickerScroll = 0
		for i, lang := range TranslateLangs {
			if strings.EqualFold(lang.Code, l.translateLang) {
				l.pickerIdx = i
			}
		}
		return nil
	case "t":
		l.showTranslation = !l.showTranslation
		if l.showTranslation {
			l.translateErr = ""
		}
		if l.NeedsTranslation() {
			return func() tea.Msg { return LyricsTranslateRequestMsg{} }
		}
		l.ensureCursorVisible()
		if l.follow && l.currentIdx >= 0 {
			l.centerOn(l.currentIdx)
		}
		return nil
	}
	if len(l.lines) == 0 {
		return nil
	}
	if l.synced {
		if l.cursor < 0 {
			l.cursor = max(0, l.currentIdx)
		}
		switch msg.String() {
		case "j", "down":
			if l.cursor < len(l.lines)-1 {
				l.cursor++
			}
			l.follow = l.cursor == l.currentIdx
			l.ensureCursorVisible()
		case "k", "up":
			if l.cursor > 0 {
				l.cursor--
			}
			l.follow = l.cursor == l.currentIdx
			l.ensureCursorVisible()
		case "g":
			l.cursor = 0
			l.follow = l.cursor == l.currentIdx
			l.scroll = 0
		case "G":
			l.cursor = len(l.lines) - 1
			l.follow = l.cursor == l.currentIdx
			l.ensureCursorVisible()
		case "enter":
			if l.cursor >= 0 && l.cursor < len(l.lines) {
				target := l.lines[l.cursor].Start
				l.follow = true
				l.currentIdx = l.cursor
				l.pos = target
				l.centerOn(l.cursor)
				return func() tea.Msg {
					return LyricsSeekMsg{Position: target}
				}
			}
		}
		return nil
	}

	maxScroll := l.maxScroll()
	switch msg.String() {
	case "j", "down":
		if l.scroll < maxScroll {
			l.scroll++
		}
	case "k", "up":
		if l.scroll > 0 {
			l.scroll--
		}
	case "g":
		l.scroll = 0
	case "G":
		l.scroll = maxScroll
	}
	return nil
}

func (l *LyricsModel) updatePicker(msg tea.KeyPressMsg) tea.Cmd {
	n, quick := len(lyrics.Sources), len(lyrics.Sources)
	if l.pickerKind == pickLang {
		n, quick = len(TranslateLangs), len(popularLangs)
	}
	switch k := msg.String(); k {
	case "j", "down", "tab":
		l.pickerIdx = (l.pickerIdx + 1) % n
	case "k", "up", "shift+tab":
		l.pickerIdx = (l.pickerIdx + n - 1) % n
	case "g", "home":
		l.pickerIdx = 0
	case "G", "end":
		l.pickerIdx = n - 1
	case "c", "T":
		l.picker = false
	case "enter":
		return l.pick(l.pickerIdx)
	default:
		if len(k) == 1 && k[0] >= '1' && int(k[0]-'1') < quick {
			return l.pick(int(k[0] - '1'))
		}
	}
	return nil
}

func (l *LyricsModel) pick(i int) tea.Cmd {
	if l.pickerKind == pickLang {
		l.picker = false
		code := TranslateLangs[i].Code
		l.translateLang = code
		l.showTranslation = true
		l.translations = nil
		l.translateErr = ""
		return func() tea.Msg { return LyricsLangSelectedMsg{Code: code} }
	}
	return l.pickSource(i)
}

func (l *LyricsModel) pickSource(i int) tea.Cmd {
	l.picker = false
	src := lyrics.Sources[i]
	l.requested = src
	return func() tea.Msg { return LyricsSourceSelectedMsg{Source: src} }
}

// karaokeEdge is how many characters the highlight's soft edge spans: the
// boundary between sung and unsung text is a short gradient rather than a
// hard cut, so the fill glides instead of jumping a whole cell at a time.
const karaokeEdge = 1.6

// litChars returns how far into line (in characters, fractional) the
// singing has reached at l.pos, from the source's word/syllable timestamps.
// Between words it holds at the end of the last word sung.
func (l *LyricsModel) litChars(line lyrics.Line, lineEnd time.Duration) float64 {
	offset := 0.0
	for i, w := range line.Words {
		n := float64(len([]rune(w.Text)))
		if l.pos < w.Start {
			return offset
		}
		end := w.Start + w.Dur
		if w.Dur <= 0 {
			switch {
			case i+1 < len(line.Words):
				end = line.Words[i+1].Start
			case lineEnd > w.Start:
				end = min(lineEnd, w.Start+time.Duration(n)*120*time.Millisecond+300*time.Millisecond)
			default:
				end = w.Start + 600*time.Millisecond
			}
		}
		if l.pos >= end {
			offset += n
			continue
		}
		// Fill the visible characters over the word's duration; a trailing
		// space lights once the word is done.
		body := float64(len([]rune(strings.TrimRight(w.Text, " \t"))))
		frac := float64(l.pos-w.Start) / float64(max(end-w.Start, time.Millisecond))
		return offset + frac*body
	}
	return offset
}

// renderActiveLine renders the line being sung. With word/syllable timing
// it is a karaoke fill: sung characters bright, unsung ones dimmed, joined by
// a soft gradient edge that advances with playback. Without word timing the
// whole line is highlighted.
func (l *LyricsModel) renderActiveLine(line lyrics.Line, lineEnd time.Duration) string {
	return l.renderActiveRange(line, lineEnd, 0, -1)
}

// lineText is the text of a line as the karaoke fill counts it: its words
// joined when it has word timing.
func lineText(line lyrics.Line) string {
	if len(line.Words) == 0 {
		return line.Text
	}
	var text strings.Builder
	for _, w := range line.Words {
		text.WriteString(w.Text)
	}
	return text.String()
}

// renderActiveRange renders runes [from, to) of the active line (to < 0:
// to the end), so a wrapped line keeps its karaoke fill on every row.
func (l *LyricsModel) renderActiveRange(line lyrics.Line, lineEnd time.Duration, from, to int) string {
	sungC, dimC, ok := styles.LyricColors()
	fadeIn := lineFadeIn
	if l.bare {
		fadeIn = coverSlide // the cover view: in step with the slide
	}
	in := easeOut(l.fade(fadeIn))
	all := []rune(lineText(line))
	if to < 0 || to > len(all) {
		to = len(all)
	}
	from = min(max(from, 0), to)
	if len(line.Words) == 0 {
		part := string(all[from:to])
		if ok {
			c := styles.LerpColor(dimC, sungC, in)
			return lipgloss.NewStyle().Bold(true).Foreground(c).Render(part)
		}
		return styles.Playing.Bold(true).Render(part)
	}
	runes := all
	lit := l.litChars(line, lineEnd)

	level := func(i int) float64 { // 0 = unsung … 1 = sung
		return min(max((lit-float64(i))/karaokeEdge+0.5, 0), 1)
	}

	var sb strings.Builder
	if !ok {
		// No RGB to blend: three palette steps (sung / edge / unsung).
		sung := styles.Playing.Bold(true)
		edge := lipgloss.NewStyle().Foreground(styles.ColorFg).Bold(true)
		dim := lipgloss.NewStyle().Foreground(styles.ColorMuted).Bold(true)
		for i := from; i < to; i++ {
			r := runes[i]
			switch v := level(i); {
			case v >= 0.75:
				sb.WriteString(sung.Render(string(r)))
			case v > 0.25:
				sb.WriteString(edge.Render(string(r)))
			default:
				sb.WriteString(dim.Render(string(r)))
			}
		}
		return sb.String()
	}

	// Group runs of identical colour so a mostly-sung line isn't one escape
	// sequence per character.
	var run []rune
	var runHex string
	flush := func() {
		if len(run) > 0 {
			sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(runHex)).Render(string(run)))
			run = run[:0]
		}
	}
	// The unsung part of the active line sits a little above the other
	// lines' grey, faded in with the line.
	unsungC := styles.LerpColor(dimC, sungC, 0.22*in)
	for i := from; i < to; i++ {
		r := runes[i]
		hex := styles.ColorHex(styles.LerpColor(unsungC, sungC, level(i)))
		if hex != runHex {
			flush()
			runHex = hex
		}
		run = append(run, r)
	}
	flush()
	return sb.String()
}

// headerLine renders "Lyrics · NetEase · word sync · ⇄ ru".
func (l *LyricsModel) headerLine() string {
	muted := styles.QueueItemMuted
	out := styles.TabActive.Render(i18n.T("Lyrics"))
	var info []string
	if l.loading {
		if src := l.RequestedSource(); src != lyrics.SourceAuto {
			info = append(info, src.Label())
		}
	} else if len(l.lines) > 0 {
		if l.source != "" {
			label := l.source.Label()
			if l.RequestedSource() == lyrics.SourceAuto {
				label = "auto: " + label
			}
			info = append(info, label)
		}
		switch {
		case l.wordSynced:
			info = append(info, i18n.T("word sync"))
		case l.synced:
			info = append(info, i18n.T("line sync"))
		default:
			info = append(info, i18n.T("unsynced"))
		}
	}
	if l.showTranslation {
		switch {
		case l.translating:
			info = append(info, i18n.T("translating…"))
		case l.translateErr != "":
			info = append(info, i18n.T("no translation"))
		case l.translateLang != "" && l.translateSrc != "":
			info = append(info, l.translateSrc+" → "+l.translateLang)
		case l.translateLang != "":
			info = append(info, "⇄ "+l.translateLang)
		}
	}
	if len(info) > 0 {
		out += muted.Render(" " + strings.Join(info, " · "))
	}
	return out
}

func (l *LyricsModel) pickerView() string {
	if l.pickerKind == pickLang {
		return l.langPickerView()
	}
	muted := styles.QueueItemMuted
	var sb strings.Builder
	sb.WriteString(l.headerLine() + "\n")
	sb.WriteString(muted.Render(strings.Repeat("─", 5)) + "\n\n")
	sb.WriteString(styles.Header.Render(i18n.T("Lyrics source")) + "\n\n")
	for i, src := range lyrics.Sources {
		label := src.Label()
		switch src {
		case lyrics.SourceAuto:
			label += muted.Render(i18n.T("  best of all — word sync preferred"))
		case lyrics.SourceService:
			label += muted.Render(i18n.T("  the music service itself — exact match"))
		case lyrics.SourceLRCLIB:
			label += muted.Render(i18n.T("  lrclib.net — line sync"))
		case lyrics.SourceNetEase:
			label += muted.Render(i18n.T("  music.163.com — word sync"))
		case lyrics.SourceLyricsOvh:
			label += muted.Render(i18n.T("  lyrics.ovh — plain text"))
		}
		num := styles.KeyName.Render(string(rune('1'+i))) + " "
		if i == l.pickerIdx {
			sb.WriteString(styles.Playing.Render("▸ ") + num + styles.Selected.Render(label) + "\n")
		} else {
			sb.WriteString("  " + num + label + "\n")
		}
	}
	sb.WriteString("\n" + muted.Render(i18n.T("enter select · esc cancel")))
	return sb.String()
}

// langPickerView lists translation languages: the popular ones (keys 1–6),
// a divider, then the rest, scrolled to keep the selection visible.
func (l *LyricsModel) langPickerView() string {
	muted := styles.QueueItemMuted
	var rows []string
	selRow := 0
	for i, lang := range TranslateLangs {
		if i == len(popularLangs) {
			rows = append(rows, "  "+muted.Render(strings.Repeat("─", 20)))
		}
		num := "  "
		if i < len(popularLangs) {
			num = styles.KeyName.Render(string(rune('1'+i))) + " "
		}
		label := lang.Name + muted.Render("  "+lang.Code)
		if strings.EqualFold(lang.Code, l.translateLang) {
			label += styles.ControlActive.Render("  ✓")
		}
		if i == l.pickerIdx {
			selRow = len(rows)
			rows = append(rows, styles.Playing.Render("▸ ")+num+styles.Selected.Render(label))
		} else {
			rows = append(rows, "  "+num+label)
		}
	}

	// header (2) + title (2) + footer (2)
	avail := max(l.height-6, 3)
	if selRow < l.pickerScroll {
		l.pickerScroll = selRow
	} else if selRow >= l.pickerScroll+avail {
		l.pickerScroll = selRow - avail + 1
	}
	l.pickerScroll = max(0, min(l.pickerScroll, len(rows)-avail))
	end := min(len(rows), l.pickerScroll+avail)

	var sb strings.Builder
	sb.WriteString(l.headerLine() + "\n")
	sb.WriteString(muted.Render(strings.Repeat("─", 5)) + "\n")
	sb.WriteString(styles.Header.Render(i18n.T("Translate into")) + "\n\n")
	for _, r := range rows[l.pickerScroll:end] {
		sb.WriteString(r + "\n")
	}
	sb.WriteString("\n" + muted.Render(i18n.T("1-6 quick pick · enter select · esc cancel")))
	return sb.String()
}

// View renders the lyrics panel as a newline-separated string.
func (l *LyricsModel) View() string {
	if l.picker {
		return l.pickerView()
	}
	muted := styles.QueueItemMuted
	normal := lipgloss.NewStyle().Foreground(styles.ColorFg)
	selected := styles.Selected
	// Synced lyrics: every line but the active one is grey, the same grey
	// the active line's unsung part fades from.
	sungC, dimC, rgb := styles.LyricColors()
	grey := muted
	if rgb {
		grey = lipgloss.NewStyle().Foreground(dimC)
	}
	transStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted).Italic(true)
	transActive := lipgloss.NewStyle().Foreground(styles.ColorFg).Italic(true)
	hint := "  " + styles.KeyName.Render(i18n.T("c")) + muted.Render(i18n.T(" choose another source"))

	header := l.headerLine() + "\n" + muted.Render(strings.Repeat("─", 5)) + "\n"
	if l.bare {
		header = ""
	}

	if l.loading {
		return header + "\n" + muted.Render(i18n.T("fetching lyrics…"))
	}

	if l.errMsg != "" {
		return header + "\n" + muted.Render(i18n.T("You cannot sing this song :(")) + "\n\n" + hint
	}

	if len(l.lines) == 0 {
		return header + "\n" + muted.Render(i18n.T("no lyrics found")) + "\n\n" + hint
	}

	var sb strings.Builder
	sb.WriteString(header)

	end := l.visibleEnd(l.scroll)
	for i := l.scroll; i < end; i++ {
		line := l.lines[i]
		text := line.Text
		var main string
		switch {
		case text == "":
			if l.selPrefix(i) > 0 {
				main = selected.Render("▸ ♪")
			}
		case !l.follow && l.synced && i == l.cursor && i != l.currentIdx:
			main = selected.Render("▸ " + text)
		case i == l.currentIdx:
			if l.selPrefix(i) > 0 {
				main = selected.Render("▸ ")
			}
			lineEnd := time.Duration(0)
			if i+1 < len(l.lines) {
				lineEnd = l.lines[i+1].Start
			}
			main += l.renderActiveLine(line, lineEnd)
		case l.synced && i == l.prevIdx && rgb:
			// The line just left fades from highlighted to grey.
			c := styles.LerpColor(sungC, dimC, easeOut(l.fade(lineFadeOut)))
			main = lipgloss.NewStyle().Foreground(c).Render(text)
		case l.synced:
			main = grey.Render(text)
		default:
			main = normal.Render(text)
		}
		sb.WriteString(main)

		// Translation: right-aligned on the same row when it fits, else
		// right-aligned on the row below.
		if tr := l.translationFor(i); tr != "" && text != "" {
			st := transStyle
			switch {
			case i == l.currentIdx && rgb:
				// A touch brighter than the other translations, fading in
				// with its line.
				c := styles.LerpColor(dimC, sungC, 0.4*easeOut(l.fade(lineFadeIn)))
				st = lipgloss.NewStyle().Foreground(c).Italic(true)
			case i == l.currentIdx:
				st = transActive
			case rgb:
				st = lipgloss.NewStyle().Foreground(dimC).Italic(true)
			}
			trW := lipgloss.Width(tr)
			if l.translationInline(i) {
				pad := l.width - l.selPrefix(i) - lipgloss.Width(text) - trW
				sb.WriteString(strings.Repeat(" ", max(pad, translationGap)) + st.Render(tr))
			} else {
				if trW > l.width {
					tr = ansi.Truncate(tr, l.width, "…")
					trW = lipgloss.Width(tr)
				}
				sb.WriteString("\n" + strings.Repeat(" ", max(0, l.width-trW)) + st.Render(tr))
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
