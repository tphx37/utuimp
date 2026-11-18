package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/settings"
)

func settingsKey(m *SettingsModel, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		msg := tea.KeyPressMsg{Text: k}
		switch k {
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "right":
			msg = tea.KeyPressMsg{Code: tea.KeyRight}
		}
		cmd = m.Update(msg)
	}
	return cmd
}

func TestSettingsAppleNeedsTokenChoice(t *testing.T) {
	i18n.Set("en")
	cfg := &config.Config{Services: []string{"local"}}
	m := NewSettings(cfg)
	m.SetSize(80, 30)

	// Apple Music can't be turned on before saying whose token it runs on.
	if cmd := settingsKey(m, "down", "enter"); cmd != nil || strings.Join(cfg.Services, ",") != "local" {
		t.Fatalf("apple turned on without a token choice: %v", cfg.Services)
	}
	if !strings.Contains(m.View(), "choose whose developer token") {
		t.Errorf("the reason should be shown:\n%s", m.View())
	}
	cmd := settingsKey(m, "down", "right")
	if cfg.AppleTokenSource != config.AppleTokenOwn || cmd == nil {
		t.Fatalf("token source = %q", cfg.AppleTokenSource)
	}
	if msg, ok := cmd().(SettingsChangedMsg); !ok || !msg.Restart || msg.Key != "services.apple_token" {
		t.Errorf("msg = %#v", cmd())
	}
	settingsKey(m, "up", "enter")
	if strings.Join(cfg.Services, ",") != "apple,local" || !m.Pending {
		t.Errorf("services = %v pending = %v", cfg.Services, m.Pending)
	}
	if !strings.Contains(m.View(), "restart utuimp") {
		t.Errorf("a restart should be offered:\n%s", m.View())
	}
}

func TestSettingsScrollKeepsRowsInOrder(t *testing.T) {
	i18n.Set("en")
	cfg := &config.Config{Services: []string{"local"}}
	m := NewSettings(cfg)
	m.SetSize(80, 17)
	for m.current().Key != "interface.theme" {
		settingsKey(m, "down")
	}
	v := m.View()
	for _, want := range []string{"Music folder", "Interface", "Language", "› Theme"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
	if strings.Count(v, "\n") > 17 {
		t.Errorf("taller than the panel (%d lines):\n%s", strings.Count(v, "\n"), v)
	}
	// Text items ask for a line of text and store what comes back.
	for m.current().Key != "services.music_dir" {
		settingsKey(m, "up")
	}
	if _, ok := settingsKey(m, "enter")().(SettingsEditMsg); !ok {
		t.Fatal("enter on a text item should ask for text")
	}
	m.SetText("services.music_dir", " /music ")
	if cfg.MusicDir != "/music" {
		t.Errorf("music dir = %q", cfg.MusicDir)
	}
}

// A choice whose value is its own label (a visualizer scene, a background)
// is shown translated, like the labelled ones.
func TestSettingsValuesTranslated(t *testing.T) {
	i18n.Set("ru")
	t.Cleanup(func() { i18n.Set("en") })
	m := NewSettings(&config.Config{VisualizerScene: "ribbons", VisualizerBackground: "stars"})
	for _, sec := range settings.Sections() {
		for i := range sec.Items {
			it := &sec.Items[i]
			switch it.Key {
			case "visualizer.scene":
				if got := m.value(it); got != "ленты" {
					t.Errorf("the scene shows as %q", got)
				}
			case "visualizer.background":
				if got := m.value(it); got != "звёзды" {
					t.Errorf("the background shows as %q", got)
				}
			}
		}
	}
}
