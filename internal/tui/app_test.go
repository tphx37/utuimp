package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
)

func testApp(t *testing.T) (*App, []*Service) {
	t.Helper()
	y, a := newModel(newMockPlayer()), newModel(newMockPlayer())
	y.introStep, a.introStep = introDone, introDone
	started := map[string]int{}
	services := []*Service{
		{Name: "Yandex Music", Key: "yandex", Hotkey: "1", Model: y, Start: func(func(tea.Msg)) { started["yandex"]++ }},
		{Name: "Apple Music", Key: "apple", Hotkey: "2", Model: a, Start: func(func(tea.Msg)) { started["apple"]++ }},
		{Name: "Spotify", Key: "spotify", Hotkey: "3", Placeholder: "Coming soon."},
	}
	app := NewApp(services, 0)
	app.Keys = map[string]int{"1": 0, "2": 1, "3": 2, "`": 2, "ё": 2}
	app.Init()
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return app, services
}

func TestApp_HotkeysSwitchTabs(t *testing.T) {
	app, services := testApp(t)
	app.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if app.Active() != 1 || !services[1].started {
		t.Fatalf("2 should open Apple Music (active=%d)", app.Active())
	}
	if services[1].Model.height != 39 {
		t.Errorf("tab body height = %d, want 39 (one row for tabs)", services[1].Model.height)
	}
	app.Update(tea.KeyPressMsg{Code: 'ё', Text: "ё"})
	if app.Active() != 2 {
		t.Fatalf("ё should open the third tab")
	}
	screen := regexp.MustCompile(`\x1b\[[0-9;:]*m`).ReplaceAllString(app.View().Content, "")
	if !strings.Contains(screen, "Coming soon.") || !strings.Contains(screen, "1 Yandex Music") {
		t.Errorf("placeholder tab:\n%s", screen)
	}
	app.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if app.Active() != 0 {
		t.Error("1 should go back to Yandex")
	}
	// Tab no longer switches.
	app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if app.Active() != 0 {
		t.Error("tab must not switch services")
	}
}

func TestApp_HotkeysIgnoredWhileTyping(t *testing.T) {
	app, services := testApp(t)
	services[0].Model.mode = modeSearch
	app.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if app.Active() != 0 {
		t.Error("digits typed into search must not switch tabs")
	}
}

func TestApp_RoutesMessagesToTheirService(t *testing.T) {
	app, services := testApp(t)
	app.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	tr := &provider.Track{ID: "y1", Title: "Yandex song"}
	// A message tagged for Yandex reaches Yandex even while Apple is shown.
	app.Update(sessionMsg{idx: 0, msg: playerStateMsg(player.State{Track: tr, Playing: true})})
	if services[0].Model.playerState.Track == nil || services[1].Model.playerState.Track != nil {
		t.Error("state should land in the Yandex session only")
	}
	// Commands from a session come back tagged for it.
	cmd := wrap(1, func() tea.Msg { return DebugLogMsg("x") })
	if sm, ok := cmd().(sessionMsg); !ok || sm.idx != 1 {
		t.Errorf("wrapped msg = %#v", cmd())
	}
	if _, ok := wrap(1, tea.Quit)().(tea.QuitMsg); !ok {
		t.Error("quit must reach the runtime untagged")
	}
}
