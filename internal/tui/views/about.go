package views

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/auth"
	"github.com/tphx37/utuimp/internal/i18n"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tphx37/utuimp/internal/tui/styles"
	"github.com/tphx37/utuimp/internal/version"
)

// AboutModel renders the About & support information.
type AboutModel struct {
	width  int
	height int
	status string
}

func NewAbout() *AboutModel {
	return &AboutModel{}
}

func (a *AboutModel) SetSize(w, h int) {
	a.width = w
	a.height = h
}

// appleTokenLine says whose Apple Music developer token is in use and for
// how long; empty when Apple Music hasn't asked for one.
func appleTokenLine(now time.Time) string {
	info := auth.DeveloperTokenInfo()
	var who string
	switch info.Source {
	case auth.SourceOwn, auth.SourceBuild:
		who = i18n.T("Apple Music runs on your own developer token")
	case auth.SourceBroker:
		who = i18n.T("Apple Music runs on utuimp's developer token")
	default:
		return ""
	}
	if info.Expires.IsZero() {
		return who
	}
	left := info.Expires.Sub(now)
	if left <= 0 {
		return who + " · " + i18n.T("expired")
	}
	return who + " · " + fmt.Sprintf(i18n.T("%d d left"), int(left.Hours()/24))
}

func (a *AboutModel) Update(tea.KeyPressMsg) tea.Cmd { return nil }

func (a *AboutModel) View() string {
	muted := styles.QueueItemMuted
	normal := lipgloss.NewStyle().Foreground(styles.ColorFg)
	header := styles.TabActive
	primary := lipgloss.NewStyle().Foreground(styles.ColorPrimary).Bold(true)
	secondary := lipgloss.NewStyle().Foreground(styles.ColorSecondary)

	var sb strings.Builder
	sb.WriteString(header.Render(i18n.T("About")) + "\n")
	sb.WriteString(muted.Render(strings.Repeat("─", 5)) + "\n\n")

	contentLines := []string{
		primary.Render("utuimp"),
		muted.Render(i18n.T("universal terminal user interface media player")),
		muted.Render(fmt.Sprintf(i18n.T("version %s"), version.Version)),
		"",
		normal.Render(i18n.T("keyboard first")),
		"",
		secondary.Render(i18n.T("made by tphx37")),
		muted.Render(i18n.T("based on vibez by simonepelosi")),
		lipgloss.NewStyle().Foreground(styles.ColorSubtle).Underline(true).Render("https://github.com/tphx37/utuimp"),
	}

	if line := appleTokenLine(time.Now()); line != "" {
		contentLines = append(contentLines, "", muted.Render(line))
	}
	if a.status != "" {
		contentLines = append(contentLines, "", styles.ControlActive.Render(a.status))
	}
	// The icon on top, when there is room for it.
	if logo := aboutLogoLines(); a.height-3 >= len(contentLines)+len(logo)+1 {
		contentLines = append(append(logo, ""), contentLines...)
	}

	neededHeight := len(contentLines)
	topPad := max(0, (a.height-3-neededHeight)/2)

	for range topPad {
		sb.WriteByte('\n')
	}

	for _, line := range contentLines {
		sb.WriteString(centerStrAbout(line, a.width) + "\n")
	}

	return sb.String()
}

// aboutLogoLines draws the small icon (aboutLogo) in half-block cells: the
// upper pixel the character's colour, the lower one its background; where
// a pixel is outside the icon's rounded corners the terminal's own
// background shows.
func aboutLogoLines() []string {
	hex := func(c int32) color.Color { return lipgloss.Color(fmt.Sprintf("#%06x", c)) }
	lines := make([]string, 0, len(aboutLogo)/2)
	for y := 0; y+1 < len(aboutLogo); y += 2 {
		var sb strings.Builder
		for x := range aboutLogo[y] {
			top, bot := aboutLogo[y][x], aboutLogo[y+1][x]
			switch {
			case top < 0 && bot < 0:
				sb.WriteByte(' ')
			case top < 0:
				sb.WriteString(lipgloss.NewStyle().Foreground(hex(bot)).Render("▄"))
			case bot < 0:
				sb.WriteString(lipgloss.NewStyle().Foreground(hex(top)).Render("▀"))
			default:
				sb.WriteString(lipgloss.NewStyle().Foreground(hex(top)).Background(hex(bot)).Render("▀"))
			}
		}
		lines = append(lines, sb.String())
	}
	return lines
}

func centerStrAbout(s string, width int) string {
	w := lipgloss.Width(s)
	pad := max(0, (width-w)/2)
	return strings.Repeat(" ", pad) + s
}
