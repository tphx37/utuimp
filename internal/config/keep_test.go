package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A key this build doesn't know (written by a newer one) survives a save.
func TestSaveKeepsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"theme":"nord","future_setting":{"a":[1,2]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Theme = "gruvbox"
	if err := cfg.Save(""); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path) //nolint:gosec // the test's own config file
	if !strings.Contains(string(b), `"future_setting"`) || !strings.Contains(string(b), `"gruvbox"`) {
		t.Errorf("saved config lost a key:\n%s", b)
	}
	again, err := Load(path)
	if err != nil || again.Theme != "gruvbox" {
		t.Fatalf("reload: %+v %v", again, err)
	}
}

// An older build that rewrites config.json without the SoundCloud keys
// doesn't lose them: they come back from credentials.json.
func TestCredentialsSurviveAnOldBuild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := &Config{SoundCloudClientID: "id", SoundCloudClientSecret: "secret", Theme: "nord"}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dir, "credentials.json")); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("credentials.json: %v %v", st, err)
	}
	// The old build's save: no SoundCloud keys.
	if err := os.WriteFile(path, []byte(`{"theme":"nord"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.SoundCloudClientID != "id" || got.SoundCloudClientSecret != "secret" {
		t.Errorf("credentials not restored: %+v", got)
	}

	// Clearing them here (setup's "-") clears the mirror too.
	got.SoundCloudClientID, got.SoundCloudClientSecret = "", ""
	if err := got.Save(""); err != nil {
		t.Fatal(err)
	}
	if again, _ := Load(path); again.SoundCloudClientID != "" {
		t.Errorf("a cleared ID came back: %q", again.SoundCloudClientID)
	}
}

// A config made in a test has no path; saving it must not reach the real
// ~/.config/utuimp/config.json.
func TestTestsCannotSaveTheRealConfig(t *testing.T) {
	if err := (&Config{}).Save(""); err != errTestDefaultPath {
		t.Errorf("err = %v, want errTestDefaultPath", err)
	}
}
