package views

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/settings"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// SettingsChangedMsg says a setting was changed (the config already has the
// new value and is to be saved and applied).
type SettingsChangedMsg struct {
	Key     string
	Restart bool // takes effect after a restart
}

// SettingsEditMsg asks for a line of text for the item; the answer goes to
// SettingsModel.SetText.
type SettingsEditMsg struct {
	Key, Title, Value string
}

// SettingsActionMsg asks to do what an Action item stands for.
type SettingsActionMsg struct{ Key string }

// SettingsRestartMsg asks to restart utuimp so the changes take effect.
type SettingsRestartMsg struct{}

type settingsRow struct {
	header string         // a section title, or
	item   *settings.Item // a setting
}

// SettingsModel is the settings panel: every setting in one list, grouped
// under section titles, changed in place.
type SettingsModel struct {
	cfg      *config.Config
	sections []settings.Section
	rows     []settingsRow
	cursor   int // index into rows, always on an item
	top      int
	width    int
	height   int
	note     string // why the last change was refused
	// Values replaces what an item shows as its value (by key), for state
	// that isn't in the config: a sign-in under way.
	Values map[string]string
	// Welcome shows a greeting over the list (the first run).
	Welcome bool
	// Pending is set once something that needs a restart was changed.
	Pending bool
}

func NewSettings(cfg *config.Config) *SettingsModel {
	m := &SettingsModel{cfg: cfg, sections: settings.Sections(), Values: map[string]string{}}
	m.rebuild()
	return m
}

func (m *SettingsModel) SetSize(w, h int) { m.width, m.height = w, h }

// rebuild lists the rows again (what is hidden depends on the config).
func (m *SettingsModel) rebuild() {
	var key string
	if m.cursor < len(m.rows) && m.rows[m.cursor].item != nil {
		key = m.rows[m.cursor].item.Key
	}
	m.rows = m.rows[:0]
	for si := range m.sections {
		sec := &m.sections[si]
		m.rows = append(m.rows, settingsRow{header: sec.Title})
		for ii := range sec.Items {
			it := &sec.Items[ii]
			if m.cfg != nil && it.Hidden != nil && it.Hidden(m.cfg) {
				continue
			}
			m.rows = append(m.rows, settingsRow{item: it})
		}
	}
	m.cursor = 1
	for i, r := range m.rows {
		if r.item != nil && r.item.Key == key {
			m.cursor = i
		}
	}
}

// Open puts the cursor on the first item of the section with that key.
func (m *SettingsModel) Open(section string) {
	for i, r := range m.rows {
		if r.item != nil && strings.HasPrefix(r.item.Key, section+".") {
			m.cursor = i
			return
		}
	}
}

// SetPending marks that something changed which needs a restart.
func (m *SettingsModel) SetPending() { m.Pending = true }

func (m *SettingsModel) move(d int) {
	for i := m.cursor + d; i >= 0 && i < len(m.rows); i += d {
		if m.rows[i].item != nil {
			m.cursor = i
			return
		}
	}
}

func (m *SettingsModel) current() *settings.Item {
	if m.cursor < len(m.rows) {
		return m.rows[m.cursor].item
	}
	return nil
}

func (m *SettingsModel) set(it *settings.Item, v string) tea.Cmd {
	if it.Blocked != nil {
		if why := it.Blocked(m.cfg); why != "" {
			m.note = i18n.T(why)
			return nil
		}
	}
	if v == it.Get(m.cfg) {
		return nil
	}
	it.Set(m.cfg, v)
	if it.Restart {
		m.Pending = true
	}
	m.rebuild()
	msg := SettingsChangedMsg{Key: it.Key, Restart: it.Restart}
	return func() tea.Msg { return msg }
}

// SetText stores the text typed for the item with that key.
func (m *SettingsModel) SetText(key, v string) tea.Cmd {
	for _, r := range m.rows {
		if r.item != nil && r.item.Key == key {
			return m.set(r.item, strings.TrimSpace(v))
		}
	}
	return nil
}

// step changes the selected item: the next (d > 0) or previous value.
func (m *SettingsModel) step(d int) tea.Cmd {
	it := m.current()
	if it == nil || m.cfg == nil {
		return nil
	}
	switch it.Kind {
	case settings.Toggle:
		v := settings.On
		if it.Get(m.cfg) == settings.On {
			v = settings.Off
		}
		return m.set(it, v)
	case settings.Choice:
		opts := it.Options(m.cfg)
		if len(opts) == 0 {
			return nil
		}
		cur := 0
		for i, o := range opts {
			if o.Value == it.Get(m.cfg) {
				cur = i
			}
		}
		return m.set(it, opts[(cur+d+len(opts))%len(opts)].Value)
	case settings.Action:
		msg := SettingsActionMsg{Key: it.Key}
		return func() tea.Msg { return msg }
	case settings.Text:
		msg := SettingsEditMsg{Key: it.Key, Title: i18n.T(it.Title), Value: it.Get(m.cfg)}
		return func() tea.Msg { return msg }
	}
	return nil
}

func (m *SettingsModel) Update(msg tea.KeyPressMsg) tea.Cmd {
	m.note = ""
	switch msg.String() {
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "home", "g":
		m.cursor = 0
		m.move(1)
	case "end", "G":
		m.cursor = len(m.rows)
		m.move(-1)
	case "enter", "right":
		return m.step(1)
	case "left":
		if it := m.current(); it != nil && it.Kind != settings.Text && it.Kind != settings.Action {
			return m.step(-1)
		}
	case "backspace", "delete":
		// Empties a line of text (the prompt itself never returns nothing).
		if it := m.current(); it != nil && it.Kind == settings.Text && m.cfg != nil {
			return m.set(it, "")
		}
	case "R":
		if m.Pending {
			return func() tea.Msg { return SettingsRestartMsg{} }
		}
	}
	return nil
}

func (m *SettingsModel) value(it *settings.Item) string {
	if v, ok := m.Values[it.Key]; ok && v != "" {
		return i18n.T(v)
	}
	v := it.Get(m.cfg)
	switch it.Kind {
	case settings.Action:
		return i18n.T(v)
	case settings.Toggle:
		return i18n.T(v)
	case settings.Choice:
		for _, o := range it.Options(m.cfg) {
			if o.Value == v {
				if o.Label != "" {
					return i18n.T(o.Label)
				}
				return i18n.T(o.Value) // a word that is its own label (a scene, a background) is translated too
			}
		}
		return v
	default:
		if v == "" {
			return "—"
		}
		if it.Secret {
			return strings.Repeat("•", 8)
		}
		return v
	}
}

// wrapText breaks s into lines of at most w cells, at spaces.
func wrapText(s string, w int) []string {
	if w < 8 {
		return []string{s}
	}
	var lines []string
	line := ""
	for word := range strings.FieldsSeq(s) {
		if line != "" && lipgloss.Width(line)+1+lipgloss.Width(word) > w {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func (m *SettingsModel) View() string {
	muted := styles.QueueItemMuted
	normal := lipgloss.NewStyle().Foreground(styles.ColorFg)
	header := styles.TabActive
	sel := lipgloss.NewStyle().Foreground(styles.ColorPrimary).Bold(true)
	accent := styles.ControlActive

	var sb strings.Builder
	sb.WriteString(header.Render(i18n.T("Settings")) + "\n")
	sb.WriteString(muted.Render(strings.Repeat("─", 8)) + "\n")
	if m.cfg == nil {
		return sb.String()
	}

	w := max(20, min(m.width, 76))
	head := 2
	if m.Welcome {
		for _, l := range wrapText(i18n.T("Welcome to utuimp. Turn on the services you use and set things up here; esc closes this, and , opens it again any time."), w-2) {
			sb.WriteString(normal.Render(" "+l) + "\n")
			head++
		}
	}
	// Below the list: what the selected item is about, and what is pending.
	var foot []string
	if it := m.current(); it != nil {
		if it.Help != "" {
			foot = append(foot, wrapText(i18n.T(it.Help), w-2)...)
		}
		if it.Restart {
			foot = append(foot, i18n.T("takes effect after a restart"))
		}
	}
	for i := range foot {
		foot[i] = muted.Render(" " + foot[i])
	}
	if m.note != "" {
		foot = append(foot, accent.Render(" "+m.note))
	}
	if m.Pending {
		foot = append(foot, accent.Render(" "+i18n.T("R: restart utuimp to apply the changes")))
	}

	listH := max(3, m.height-head-len(foot)-1)
	if m.cursor < m.top+1 {
		m.top = max(0, m.cursor-1) // keep the section title in sight
	}
	if m.cursor >= m.top+listH {
		m.top = m.cursor - listH + 1
	}
	for i := m.top; i < len(m.rows) && i < m.top+listH; i++ {
		r := m.rows[i]
		if r.item == nil {
			sb.WriteString(" " + accent.Render(i18n.T(r.header)) + "\n")
			continue
		}
		title, val := i18n.T(r.item.Title), m.value(r.item)
		gap := max(2, w-4-lipgloss.Width(title)-lipgloss.Width(val))
		if i == m.cursor {
			sb.WriteString(sel.Render(" › "+title) + strings.Repeat(" ", gap) + sel.Render(val) + "\n")
		} else {
			sb.WriteString(normal.Render("   "+title) + strings.Repeat(" ", gap) + muted.Render(val) + "\n")
		}
	}
	sb.WriteString("\n" + strings.Join(foot, "\n"))
	return sb.String()
}
