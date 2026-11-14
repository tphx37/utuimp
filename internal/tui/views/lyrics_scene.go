package views

import (
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/i18n"
)

// CoverSceneLine is one line of the lyrics, with its translation when that
// is shown.
type CoverSceneLine struct {
	Text, Translation string
}

// CoverScene is the cover view's lyrics as things to draw rather than rows
// of cells: for a terminal where they are a picture (see the tui package's
// lyrics picture), drawn in a font and at a size of its own.
type CoverScene struct {
	// Message, when set, is all there is to show (loading, no lyrics),
	// with Hint under it.
	Message, Hint string
	Status        string // a note on what the lyrics are doing ("translating…")
	Lines         []CoverSceneLine
	Focus         int  // the line the view is built round
	Current       int  // the line being sung, -1 for none
	Cursor        int  // the line picked while browsing, -1 while following
	Synced        bool // the lines have times
	// Lit is how many letters of the current line are sung (a fraction
	// while one is being sung); -1 when the line has no word times, and
	// lights up whole.
	Lit float64
}

// CoverScene describes the lyrics for the cover view.
func (l *LyricsModel) CoverScene() CoverScene {
	s := CoverScene{Current: -1, Cursor: -1, Lit: -1, Synced: l.synced}
	switch {
	case l.loading:
		s.Message = i18n.T("fetching lyrics…")
		return s
	case l.errMsg != "", len(l.lines) == 0:
		s.Message = i18n.T("no lyrics found")
		if l.errMsg != "" {
			s.Message = i18n.T("You cannot sing this song :(")
		}
		s.Hint = i18n.T("c") + i18n.T(" choose another source")
		return s
	}
	s.Lines = make([]CoverSceneLine, len(l.lines))
	for i, line := range l.lines {
		s.Lines[i].Text = lineText(line)
		if tr := l.translationFor(i); tr != "" && strings.TrimSpace(s.Lines[i].Text) != "" {
			s.Lines[i].Translation = tr
		}
	}
	focus := l.scroll
	switch {
	case !l.follow && l.cursor >= 0:
		focus = l.cursor
		s.Cursor = l.cursor
	case l.synced:
		focus = max(l.currentIdx, 0)
	}
	s.Focus = min(max(focus, 0), len(l.lines)-1)
	if l.synced && l.currentIdx >= 0 && l.currentIdx < len(l.lines) {
		s.Current = l.currentIdx
		if line := l.lines[l.currentIdx]; len(line.Words) > 0 {
			lineEnd := time.Duration(0)
			if l.currentIdx+1 < len(l.lines) {
				lineEnd = l.lines[l.currentIdx+1].Start
			}
			s.Lit = l.litChars(line, lineEnd)
		}
	}
	switch {
	case l.showTranslation && l.translating:
		s.Status = i18n.T("translating…")
	case l.showTranslation && l.translateErr != "" && l.now().Sub(l.translateErrAt) < noteShow:
		s.Status = i18n.T("no translation")
	}
	return s
}
