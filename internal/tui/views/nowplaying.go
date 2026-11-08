package views

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// RenderGlowTitle renders each rune with a colour based on how far the
// "bright spot" has swept past it. The spot starts before the first char,
// sweeps right, exits after the last char, then the cycle restarts.
func RenderGlowTitle(title string, glowStep int) string {
	palette := styles.GlowPalette
	pLen := len(palette)
	runes := []rune(title)
	n := len(runes)
	if n == 0 {
		return ""
	}
	// Sweep position within cycle [0, n+pLen)
	pos := glowStep % (n + pLen)
	var sb strings.Builder
	for i, r := range runes {
		dist := pos - i // positive = spot has passed char i
		var c color.Color
		if dist >= 0 && dist < pLen {
			c = palette[pLen-1-dist] // brightest when dist==0
		} else {
			c = palette[0] // dim outside the sweep window
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(c).Italic(true).Render(string(r)))
	}
	return sb.String()
}

// FormatDuration formats a duration as m:ss. Exported for use in model footer.
func FormatDuration(d time.Duration) string {
	secs := int(d.Seconds())
	mins := secs / 60
	secs %= 60
	return fmt.Sprintf("%d:%02d", mins, secs)
}

// progressGradient returns the gradient colour for position i out of total
// cells, interpolating across styles.ProgressGradStops.
func progressGradient(i, total int) color.Color {
	stops := styles.ProgressGradStops
	if total <= 1 || len(stops) < 2 {
		return stops[0]
	}
	segments := len(stops) - 1
	t := float64(i) / float64(total-1) // 0.0 … 1.0
	seg := int(t * float64(segments))
	if seg >= segments {
		return stops[len(stops)-1]
	}
	local := t*float64(segments) - float64(seg)
	return styles.LerpColor(stops[seg], stops[seg+1], local)
}

// RenderProgressBar renders a flat progress line: a heavy rule (━) for the
// elapsed part and a thin rule (─) for the rest. The elapsed part is a
// gradient in the cover's colours (or the theme's on RGB themes), and a single
// palette colour on ANSI themes when the track has no artwork palette.
func RenderProgressBar(pos, dur time.Duration, width int) string {
	if width <= 0 {
		return ""
	}
	ratio := 0.0
	if dur > 0 {
		ratio = min(float64(pos)/float64(dur), 1)
	}
	// Half-cell resolution: a left-half heavy rule (╸) at the tip lets the
	// line grow in half steps, so it creeps forward instead of jumping.
	halves := int(ratio * float64(width) * 2)
	filled := halves / 2
	half := halves%2 == 1 && filled < width

	color := func(i int) lipgloss.Style {
		if styles.ANSIMode && !styles.ArtworkActive {
			return styles.ProgressBar
		}
		return lipgloss.NewStyle().Foreground(progressGradient(i, max(filled, 1)+1))
	}

	var sb strings.Builder
	if styles.ANSIMode && !styles.ArtworkActive {
		sb.WriteString(styles.ProgressBar.Render(strings.Repeat("━", filled)))
	} else {
		for i := range filled {
			sb.WriteString(color(i).Render("━"))
		}
	}
	rest := width - filled
	if half {
		sb.WriteString(color(filled).Render("╸"))
		rest--
	}
	if rest > 0 {
		sb.WriteString(styles.ProgressBg.Render(strings.Repeat("─", rest)))
	}
	return sb.String()
}

func centerLine(s string, width int) string {
	sw := lipgloss.Width(s)
	pad := max(0, (width-sw)/2)
	return strings.Repeat(" ", pad) + s
}
