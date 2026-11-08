package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/tphx37/utuimp/internal/i18n"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/lastfm"
	localPlayer "github.com/tphx37/utuimp/internal/player/local"
	"github.com/tphx37/utuimp/internal/player/mux"
	localProvider "github.com/tphx37/utuimp/internal/provider/local"
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
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
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

// runServices runs the tabbed app with every service.
func runServices(cfg *config.Config, opts tui.Options, audioBitrateKbps int, initial string) error {
	defs := enabledServices(serviceDefs(audioBitrateKbps), cfg.Services)
	keys := serviceKeys(cfg, defs)

	if initial == "" {
		initial = cfg.Service
	}
	active := 0
	for i, d := range defs {
		if d.key == initial {
			active = i
		}
	}

	env := &engineEnv{mux: mux.New(active)}
	defer env.close()

	services := make([]*tui.Service, len(defs))
	for i, d := range defs {
		s := &tui.Service{Name: d.name, Key: d.key, Placeholder: d.placeholder}
		var hot []string
		for k, svc := range keys {
			if svc == d.key && k != "ё" {
				hot = append(hot, k)
			}
		}
		sort.Strings(hot)
		s.Hotkey = strings.Join(hot, "/")
		if d.start != nil {
			o := opts
			o.HistoryName = d.history
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
	app.Keys = map[string]int{}
	for k, svc := range keys {
		for i, d := range defs {
			if d.key == svc {
				app.Keys[k] = i
			}
		}
	}
	app.OnSwitch = func(idx int) {
		env.mux.Focus(idx)
		cfg.Service = defs[idx].key
		_ = cfg.Save(cfgFile)
	}

	prog := tea.NewProgram(app)
	app.SetSend(prog.Send)

	debugLog := func(msg string) { prog.Send(tui.DebugLogMsg(msg)) }
	lastfm.ApplyEmbedded(cfg)
	env.addCleanup(setupMuxPlatform(env.mux))
	startLastfmScrobbler(cfg, env.mux, debugLog)
	env.addCleanup(startDiscordRPC(cfg, env.mux, debugLog))

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
