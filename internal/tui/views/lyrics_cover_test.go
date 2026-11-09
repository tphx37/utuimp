package views

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/lyrics"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

var fgRGB = regexp.MustCompile(`38;2;(\d+);(\d+);(\d+)`)

// brightness of the first foreground colour in s (-1 when none).
func brightness(s string) int {
	m := fgRGB.FindStringSubmatch(s)
	if m == nil {
		return -1
	}
	r, _ := strconv.Atoi(m[1])
	g, _ := strconv.Atoi(m[2])
	b, _ := strconv.Atoi(m[3])
	return r + g + b
}

func TestCoverViewFadesSungLines(t *testing.T) {
	if _, _, ok := styles.LyricFadeColors(); !ok {
		t.Skip("theme without RGB colours")
	}
	l := NewLyrics()
	var ls []lyrics.Line
	for i := range 30 {
		ls = append(ls, lyrics.Line{Start: time.Duration(i) * time.Second, Text: fmt.Sprintf("line %02d", i)})
	}
	l.SetLyrics(&lyrics.Result{Lines: ls, Synced: true}, nil)
	l.SetPosition(15500 * time.Millisecond)
	l.changedAt = time.Time{} // no slide in progress
	rows := l.CoverView(40, 30)
	if len(rows) != 30 {
		t.Fatalf("%d rows, want 30", len(rows))
	}
	anchor := int(30 * coverAnchor)
	if !strings.Contains(rows[anchor], "line 15") {
		t.Fatalf("row %d = %q, want the current line", anchor, rows[anchor])
	}
	// Sung lines get darker the higher they are; the coming one is lit
	// more than the line just above the current.
	near, far := brightness(rows[anchor-2]), brightness(rows[1])
	if strings.TrimSpace(rows[1]) != "" && far >= near {
		t.Errorf("top row (%d) should be dimmer than the row above the current line (%d)", far, near)
	}
	if next := brightness(rows[anchor+2]); next <= near {
		t.Errorf("coming line (%d) should be brighter than the one just sung (%d)", next, near)
	}
}

func TestWrapRanges(t *testing.T) {
	got := wrapRanges("one two three four", 9)
	var parts []string
	for _, r := range got {
		parts = append(parts, string([]rune("one two three four")[r[0]:r[1]]))
	}
	if strings.Join(parts, "|") != "one two|three|four" {
		t.Errorf("wrapped = %q", parts)
	}
}

// A line change moves the text once, straight to its place (stepping it row
// by row reads as jitter), and the old line's highlight fades out.
func TestCoverViewMovesOnceAndFades(t *testing.T) {
	l := NewLyrics()
	var ls []lyrics.Line
	for i := range 20 {
		ls = append(ls, lyrics.Line{Start: time.Duration(i) * time.Second,
			Text: fmt.Sprintf("line %02d is long enough to wrap", i)})
	}
	l.SetLyrics(&lyrics.Result{Lines: ls, Synced: true}, nil)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	l.SetPosition(5500 * time.Millisecond)
	now = now.Add(time.Second)
	l.SetPosition(6500 * time.Millisecond)

	rowOf := func(rows []string, text string) int {
		for i, r := range rows {
			if strings.Contains(r, text) {
				return i
			}
		}
		return -1
	}
	want := int(30 * coverAnchor)
	var prevShades []int
	for step := 0; step <= 15; step++ {
		rows := l.CoverView(20, 30)
		if row := rowOf(rows, "line 06"); row != want {
			t.Fatalf("frame %d: line 06 at row %d, want %d from the first frame on", step, row, want)
		}
		if _, _, ok := styles.LyricFadeColors(); ok {
			prevShades = append(prevShades, brightness(rows[rowOf(rows, "line 05")]))
		}
		now = now.Add(40 * time.Millisecond)
	}
	for i := 1; i < len(prevShades); i++ {
		if prevShades[i] > prevShades[i-1] {
			t.Errorf("the sung line brightened at frame %d: %v", i, prevShades)
		}
	}
	if len(prevShades) > 1 && prevShades[0] == prevShades[len(prevShades)-1] {
		t.Errorf("the sung line didn't fade: %v", prevShades)
	}
}
