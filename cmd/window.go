package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tphx37/utuimp/internal/assets"
)

// terminalLauncher knows how to open a terminal emulator window running
// argv, with the window's app id / class set to utuimp's so the compositor,
// task bar and dock show the utuimp icon and group it as its own app.
type terminalLauncher struct {
	bin  string
	args func(argv []string) []string
}

var terminalLaunchers = []terminalLauncher{
	{"kitty", func(a []string) []string {
		return append([]string{"--class", assets.AppID, "--title", "utuimp", "-e"}, a...)
	}},
	{"foot", func(a []string) []string {
		return append([]string{"--app-id=" + assets.AppID, "--title=utuimp"}, a...)
	}},
	{"alacritty", func(a []string) []string {
		return append([]string{"--class", assets.AppID + "," + assets.AppID, "--title", "utuimp", "-e"}, a...)
	}},
	{"wezterm", func(a []string) []string {
		return append([]string{"start", "--class", assets.AppID, "--"}, a...)
	}},
	{"ghostty", func(a []string) []string {
		return append([]string{"--class=" + assets.AppID, "--title=utuimp", "-e"}, a...)
	}},
	{"konsole", func(a []string) []string {
		return append([]string{"-p", "tabtitle=utuimp", "-e"}, a...)
	}},
	{"xdg-terminal-exec", func(a []string) []string { return a }},
}

// pickTerminal prefers $UTUIMP_TERMINAL, then the terminal we're running in,
// then the first known one installed.
func pickTerminal() (terminalLauncher, string, error) {
	find := func(name string) (terminalLauncher, string, bool) {
		for _, l := range terminalLaunchers {
			if l.bin == name {
				if p, err := exec.LookPath(l.bin); err == nil {
					return l, p, true
				}
			}
		}
		return terminalLauncher{}, "", false
	}
	var prefer []string
	if t := os.Getenv("UTUIMP_TERMINAL"); t != "" {
		prefer = append(prefer, filepath.Base(t))
	}
	switch {
	case os.Getenv("KITTY_PID") != "" || os.Getenv("TERM") == "xterm-kitty":
		prefer = append(prefer, "kitty")
	case os.Getenv("WEZTERM_PANE") != "":
		prefer = append(prefer, "wezterm")
	case os.Getenv("GHOSTTY_RESOURCES_DIR") != "":
		prefer = append(prefer, "ghostty")
	case os.Getenv("ALACRITTY_WINDOW_ID") != "":
		prefer = append(prefer, "alacritty")
	case strings.HasPrefix(os.Getenv("TERM"), "foot"):
		prefer = append(prefer, "foot")
	case os.Getenv("KONSOLE_VERSION") != "":
		prefer = append(prefer, "konsole")
	}
	for _, l := range terminalLaunchers {
		prefer = append(prefer, l.bin)
	}
	for _, name := range prefer {
		if l, p, ok := find(name); ok {
			return l, p, nil
		}
	}
	return terminalLauncher{}, "", errors.New("no supported terminal emulator found (set UTUIMP_TERMINAL)")
}

// openInOwnWindow starts utuimp (with the remaining arguments) in a new
// terminal window and returns at once.
func openInOwnWindow(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args = slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == "--window" || a == "--window=true" })
	l, termPath, err := pickTerminal()
	if err != nil {
		return err
	}
	cmd := exec.Command(termPath, l.args(append([]string{exe}, args...))...) //nolint:gosec // our own executable
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
