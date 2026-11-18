package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

func TestAppAMOLEDBackground(t *testing.T) {
	app, services := testApp(t)
	t.Cleanup(func() { styles.BlackBackground = false })
	services[0].Model.cfg.AMOLED = true
	styles.BlackBackground = true
	if app.View().BackgroundColor == nil {
		t.Fatal("the tabbed app should retain the model's black background")
	}
	app.active = 2
	if app.View().BackgroundColor == nil {
		t.Fatal("placeholder tabs should retain the black background")
	}
}

func testApp(t *testing.T) (*App, []*Service) {
	t.Helper()
	y, a := newModel(newMockPlayer()), newModel(newMockPlayer())
	y.introStep, a.introStep = introDone, introDone
	// (Each tab's Start runs on a goroutine of its own: it must not
	// touch anything the test shares.)
	services := []*Service{
		{Name: "Yandex Music", Key: "yandex", Hotkey: "1", Model: y, Start: func(func(tea.Msg)) {}},
		{Name: "Apple Music", Key: "apple", Hotkey: "2", Model: a, Start: func(func(tea.Msg)) {}},
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

// runCmd runs cmd and the commands it batches, returning their messages.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, runCmd(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// update sends msg to the app and runs what it returns.
func update(app *App, msg tea.Msg) {
	_, cmd := app.Update(msg)
	runCmd(cmd)
}

// One volume for all players: a change in one tab reaches the others,
// mute included, and a tab opened later starts with it.
func TestApp_VolumeIsShared(t *testing.T) {
	app, services := testApp(t)
	yandex := services[0].Model
	apple := services[1].Model
	applePlayer := apple.player.(*mockPlayer)

	update(app, sessionMsg{idx: 0, msg: saveVolumeMsg{vol: 0.3}})
	if applePlayer.volume != nil {
		t.Fatal("a tab that hasn't started yet has no player to set")
	}
	app.switchTo(1) // Apple starts: it takes the shared volume (its start-up commands aren't run)
	if apple.playerState.Volume != 0.3 {
		t.Errorf("apple volume = %v, want 0.3", apple.playerState.Volume)
	}

	update(app, sessionMsg{idx: 1, msg: saveVolumeMsg{muted: true, preMute: 0.3}})
	if yandex.preMuteVol != 0.3 || yandex.player.(*mockPlayer).volume == nil || *yandex.player.(*mockPlayer).volume != 0 {
		t.Errorf("mute in Apple should mute Yandex: preMute %v, volume %v", yandex.preMuteVol, yandex.player.(*mockPlayer).volume)
	}
	update(app, sessionMsg{idx: 1, msg: saveVolumeMsg{vol: 0.6}})
	if yandex.preMuteVol != -1 || *yandex.player.(*mockPlayer).volume != 0.6 {
		t.Errorf("unmute should reach Yandex: preMute %v, volume %v", yandex.preMuteVol, *yandex.player.(*mockPlayer).volume)
	}
}

// The visualizer takes the whole screen, the tab bar's row included, and
// the tab keys do nothing while it is open.
func TestApp_VisualizerHidesTheTabs(t *testing.T) {
	app, services := testApp(t)
	app.Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
	if !services[0].Model.vizOpen() {
		t.Fatal("V should open the visualizer")
	}
	content := app.View().Content
	if strings.Contains(content, "Apple Music") {
		t.Error("the tab bar shows over the visualizer")
	}
	if n := strings.Count(content, "\n") + 1; n != 40 {
		t.Errorf("the visualizer is %d rows tall, want all 40", n)
	}
	app.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if app.Active() != 0 || !services[0].Model.vizOpen() {
		t.Errorf("2 switched tabs from inside the visualizer (active=%d)", app.Active())
	}
	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !strings.Contains(app.View().Content, "Apple Music") {
		t.Error("the tab bar should be back after esc")
	}
	app.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if app.Active() != 1 {
		t.Error("2 should switch tabs again after esc")
	}
}

// The start screen is shown once, on the tab the app opens on: leaving that
// tab while it is up ends it, and a tab opened later has none (its own
// would dance over the tabs until its service was ready, for ever if it
// never is).
func TestApp_StartScreenOnce(t *testing.T) {
	y, a := newModel(newMockPlayer()), newModel(newMockPlayer())
	y.player, a.player = nil, nil // services still starting
	services := []*Service{
		{Name: "Yandex Music", Key: "yandex", Hotkey: "1", Model: y, Start: func(func(tea.Msg)) {}},
		{Name: "Apple Music", Key: "apple", Hotkey: "2", Model: a, Start: func(func(tea.Msg)) {}},
	}
	app := NewApp(services, 0)
	app.Keys = map[string]int{"1": 0, "2": 1}
	app.Init()
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if y.introStep == introDone {
		t.Fatal("the first tab should open on the start screen")
	}
	app.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if y.introStep != introDone || y.intro != nil {
		t.Error("leaving the tab should end its start screen")
	}
	if a.introStep != introDone || a.intro != nil {
		t.Error("a tab opened later should have no start screen")
	}
}

// While the visualizer opens (or closes) over the interface, the interface
// under it stays where it was, tabs and all: without them it jumped up a
// row before the picture covered it.
func TestApp_VisualizerOpeningKeepsTheTabs(t *testing.T) {
	app, services := testApp(t)
	before := app.View().Content
	app.Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
	m := services[0].Model
	m.viz.over = true // opening as a picture over the interface
	content := app.View().Content
	if !strings.Contains(content, "Apple Music") {
		t.Error("the tabs should stay while the visualizer opens over them")
	}
	if strings.Count(content, "\n") != strings.Count(before, "\n") {
		t.Errorf("the screen under the opening visualizer is %d rows, was %d", strings.Count(content, "\n")+1, strings.Count(before, "\n")+1)
	}
	first := func(s string) string { return strings.SplitN(s, "\n", 2)[0] }
	if first(content) != first(before) {
		t.Error("the first row moved as the visualizer began to open")
	}
}
