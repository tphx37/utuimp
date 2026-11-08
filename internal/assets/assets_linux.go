//go:build linux

package assets

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// dataHome is $XDG_DATA_HOME, or ~/.local/share when unset, per the XDG base
// directory spec. The fallback icon and desktop entry are written under it.
func dataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share")
}

// dataDirs lists every directory a desktop searches for applications and
// icons: dataHome first, then $XDG_DATA_DIRS (default
// /usr/local/share:/usr/share). A distro package, a Flatpak export, and the
// install script each put their files in one of these.
func dataDirs() []string {
	var dirs []string
	if h := dataHome(); h != "" {
		dirs = append(dirs, h)
	}
	sys := os.Getenv("XDG_DATA_DIRS")
	if sys == "" {
		sys = "/usr/local/share:/usr/share"
	}
	for d := range strings.SplitSeq(sys, ":") {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// installedIcon returns the path of an AppID icon already present in the
// hicolor theme of any data dir, or "" when there is none.
func installedIcon() string {
	for _, d := range dataDirs() {
		matches, _ := filepath.Glob(filepath.Join(d, "icons", "hicolor", "*", "apps", AppID+".*"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

// installedDesktopEntry reports whether an AppID.desktop already exists in the
// applications dir of any data dir.
func installedDesktopEntry() bool {
	for _, d := range dataDirs() {
		if _, err := os.Stat(filepath.Join(d, "applications", AppID+".desktop")); err == nil {
			return true
		}
	}
	return false
}

// removeLegacy deletes a file that versions before the AppID rename wrote
// under ~/.local/share on every launch. Nothing resolves those names now.
func removeLegacy(rel ...string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	_ = os.Remove(filepath.Join(append([]string{home, ".local", "share"}, rel...)...))
}

// InstallIcon makes sure an AppID icon is available to the desktop and returns
// its absolute path (used for notifications), or "" if none could be found or
// written. An icon a package, a Flatpak, or the install script already put in
// a data dir wins. The embedded SVG is the fallback for installs that brought
// no icon of their own, such as `make install` or a plain `go build`. All
// operations are best-effort; errors are silently ignored.
func InstallIcon() string {
	removeLegacy("icons", "hicolor", "scalable", "apps", "utuimp.svg")
	base := dataHome()
	own := ""
	if base != "" {
		own = filepath.Join(base, "icons", "hicolor", "scalable", "apps", AppID+".svg")
	}
	// An icon installed elsewhere (a package, a Flatpak) wins; our own copy
	// under dataHome is refreshed whenever the embedded icon changes.
	if p := installedIcon(); p != "" && p != own {
		return p
	}
	if own == "" {
		return ""
	}
	if cur, err := os.ReadFile(own); err == nil && bytes.Equal(cur, Icon) { //nolint:gosec // our own file
		return own
	}
	if err := os.MkdirAll(filepath.Dir(own), 0o750); err != nil { //nolint:gosec // XDG icon dir
		return ""
	}
	if err := os.WriteFile(own, Icon, 0o644); err != nil { //nolint:gosec // public icon file
		return ""
	}
	hicolor := filepath.Join(base, "icons", "hicolor")
	_ = exec.Command("gtk-update-icon-cache", "--force", "--ignore-theme-index", hicolor).Run() //nolint:gosec
	return own
}

// desktopEntry returns the embedded .desktop file with Exec pointing at the
// utuimp executable by absolute path, so the launcher works even when ~/.local/bin isn't on the
// desktop session's PATH.
func desktopEntry() []byte {
	// Prefer the installed `utuimp` on PATH over whichever copy is running
	// (a dev build in the source tree shouldn't capture the launcher).
	exe, err := exec.LookPath("utuimp")
	if err == nil {
		exe, err = filepath.Abs(exe)
	}
	if err != nil {
		if exe, err = os.Executable(); err != nil {
			return DesktopEntry
		}
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if strings.ContainsAny(exe, " \t\"'\\") {
		exe = `"` + exe + `"`
	}
	return []byte(strings.Replace(string(DesktopEntry), "Exec=utuimp ", "Exec="+exe+" ", 1))
}

// InstallDesktopEntry installs the utuimp launcher (app menu entry that opens
// utuimp in its own terminal window, with its own icon), which also lets MPRIS
// consumers resolve the DesktopEntry property to an icon. An entry from a
// package or Flatpak wins; one utuimp wrote itself (X-utuimp-Generated, or the
// older hidden fallback) is refreshed when its content changes. Best-effort.
func InstallDesktopEntry() {
	removeLegacy("applications", "utuimp.desktop")
	base := dataHome()
	if base == "" {
		return
	}
	dir := filepath.Join(base, "applications")
	own := filepath.Join(dir, AppID+".desktop")
	want := desktopEntry()
	if cur, err := os.ReadFile(own); err == nil { //nolint:gosec // our own file
		generated := strings.Contains(string(cur), "X-utuimp-Generated=true") || strings.Contains(string(cur), "NoDisplay=true")
		if !generated || bytes.Equal(cur, want) {
			return
		}
	} else if installedDesktopEntry() {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // XDG applications dir
		return
	}
	if err := os.WriteFile(own, want, 0o644); err != nil { //nolint:gosec // public .desktop file
		return
	}
	_ = exec.Command("update-desktop-database", dir).Run() //nolint:gosec
}
