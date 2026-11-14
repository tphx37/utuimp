package cmd

import (
	"testing"

	"github.com/tphx37/utuimp/internal/settings"
)

// Every section of the settings can be opened with `utuimp setup <section>`.
func TestSetupSectionsCoverSettings(t *testing.T) {
	opens := map[string]bool{}
	for _, sec := range setupSections {
		opens[sec] = true
	}
	for _, sec := range settings.Sections() {
		if sec.Key == "about" {
			continue // an action, not settings
		}
		if !opens[sec.Key] {
			t.Errorf("utuimp setup can't open the section %q", sec.Key)
		}
		if setupSections[sec.Key] != sec.Key {
			t.Errorf("utuimp setup %s should open the section of that name", sec.Key)
		}
	}
	for name, sec := range setupSections {
		found := false
		for _, s := range settings.Sections() {
			found = found || s.Key == sec
		}
		if !found {
			t.Errorf("utuimp setup %s opens %q, which is no section", name, sec)
		}
	}
}
