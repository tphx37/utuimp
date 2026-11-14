//go:build linux

package openurl

import (
	"errors"
	"slices"
	"testing"
)

func TestCommand(t *testing.T) {
	const url = "https://example.com/login"
	have := func(names ...string) func(string) (string, error) {
		return func(p string) (string, error) {
			if slices.Contains(names, p) {
				return "/usr/bin/" + p, nil
			}
			return "", errors.New("not found")
		}
	}
	cases := []struct {
		name     string
		running  []browser
		env      string
		look     func(string) (string, error)
		wantName string
		wantArgs []string
	}{
		{"a running Chrome gets a tab", []browser{{"/opt/google/chrome/chrome", "chrome"}}, "firefox", have("firefox"),
			"/opt/google/chrome/chrome", []string{url}},
		{"a running Firefox gets a tab", []browser{{"/usr/lib/firefox/firefox", "firefox"}}, "", have(),
			"/usr/lib/firefox/firefox", []string{"--new-tab", url}},
		{"$BROWSER that is not here is passed over", nil, "firefox", have(), "xdg-open", []string{url}},
		{"$BROWSER that is here", nil, "nothere:brave --incognito", have("brave"), "brave", []string{"--incognito", url}},
		{"$BROWSER with %s", nil, "w3m %s -o x", have("w3m"), "w3m", []string{url, "-o", "x"}},
		{"nothing: xdg-open", nil, "", have(), "xdg-open", []string{url}},
	}
	for _, c := range cases {
		name, args := command(url, c.running, c.env, c.look)
		if name != c.wantName || !slices.Equal(args, c.wantArgs) {
			t.Errorf("%s: %s %q, want %s %q", c.name, name, args, c.wantName, c.wantArgs)
		}
	}
}

// The browsers running here are found (none is fine: the list may be
// empty), each program once.
func TestRunningBrowsers(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range runningBrowsers() {
		if seen[b.exe] || !browserNames[b.name] {
			t.Errorf("%+v", b)
		}
		seen[b.exe] = true
	}
}
