package views

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAboutModel(t *testing.T) {
	m := NewAbout()
	m.SetSize(80, 30)

	plain := stripANSI(m.View())
	for _, want := range []string{"utuimp", "universal terminal user interface media player", "keyboard first", "made by tphx37", "based on vibez by simonepelosi", "https://github.com/tphx37/utuimp"} {
		if !strings.Contains(plain, want) {
			t.Errorf("about view missing %q:\n%s", want, plain)
		}
	}
	if strings.Count(plain, "▀")+strings.Count(plain, "▄") < 200 {
		t.Errorf("the icon should be drawn on top when there's room:\n%s", plain)
	}
	short := NewAbout()
	short.SetSize(80, 12)
	if p := stripANSI(short.View()); strings.Contains(p, "▀") || !strings.Contains(p, "made by tphx37") {
		t.Errorf("with no room for the icon the text should stay, without it:\n%s", p)
	}
	if strings.Contains(plain, "ko-fi") || strings.Contains(strings.ToLower(plain), "donat") {
		t.Error("about view must not ask for donations")
	}
	// No key opens anything in the browser any more.
	for _, k := range []string{"enter", "d", "x"} {
		if cmd := m.Update(tea.KeyPressMsg{Text: k}); cmd != nil {
			t.Errorf("key %q returned a command", k)
		}
	}
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}
