package views

import (
	"fmt"
	"github.com/tphx37/utuimp/internal/i18n"
	"strings"

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
		normal.Render(i18n.T("Keyboard-first.")),
		"",
		secondary.Render(i18n.T("made by tphx37")),
		muted.Render(i18n.T("based on vibez by Simone Pelosi")),
		lipgloss.NewStyle().Foreground(styles.ColorSubtle).Underline(true).Render("https://github.com/tphx37/utuimp"),
	}

	if a.status != "" {
		contentLines = append(contentLines, "", styles.ControlActive.Render(a.status))
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

func centerStrAbout(s string, width int) string {
	w := lipgloss.Width(s)
	pad := max(0, (width-w)/2)
	return strings.Repeat(" ", pad) + s
}
