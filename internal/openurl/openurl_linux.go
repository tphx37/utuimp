//go:build linux

package openurl

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Open shows url in the browser: as a new tab of the one already running
// where there is one, not a window of another browser started for it (the
// one xdg-open names is not always the one in use); else in $BROWSER, if
// that is a program here; else as xdg-open says. The browser is started
// apart from utuimp, so that closing the terminal does not close it.
func Open(url string) error {
	name, args := command(url, runningBrowsers(), os.Getenv("BROWSER"), exec.LookPath)
	cmd := exec.Command(name, args...) //nolint:gosec // a browser, the address as its argument
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // (no zombie left behind)
	return nil
}

// browser is a browser found running: its program, and what it is called.
type browser struct {
	exe  string
	name string
}

// browserNames are the programs, by the name the system gives their
// process, that open a new tab of the running browser when started with
// an address; newTab are those that need --new-tab for it.
var (
	browserNames = map[string]bool{ //nolint:gochecknoglobals // a constant table
		"chrome": true, "chromium": true, "brave": true, "vivaldi-bin": true, "opera": true,
		"yandex_browser": true, "thorium": true, "helium": true, "msedge": true,
		"firefox": true, "firefox-bin": true, "librewolf": true, "zen": true, "zen-bin": true, "floorp": true, "waterfox": true,
	}
	newTab = map[string]bool{ //nolint:gochecknoglobals // a constant table
		"firefox": true, "firefox-bin": true, "librewolf": true, "zen": true, "zen-bin": true, "floorp": true, "waterfox": true,
	}
)

// command is what opens url: the first running browser, $BROWSER (one
// program or several, with ":" between them, "%s" for the address), else
// xdg-open. lookPath finds a program.
func command(url string, running []browser, envBrowser string, lookPath func(string) (string, error)) (string, []string) {
	if len(running) > 0 {
		if b := running[0]; newTab[b.name] {
			return b.exe, []string{"--new-tab", url}
		}
		return running[0].exe, []string{url}
	}
	for c := range strings.SplitSeq(envBrowser, ":") {
		f := strings.Fields(c)
		if len(f) == 0 {
			continue
		}
		if _, err := lookPath(f[0]); err != nil {
			continue // set once for a browser no longer here
		}
		args, placed := []string{}, false
		for _, a := range f[1:] {
			if strings.Contains(a, "%s") {
				a, placed = strings.ReplaceAll(a, "%s", url), true
			}
			args = append(args, a)
		}
		if !placed {
			args = append(args, url)
		}
		return f[0], args
	}
	return "xdg-open", []string{url}
}

// runningBrowsers lists the browsers running for this user, by their
// processes' names, with the programs they run.
func runningBrowsers() []browser {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	uid := os.Getuid()
	var out []browser
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		dir := filepath.Join("/proc", e.Name())
		comm, err := os.ReadFile(filepath.Join(dir, "comm")) //nolint:gosec // a process's name, under /proc
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		if !browserNames[name] {
			continue
		}
		if st, err := os.Stat(dir); err != nil || !ownedBy(st, uid) {
			continue
		}
		exe, err := os.Readlink(filepath.Join(dir, "exe"))
		if err != nil || seen[exe] || strings.HasSuffix(exe, " (deleted)") {
			continue // (an old one, replaced by an update while it ran)
		}
		if _, err := os.Stat(exe); err != nil {
			continue // in a sandbox of its own (a Flatpak): not a program here
		}
		seen[exe] = true
		out = append(out, browser{exe: exe, name: name})
	}
	return out
}

func ownedBy(st os.FileInfo, uid int) bool {
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && int(s.Uid) == uid
}
