package tui

import (
	"reflect"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tphx37/utuimp/internal/tui/styles"
)

// Service is one music service shown as a tab: its own Model (queue,
// library, player…) plus a Start func that brings its engine up the first
// time the tab is opened. A service without a Model is a placeholder that
// shows Placeholder instead.
type Service struct {
	Name        string // tab label, e.g. "Yandex Music"
	Key         string // config value, e.g. "yandex"
	Model       *Model
	Placeholder string
	// Hotkey is the key that opens this tab (shown on the tab).
	Hotkey string
	// Start launches the engine in the background. send delivers messages
	// (InitStatusMsg, EngineReadyMsg, …) to this service's Model.
	Start func(send func(tea.Msg))

	started bool
}

// App shows several services as tabs and routes each message to the
// service it belongs to. Keys (1, 2, 3, ` …) switch tabs.
type App struct {
	services []*Service
	// Keys maps a key (as tea reports it, e.g. "1", "`", "ё") to a tab.
	Keys   map[string]int
	active int
	width  int
	height int
	send   func(tea.Msg)
	// OnSwitch is called with the new tab index (to persist it, refocus
	// media keys, …).
	OnSwitch func(idx int)
}

// sessionMsg tags a message with the service it belongs to.
type sessionMsg struct {
	idx int
	msg tea.Msg
}

// NewApp builds the tabbed app; active is the initially shown service.
func NewApp(services []*Service, active int) *App {
	a := &App{services: services, active: min(max(active, 0), len(services)-1)}
	for i, s := range services {
		if s.Model != nil {
			s.Model.background = i != a.active
		}
	}
	return a
}

// SetSend gives the app the program's Send (call after tea.NewProgram).
func (a *App) SetSend(send func(tea.Msg)) { a.send = send }

// Sender returns a func that delivers messages to service idx from outside
// the program (engine goroutines).
func (a *App) Sender(idx int) func(tea.Msg) {
	return func(msg tea.Msg) {
		if a.send != nil {
			a.send(sessionMsg{idx: idx, msg: msg})
		}
	}
}

// Active returns the index of the shown service.
func (a *App) Active() int { return a.active }

var teaPkg = reflect.TypeOf(tea.QuitMsg{}).PkgPath()

// wrap makes cmd's result come back tagged for service idx. Bubble Tea's own
// messages (quit, batches, sequences, clipboard, terminal queries) pass
// through untouched so the runtime still sees them; batches are unpacked so
// their parts get tagged too.
func wrap(idx int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		switch m := msg.(type) {
		case nil:
			return nil
		case tea.BatchMsg:
			out := make(tea.BatchMsg, 0, len(m))
			for _, c := range m {
				if c != nil {
					out = append(out, wrap(idx, c))
				}
			}
			return out
		}
		if t := reflect.TypeOf(msg); t != nil && t.PkgPath() == teaPkg {
			return msg
		}
		return sessionMsg{idx: idx, msg: msg}
	}
}

func (a *App) start(idx int) tea.Cmd {
	s := a.services[idx]
	if s.started {
		return nil
	}
	s.started = true
	if s.Model == nil {
		return nil
	}
	cmds := []tea.Cmd{wrap(idx, s.Model.Init())}
	if a.width > 0 {
		_, c := s.Model.Update(tea.WindowSizeMsg{Width: a.width, Height: a.bodyHeight()})
		cmds = append(cmds, wrap(idx, c))
	}
	if s.Start != nil {
		go s.Start(a.Sender(idx))
	}
	return tea.Batch(cmds...)
}

func (a *App) Init() tea.Cmd { return a.start(a.active) }

func (a *App) bodyHeight() int { return max(0, a.height-1) }

// forward delivers msg to service idx and tags whatever it returns.
func (a *App) forward(idx int, msg tea.Msg) tea.Cmd {
	if a.services[idx].Model == nil {
		return nil
	}
	_, cmd := a.services[idx].Model.Update(msg)
	return wrap(idx, cmd)
}

func (a *App) switchTo(idx int) tea.Cmd {
	if idx == a.active || idx < 0 || idx >= len(a.services) {
		return nil
	}
	if old := a.services[a.active].Model; old != nil {
		old.background = true
	}
	a.active = idx
	if m := a.services[idx].Model; m != nil {
		m.background = false
		m.applyTrackPalette(m.playerState.Track)
	}
	if a.OnSwitch != nil {
		a.OnSwitch(idx)
	}
	return a.start(idx)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case sessionMsg:
		if m.idx >= 0 && m.idx < len(a.services) {
			return a, a.forward(m.idx, m.msg)
		}
		return a, nil
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		inner := tea.WindowSizeMsg{Width: m.Width, Height: a.bodyHeight()}
		var cmds []tea.Cmd
		for i, s := range a.services {
			if s.started {
				cmds = append(cmds, a.forward(i, inner))
			}
		}
		return a, tea.Batch(cmds...)
	case tea.ForegroundColorMsg, tea.BackgroundColorMsg:
		var cmds []tea.Cmd
		for i, s := range a.services {
			if s.started {
				cmds = append(cmds, a.forward(i, msg))
			}
		}
		return a, tea.Batch(cmds...)
	case tea.KeyPressMsg:
		k := m.String()
		cur := a.services[a.active].Model
		if idx, ok := a.Keys[k]; ok && (cur == nil || cur.canSwitchByKey()) {
			return a, a.switchTo(idx)
		}
		if cur == nil {
			if k == "ctrl+c" || k == "q" {
				return a, tea.Quit
			}
			return a, nil
		}
	}
	return a, a.forward(a.active, msg)
}

// tabBar renders the service tabs.
func (a *App) tabBar() string {
	var parts []string
	for i, s := range a.services {
		label := " " + s.Name + " "
		if s.Hotkey != "" {
			label = " " + s.Hotkey + " " + s.Name + " "
		}
		if m := s.Model; m != nil && m.playerState.Playing {
			label += "♪ "
		}
		st := lipgloss.NewStyle().Foreground(styles.ColorMuted)
		if i == a.active {
			st = lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(styles.ColorBorderFocus)
		}
		parts = append(parts, st.Render(label))
	}
	bar := " " + strings.Join(parts, " ")
	hint := ""
	pad := max(1, a.width-lipgloss.Width(bar)-lipgloss.Width(hint))
	return bar + strings.Repeat(" ", pad) + hint
}

func (a *App) View() tea.View {
	s := a.services[a.active]
	if s.Model == nil {
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.ColorBorderFocus).
			Padding(1, 4).
			Render(lipgloss.NewStyle().Bold(true).Render(s.Name) + "\n\n" + styles.QueueItemMuted.Render(s.Placeholder))
		body := lipgloss.Place(a.width, a.bodyHeight(), lipgloss.Center, lipgloss.Center, box)
		v := tea.NewView(a.tabBar() + "\n" + body)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		v.WindowTitle = "utuimp · " + s.Name
		return v
	}
	inner := s.Model.View()
	if a.width == 0 {
		return inner
	}
	v := tea.NewView(a.tabBar() + "\n" + inner.Content)
	v.AltScreen = true
	v.WindowTitle = inner.WindowTitle
	v.MouseMode = inner.MouseMode
	return v
}
