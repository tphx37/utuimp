package tui

import (
	"image"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/player"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/views"
)

// ← → and + - go as far as the settings say, and the hints say so too.
func TestSeekAndVolumeSteps(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	m.cfg.SeekStep, m.cfg.VolumeStep = 15, 2
	m.playerState = player.State{Track: &provider.Track{ID: "1", Title: "Song", Duration: 3 * time.Minute},
		Position: time.Minute, Volume: 0.5, Playing: true}
	cmd, _ := m.playbackKey("right")
	runCmd(cmd)
	if mp.seekPos != 75*time.Second {
		t.Errorf("→ went to %v, want 1:15", mp.seekPos)
	}
	cmd, _ = m.playbackKey("left")
	runCmd(cmd)
	if mp.seekPos != 60*time.Second {
		t.Errorf("← went to %v, want 1:00 (back from 1:15)", mp.seekPos)
	}
	cmd, _ = m.playbackKey("+")
	runCmd(cmd)
	if mp.volume == nil || *mp.volume < 0.519 || *mp.volume > 0.521 {
		t.Errorf("+ set the volume to %v, want 52%%", mp.volume)
	}
	hints := ansi.Strip(strings.Join(m.statusLinesFull(200), " "))
	if !strings.Contains(hints, "±15s") {
		t.Errorf("the hints should show the seek step: %q", hints)
	}
	m.cfg.SeekStep, m.cfg.VolumeStep = 0, 0
	if m.seekStep() != 10*time.Second || m.volumeStep() != 0.05 {
		t.Error("nothing set: 10 s and 5%")
	}
}

// What a tab keeps as its own follows the config: changed in the
// settings, or in another tab.
func TestSettingsFollowed(t *testing.T) {
	m := newModel(newMockPlayer())
	m.cfg.HideHints = true
	m.Update(tickMsg{})
	if !m.hintsHidden {
		t.Error("the key hints should follow the setting")
	}
	m.cfg.HideHints = false
	m.Update(tickMsg{})
	if m.hintsHidden {
		t.Error("and come back")
	}
	m.cfg.AlbumArt = true
	m.supportsArtColor = func() bool { return true }
	m.artwork.url = "http://cover"
	cmd := m.followSettings()
	if !m.artMode || cmd == nil {
		t.Errorf("the album art should follow the setting and its cover be fetched (art %v, cmd %v)", m.artMode, cmd != nil)
	}
	m.cfg.AlbumArt = false
	m.followSettings()
	if m.artMode {
		t.Error("the album art should go when the setting is off")
	}
}

// The frame rate: fixed at 60 or 30, or fitting the screen; UTUIMP_FPS goes
// before the setting.
func TestFrameRateSetting(t *testing.T) {
	was, wasSetting, wasHz, wasEnv := screenFrame, paceSetting, displayHz, paceEnv
	t.Cleanup(func() { screenFrame, paceSetting, displayHz, paceEnv = was, wasSetting, wasHz, wasEnv })
	displayHz, paceEnv = 144, 0
	for setting, want := range map[string]time.Duration{"30": time.Second / 30, "60": time.Second / 60, "": time.Second / 72} {
		applyFrameRate(setting)
		if d := screenFrame - want; d < -time.Microsecond || d > time.Microsecond {
			t.Errorf("%q: a frame is %v, want %v", setting, screenFrame, want)
		}
	}
	paceEnv = 90
	applyFrameRate("30")
	if d := screenFrame - time.Second/90; d < -time.Microsecond || d > time.Microsecond {
		t.Errorf("UTUIMP_FPS=90 should win over the setting: %v", screenFrame)
	}
	// Through the settings panel.
	paceEnv = 0
	m := newModel(newMockPlayer())
	m.cfg.FrameRate = "30"
	m.settingsChanged(views.SettingsChangedMsg{Key: "interface.frame_rate"})
	if screenFrame != time.Second/30 {
		t.Errorf("changed in the settings, a frame is %v", screenFrame)
	}
}

// The lyrics' size follows the setting.
func TestLyricsSizeSetting(t *testing.T) {
	var p lyricsPic
	if !p.ready(18, 1) {
		t.Skip("no font to draw with here")
	}
	normal := p.font.Size
	if !p.ready(18, (&config.Config{LyricsSize: config.LyricsLarge}).LyricsScale()) || p.font.Size <= normal {
		t.Errorf("large: %v, normal %v", p.font.Size, normal)
	}
	if !p.ready(18, (&config.Config{LyricsSize: config.LyricsSmall}).LyricsScale()) || p.font.Size >= normal {
		t.Errorf("small: %v, normal %v", p.font.Size, normal)
	}
}

// A new song is announced as the setting says: with its cover once that is
// in, without it if it doesn't come, only while away if so asked.
func TestSongNotifications(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var sent [][]string
	wasRun := notifyRun
	t.Cleanup(func() { notifyRun = wasRun; windowAway.Store(false) })
	notifyRun = func(name string, args ...string) error {
		sent = append(sent, append([]string{name}, args...))
		return nil
	}
	m := newModel(newMockPlayer())
	m.supportsArtColor = func() bool { return true }
	tr := &provider.Track{ID: "s1", Title: "Night Song", Artist: "Band", Album: "Record", ArtworkURL: "http://cover/1"}

	// Off: nothing.
	m.Update(playerStateMsg{Track: tr, Playing: true})
	if m.notifyPending != "" {
		t.Fatal("notifications are off")
	}

	// Always: it waits for the cover, then goes with it.
	m.cfg.Notify = config.NotifyAlways
	tr2 := *tr
	tr2.ID, tr2.ArtworkURL = "s2", "http://cover/2"
	m.Update(playerStateMsg{Track: &tr2, Playing: true})
	if m.notifyPending == "" {
		t.Fatal("a new song should be waiting for its cover to be announced")
	}
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	img.Set(1, 1, color.RGBA{200, 10, 10, 255})
	_, cmd := m.Update(artworkLoadedMsg{url: tr2.ArtworkURL, gen: m.artworkGen, img: img})
	runCmd(cmd)
	if len(sent) != 1 {
		t.Fatalf("one notification, got %d", len(sent))
	}
	got := sent[0]
	if got[0] != "notify-send" || !slices.Contains(got, "Night Song") || !slices.Contains(got, "Band — Record") ||
		!slices.ContainsFunc(got, func(a string) bool { return strings.HasPrefix(a, "--icon=") && strings.HasSuffix(a, ".png") }) {
		t.Errorf("the notification: %q", got)
	}

	// The cover never comes: it goes without one.
	tr3 := *tr
	tr3.ID, tr3.ArtworkURL = "s3", "http://cover/3"
	m.Update(playerStateMsg{Track: &tr3, Playing: true})
	_, cmd = m.Update(notifyTimeoutMsg{id: m.notifyPending})
	runCmd(cmd)
	if len(sent) != 2 || slices.ContainsFunc(sent[1], func(a string) bool { return strings.HasPrefix(a, "--icon=") }) {
		t.Errorf("without its cover: %q", sent)
	}

	// Away: not while utuimp is the window in use.
	m.cfg.Notify = config.NotifyAway
	m.Update(tea.FocusMsg{})
	tr4 := *tr
	tr4.ID, tr4.ArtworkURL = "s4", ""
	m.Update(playerStateMsg{Track: &tr4, Playing: true})
	if len(sent) != 2 || m.notifyPending != "" {
		t.Error("in utuimp's window: no notification")
	}
	m.Update(tea.BlurMsg{})
	tr5 := *tr
	tr5.ID, tr5.ArtworkURL = "s5", ""
	if cmd := m.notifyTrack(&tr5); cmd != nil {
		runCmd(cmd)
	}
	if len(sent) != 3 {
		t.Error("in another window: a notification")
	}
}

func TestNotifyCommand(t *testing.T) {
	name, args := notifyCommand("darwin", `Say "hi"`, `A \ B`, "")
	if name != "osascript" || len(args) != 2 || args[1] != `display notification "A \\ B" with title "Say \"hi\""` {
		t.Errorf("macOS: %s %q", name, args)
	}
	name, args = notifyCommand("linux", "-rf", "Tom & Jerry <live>", "/x.png")
	if name != "notify-send" || args[len(args)-3] != "--" || args[len(args)-2] != "-rf" || args[len(args)-1] != "Tom &amp; Jerry &lt;live&gt;" ||
		!slices.Contains(args, "--icon=/x.png") {
		t.Errorf("linux: %q", args)
	}
}

// The view and the cover view's lyrics changed in the settings leave the
// settings open: the next key is still theirs.
func TestViewChangedInSettingsKeepsThem(t *testing.T) {
	m := newModel(newMockPlayer())
	m.width, m.height = 120, 36
	m.openPanel(m.settingsP)
	if !m.settingsOpen() {
		t.Fatal("the settings should be open")
	}
	m.cfg.Layout = LayoutCover
	m.Update(tickMsg{})
	if !m.coverView || !m.settingsOpen() {
		t.Errorf("the cover view (%v), the settings still open (%v)", m.coverView, m.settingsOpen())
	}
	m.cfg.CoverHideLyrics = true
	m.Update(tickMsg{})
	if !m.coverPlain || !m.settingsOpen() {
		t.Errorf("lyrics hidden (%v), the settings still open (%v)", m.coverPlain, m.settingsOpen())
	}
}
