package cmd

import (
	"path/filepath"
	"strings"

	"github.com/tphx37/utuimp/internal/i18n"
	"testing"

	"github.com/tphx37/utuimp/internal/config"
)

func TestServiceKeys(t *testing.T) {
	defs := serviceDefs(256)
	if defs[len(defs)-1].key != "local" {
		t.Fatal("local files must be the last tab")
	}
	k := serviceKeys(&config.Config{}, defs)
	if k["1"] != "yandex" || k["2"] != "apple" || k["3"] != "spotify" ||
		k["4"] != "youtube" || k["5"] != "soundcloud" || k["6"] != "local" {
		t.Errorf("defaults = %v", k)
	}
	// Rebinding a service moves it; its default key is freed.
	k = serviceKeys(&config.Config{ServiceKeys: map[string]string{"9": "yandex", "ё": "apple"}}, defs)
	if k["9"] != "yandex" || k["1"] != "" {
		t.Errorf("rebinding yandex: %v", k)
	}
	if k["ё"] != "apple" || k["`"] != "apple" {
		t.Errorf("ё and ` are one key: %v", k)
	}
}

func TestSignOutPerService(t *testing.T) {
	cfg := &config.Config{AppleUserToken: "a", YandexToken: "y", YandexRefreshToken: "r"}
	if _, err := signOut(cfg, []string{"yandex"}); err != nil {
		t.Fatal(err)
	}
	if cfg.YandexToken != "" || cfg.YandexRefreshToken != "" || cfg.AppleUserToken != "a" {
		t.Errorf("yandex only: %+v", cfg)
	}
	if _, err := signOut(cfg, []string{"all"}); err != nil || cfg.AppleUserToken != "" {
		t.Errorf("all: %+v %v", cfg, err)
	}
	if _, err := signOut(cfg, []string{"deezer"}); err == nil {
		t.Error("unknown service should be an error")
	}
}

func TestSetupSavesAnswers(t *testing.T) {
	dir := t.TempDir()
	cfgFile = filepath.Join(dir, "config.json")
	t.Cleanup(func() { cfgFile = ""; i18n.Set("en") })
	cfg := &config.Config{}
	// Russian, no Discord, services: yandex y, apple n, spotify, youtube and
	// soundcloud (default n), local y + folder.
	in := strings.NewReader("2\nн\nд\nн\n\n\n\nд\n/music\n")
	var out strings.Builder
	if err := runSetup(cfg, in, &out); err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "ru" || cfg.DiscordRPCEnabled() || !cfg.SetupDone || cfg.MusicDir != "/music" {
		t.Errorf("cfg = %+v", cfg)
	}
	if strings.Join(cfg.Services, ",") != "yandex,local" {
		t.Errorf("services = %v", cfg.Services)
	}
	if !strings.Contains(out.String(), "Сервисы") {
		t.Errorf("questions after choosing Russian should be in Russian:\n%s", out.String())
	}
	defs := enabledServices(serviceDefs(256), cfg.Services)
	if keys := serviceKeys(cfg, defs); keys["1"] != "yandex" || keys["2"] != "local" {
		t.Errorf("keys follow the enabled tabs: %v", keys)
	}
}
