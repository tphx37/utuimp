package settings

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/i18n"
)

// Every title, help, label and reason the panel can show has its Russian.
func TestSettingsTranslated(t *testing.T) {
	i18n.Set("ru")
	t.Cleanup(func() { i18n.Set("en") })
	c := &config.Config{}
	check := func(what, s string) {
		if strings.HasSuffix(s, "%") || strings.Contains(s, "Client ID") || strings.Contains(s, "Client Secret") {
			return // numbers, and the services' own terms
		}
		if s != "" && i18n.T(s) == s {
			t.Errorf("%s has no translation: %q", what, s)
		}
	}
	keys := map[string]bool{}
	for _, sec := range Sections() {
		check("section", sec.Title)
		for _, it := range sec.Items {
			if keys[it.Key] {
				t.Errorf("two items are %q", it.Key)
			}
			keys[it.Key] = true
			if it.Key == "services.music_dir" || sec.Key == "services" && it.Kind == Toggle {
				continue // the services' own names
			}
			check(it.Key+" title", it.Title)
			check(it.Key+" help", it.Help)
			if it.Options != nil && it.Key != "interface.language" && it.Key != "interface.theme" {
				for _, o := range it.Options(c) {
					label := o.Label
					if label == "" {
						label = o.Value // shown as it is
					}
					if _, err := strconv.Atoi(label); err == nil || label == "100%" || len(label) <= 2 {
						continue // a number
					}
					check(it.Key+" option", label)
				}
			}
		}
	}
	for _, why := range []string{animationOff(&config.Config{UIMotion: config.MotionOff}), "the pixel interface is off", "the cover is in characters"} {
		check("reason", why)
	}
}

// The parts of the pixel interface can't be changed where they would not
// show: with the pixel interface or the animation off, nor the light with
// the cover in characters.
func TestSettingsGraphicsBlocked(t *testing.T) {
	item := func(key string) Item {
		for _, sec := range Sections() {
			for _, it := range sec.Items {
				if it.Key == key {
					return it
				}
			}
		}
		t.Fatalf("no item %q", key)
		return Item{}
	}
	cover, light, style := item("graphics.cover"), item("graphics.light"), item("graphics.style")
	if why := cover.Blocked(&config.Config{}); why != "" {
		t.Errorf("the defaults: the cover can be changed, not %q", why)
	}
	if cover.Blocked(&config.Config{UIStyle: config.StyleSimple}) == "" {
		t.Error("the pixel interface off: the cover can't be changed")
	}
	if cover.Blocked(&config.Config{UIMotion: config.MotionOff}) == "" || style.Blocked(&config.Config{UIMotion: config.MotionOff}) == "" {
		t.Error("the animation off: no pixel interface to set")
	}
	if light.Blocked(&config.Config{UICover: config.PartCells}) == "" {
		t.Error("the cover in characters: no light behind it")
	}
	c := &config.Config{UILines: "something else"}
	if got := item("graphics.lines").Get(c); got != "" {
		t.Errorf("a value set by hand that isn't one of the options shows as the default, not %q", got)
	}
}

func findItem(t *testing.T, key string) Item {
	t.Helper()
	for _, sec := range Sections() {
		for _, it := range sec.Items {
			if it.Key == key {
				return it
			}
		}
	}
	t.Fatalf("no item %q", key)
	return Item{}
}

// The settings added for playback, the interface, the lyrics and the
// visualizer read and write their config fields, show the defaults for
// what is not set, and reading one changes nothing.
func TestSettingsConvenience(t *testing.T) {
	c := &config.Config{}
	want := map[string]string{
		"interface.view": "", "interface.cover_lyrics": On, "interface.album_art": Off, "interface.hints": On, "interface.amoled": Off,
		"interface.frame_rate": "", "playback.seek_step": "10", "playback.volume_step": "5",
		"playback.notify": "", "lyrics.size": "", "visualizer.scene": "", "visualizer.brightness": "100",
	}
	for key, def := range want {
		if got := findItem(t, key).Get(c); got != def {
			t.Errorf("%s: default %q, want %q", key, got, def)
		}
	}
	if !reflect.DeepEqual(*c, config.Config{}) {
		t.Errorf("reading the defaults changed the config: %+v", *c)
	}

	set := func(key, v string) { findItem(t, key).Set(c, v) }
	set("interface.view", "cover")
	set("interface.amoled", On)
	set("interface.cover_lyrics", Off)
	set("interface.album_art", On)
	set("interface.hints", Off)
	set("interface.frame_rate", "30")
	set("playback.seek_step", "15")
	set("playback.volume_step", "2")
	set("playback.notify", config.NotifyAway)
	set("lyrics.size", config.LyricsLarge)
	set("visualizer.scene", "nebula")
	set("visualizer.brightness", "70")
	if c.Layout != "cover" || !c.AMOLED || !c.CoverHideLyrics || !c.AlbumArt || !c.HideHints || c.FrameRate != "30" ||
		c.SeekStep != 15 || c.VolumeStep != 2 || c.Notify != config.NotifyAway || c.LyricsSize != config.LyricsLarge ||
		c.VisualizerScene != "nebula" || c.VisualizerBrightness == nil || *c.VisualizerBrightness != 70 {
		t.Errorf("not written: %+v", *c)
	}
	if c.SeekStepDur().Seconds() != 15 || c.VolumeStepFrac() != 0.02 || c.LyricsScale() <= 1 || c.VizBrightness() != 0.7 {
		t.Error("the config does not read them back")
	}
	// Back to the defaults: nothing left in the config for them.
	set("playback.seek_step", "10")
	set("playback.volume_step", "5")
	set("visualizer.brightness", "100")
	if c.SeekStep != 0 || c.VolumeStep != 0 || c.VisualizerBrightness != nil {
		t.Errorf("a default is not kept as nothing: %d %d %v", c.SeekStep, c.VolumeStep, c.VisualizerBrightness)
	}
	// A brightness set with { } shows as it is, among the choices.
	n := 80
	c.VisualizerBrightness = &n
	it := findItem(t, "visualizer.brightness")
	found := false
	for _, o := range it.Options(c) {
		found = found || o.Value == "80"
	}
	if it.Get(c) != "80" || !found {
		t.Error("a brightness of 80% should be shown and be a choice")
	}
	// The visualizer's own name for the first scene is the default.
	c.VisualizerScene = "tunnel"
	if findItem(t, "visualizer.scene").Get(c) != "" {
		t.Error("tunnel is the first choice")
	}
}

// Song notifications are offered only where the desktop can show them.
func TestSettingsNotifyHidden(t *testing.T) {
	was := NotifierAvailable
	t.Cleanup(func() { NotifierAvailable = was })
	it := findItem(t, "playback.notify")
	NotifierAvailable = func() bool { return false }
	if !it.Hidden(&config.Config{}) {
		t.Error("no notify-send nor osascript: the item should be hidden")
	}
	NotifierAvailable = func() bool { return true }
	if it.Hidden(&config.Config{}) {
		t.Error("with a notifier the item should show")
	}
}
