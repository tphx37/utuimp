package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/tui/styles"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/lastfm"
	localPlayer "github.com/tphx37/utuimp/internal/player/local"
	"github.com/tphx37/utuimp/internal/player/mux"
	localProvider "github.com/tphx37/utuimp/internal/provider/local"
	"github.com/tphx37/utuimp/internal/provider/localstore"
	"github.com/tphx37/utuimp/internal/tui"
)

// engineEnv is what the per-service engines share: the player mux for
// system integrations, cleanup on exit, and the restart-after-update hook.
type engineEnv struct {
	mux *mux.Mux

	mu         sync.Mutex
	cleanups   []func()
	restartExe string
}

func (e *engineEnv) addCleanup(fn func()) {
	e.mu.Lock()
	e.cleanups = append(e.cleanups, fn)
	e.mu.Unlock()
}

func (e *engineEnv) restart(exe string) {
	e.mu.Lock()
	e.restartExe = exe
	e.mu.Unlock()
}

func (e *engineEnv) close() {
	e.mu.Lock()
	fns := e.cleanups
	e.cleanups = nil
	e.mu.Unlock()
	for _, fn := range slices.Backward(fns) {
		fn()
	}
}

// serviceDef describes a service tab.
type serviceDef struct {
	key, name   string
	placeholder string
	history     string
	start       func(cfg *config.Config, send func(tea.Msg), env *engineEnv, idx int)
}

// serviceDefs lists the tabs in order. Local files stay last: add new
// services before it. Default keys are the tab numbers (1, 2, …).
func serviceDefs(audioBitrateKbps int) []serviceDef {
	return []serviceDef{
		{key: "yandex", name: i18n.T("Yandex Music"), history: "history-yandex", start: startYandexEngine},
		{key: "apple", name: i18n.T("Apple Music"), history: "history", start: func(cfg *config.Config, send func(tea.Msg), env *engineEnv, idx int) {
			startAppleEngine(cfg, send, env, idx, audioBitrateKbps)
		}},
		{key: "spotify", name: i18n.T("Spotify"), history: "history-spotify", start: startSpotifyEngine},
		{key: "youtube", name: i18n.T("YouTube Music"), history: "history-youtube", start: startYouTubeEngine},
		{key: "soundcloud", name: i18n.T("SoundCloud"), history: "history-soundcloud", start: startSoundCloudEngine},
		{key: "local", name: i18n.T("Local files"), history: "history-local", start: startLocalEngine},
	}
}

// enabledServices keeps the services chosen in setup (all when none were),
// in tab order.
func enabledServices(defs []serviceDef, enabled []string) []serviceDef {
	if len(enabled) == 0 {
		return defs
	}
	var out []serviceDef
	for _, d := range defs {
		if slices.Contains(enabled, d.key) {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return defs
	}
	return out
}

// serviceKeys resolves the key → service map: defaults, overridden by the
// config's service_keys (a service bound there loses its default key).
func serviceKeys(cfg *config.Config, defs []serviceDef) map[string]string {
	defaultServiceKeys := map[string]string{}
	for i, d := range defs {
		defaultServiceKeys[strconv.Itoa(i+1)] = d.key
	}
	keys := map[string]string{}
	rebound := map[string]bool{}
	for _, svc := range cfg.ServiceKeys {
		rebound[svc] = true
	}
	for k, svc := range defaultServiceKeys {
		if !rebound[svc] {
			keys[k] = svc
		}
	}
	twin := map[string]string{"`": "ё", "ё": "`"}
	for k, svc := range cfg.ServiceKeys {
		keys[k] = svc
		// A binding for ` or ё claims the physical key in both layouts,
		// unless the config binds the twin separately.
		if t, ok := twin[k]; ok {
			if _, own := cfg.ServiceKeys[t]; !own {
				keys[t] = svc
			}
		}
	}
	// ` and ё are the same physical key on en/ru layouts.
	for k, svc := range keys {
		switch k {
		case "`":
			if _, ok := keys["ё"]; !ok {
				keys["ё"] = svc
			}
		case "ё":
			if _, ok := keys["`"]; !ok {
				keys["`"] = svc
			}
		}
	}
	return keys
}

// runServices runs the tabbed app. Every service has its tab from the
// start, so each keeps its place (and its player's slot) for good; the ones
// turned off are hidden, and the settings show and hide them while utuimp
// runs.
func runServices(cfg *config.Config, opts tui.Options, audioBitrateKbps int, initial string) error {
	defs := serviceDefs(audioBitrateKbps)
	shown := func() []serviceDef { return enabledServices(defs, cfg.Services) }
	isShown := func(key string) bool {
		return slices.ContainsFunc(shown(), func(d serviceDef) bool { return d.key == key })
	}

	if initial == "" {
		initial = cfg.Service
	}
	if !isShown(initial) {
		initial = shown()[0].key
	}
	active := slices.IndexFunc(defs, func(d serviceDef) bool { return d.key == initial })

	env := &engineEnv{mux: mux.New(active)}
	defer env.close()

	opts.Restart = func() {
		if exe, err := os.Executable(); err == nil {
			env.restart(exe)
		}
	}
	var onSetting func(string)
	opts.OnSetting = func(key string) {
		if onSetting != nil {
			onSetting(key)
		}
	}

	services := make([]*tui.Service, len(defs))
	for i, d := range defs {
		s := &tui.Service{Name: d.name, Key: d.key, Placeholder: d.placeholder}
		if d.start != nil {
			o := opts
			o.HistoryName = d.history
			if d.key != initial {
				o.OpenSettings, o.Welcome = "", false
			}
			if d.key == "local" {
				o.EmptyQueueHint = i18n.T("No music found. Put audio files (MP3, FLAC, M4A…) in") + "\n" +
					localMusicDir(cfg) + "\n" +
					i18n.T("or set \"music_dir\" in the config / run utuimp --music-dir <path>.")
			}
			s.Model = tui.New(cfg, nil, nil, o)
			start := d.start
			idx := i
			s.Start = func(send func(tea.Msg)) { start(cfg, send, env, idx) }
		}
		services[i] = s
	}

	app := tui.NewApp(services, active)
	// retab hides the tabs that are off and hands out the keys again.
	retab := func() {
		keys := serviceKeys(cfg, shown())
		app.Keys = map[string]int{}
		for i, d := range defs {
			services[i].Hidden = !isShown(d.key)
			var hot []string
			for k, svc := range keys {
				if svc != d.key {
					continue
				}
				app.Keys[k] = i
				if k != "ё" {
					hot = append(hot, k)
				}
			}
			sort.Strings(hot)
			services[i].Hotkey = strings.Join(hot, "/")
		}
	}
	retab()
	app.OnSwitch = func(idx int) {
		env.mux.Focus(idx)
		cfg.Service = defs[idx].key
		_ = cfg.Save(cfgFile)
	}

	prog := tea.NewProgram(app, programOptions()...)
	app.SetSend(prog.Send)

	debugLog := func(msg string) { prog.Send(tui.DebugLogMsg(msg)) }
	lastfm.ApplyEmbedded(cfg)
	env.addCleanup(setupMuxPlatform(env.mux))
	startLastfmScrobbler(cfg, env.mux, debugLog)
	// Called from the program's own goroutine, which must not wait on
	// prog.Send: whatever is sent from here goes through a goroutine.
	asyncLog := func(msg string) { go debugLog(msg) }
	stopDiscord := startDiscordRPC(cfg, env.mux, debugLog)
	env.addCleanup(func() { stopDiscord() })

	onSetting = func(key string) {
		switch {
		case strings.HasPrefix(key, "services."):
			was := make([]bool, len(services))
			for i, s := range services {
				was[i] = s.Hidden
			}
			retab()
			for i, s := range services {
				if s.Hidden && !was[i] && s.Started() {
					go app.Sender(i)(tui.ServiceOffMsg{})
				}
			}
		case key == "integrations.discord":
			stopDiscord()
			stopDiscord = startDiscordRPC(cfg, env.mux, asyncLog)
		case key == "integrations.lastfm":
			startLastfmScrobbler(cfg, env.mux, asyncLog)
		case key == "interface.theme":
			if styles.DetectConsole() {
				return // the console keeps its own palette
			}
			dir := ""
			if p, err := config.ConfigPath(cfgFile); err == nil {
				dir = filepath.Dir(p)
			}
			theme, _ := styles.LoadTheme(cfg.Theme, dir)
			styles.Apply(theme)
		}
	}

	restoreStderr := captureStderr()
	_, err := prog.Run()
	restoreStderr()
	env.close()

	if env.restartExe != "" {
		if restartErr := restartProcess(env.restartExe); restartErr != nil {
			return errors.Join(err, fmt.Errorf("restarting into the update at %s: %w", env.restartExe, restartErr))
		}
	}
	return err
}

// localMusicDir is music_dir from the config, else ~/Music.
func localMusicDir(cfg *config.Config) string {
	if cfg.MusicDir != "" {
		return cfg.MusicDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "Music"
	}
	return filepath.Join(home, "Music")
}

// startLocalEngine scans the music directory (config music_dir, default
// ~/Music) and plays files from it.
func startLocalEngine(cfg *config.Config, send func(tea.Msg), env *engineEnv, idx int) {
	dir := localMusicDir(cfg)
	send(tui.InitStatusMsg(i18n.T("Scanning ") + dir + "…"))
	prov, err := localProvider.New(dir)
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("local files: %w", err)})
		return
	}
	if st, err := localstore.Open(localstore.DefaultPath("library-local")); err == nil {
		prov.SetStore(st) // playlists made in utuimp
	}
	plyr, err := localPlayer.New(func(msg string) { send(tui.DebugLogMsg(msg)) })
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("audio engine: %w", err)})
		return
	}
	env.addCleanup(func() { _ = plyr.Close() })
	tracks, err := prov.GetLibraryTracks(context.Background())
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("scanning %s: %w", dir, err)})
		return
	}
	plyr.LoadTracks(tracks)
	env.mux.Set(idx, plyr)
	notice := prov.ScanNotice()
	if len(tracks) == 0 && notice == "" {
		notice = "No audio files in " + dir + " (set music_dir in the config)"
	}
	send(tui.EngineReadyMsg{
		Player: plyr, Provider: prov,
		Backend: "Local files · " + dir,
		Tracks:  tracks, Notice: notice,
	})
}
