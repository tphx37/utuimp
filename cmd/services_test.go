package cmd

import (
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
		k["4"] != "youtube" || k["5"] != "soundcloud" || k["6"] != "radio" || k["7"] != "local" {
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

func TestKeysFollowEnabledTabs(t *testing.T) {
	cfg := &config.Config{Services: []string{"yandex", "local"}}
	defs := enabledServices(serviceDefs(256), cfg.Services)
	if keys := serviceKeys(cfg, defs); keys["1"] != "yandex" || keys["2"] != "local" {
		t.Errorf("keys follow the enabled tabs: %v", keys)
	}
}

func TestSetupSections(t *testing.T) {
	for _, name := range []string{"language", "discord", "services", "visualizer", "interface", "library"} {
		if setupSections[name] == "" {
			t.Errorf("the old setup section %q should still open something", name)
		}
	}
}
