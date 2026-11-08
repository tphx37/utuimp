// Package i18n translates the interface. Strings are written in English in
// the code and looked up in a per-language table; anything without a
// translation is shown as is.
package i18n

import (
	"os"
	"strings"
	"sync"
)

var (
	mu   sync.RWMutex
	lang = "en"
)

// Languages lists the supported interface languages (code, native name).
var Languages = [][2]string{{"en", "English"}, {"ru", "Русский"}}

// Set selects the interface language ("en", "ru"); unknown codes mean English.
func Set(code string) {
	mu.Lock()
	defer mu.Unlock()
	switch strings.ToLower(code) {
	case "ru":
		lang = "ru"
	default:
		lang = "en"
	}
}

// Lang returns the current language code.
func Lang() string {
	mu.RLock()
	defer mu.RUnlock()
	return lang
}

// Detect guesses the language from the locale (LC_ALL, LC_MESSAGES, LANG).
func Detect() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.ToLower(os.Getenv(env)); v != "" && v != "c" && v != "posix" {
			if strings.HasPrefix(v, "ru") {
				return "ru"
			}
			return "en"
		}
	}
	return "en"
}

// T translates s, keeping its leading and trailing spaces.
func T(s string) string {
	mu.RLock()
	l := lang
	mu.RUnlock()
	if l == "en" {
		return s
	}
	core := strings.TrimSpace(s)
	tr, ok := ru[core]
	if !ok || core == "" {
		return s
	}
	start := strings.Index(s, core)
	return s[:start] + tr + s[start+len(core):]
}
