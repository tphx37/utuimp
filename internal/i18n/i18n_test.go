package i18n

import "testing"

func TestT(t *testing.T) {
	Set("ru")
	defer Set("en")
	if got := T(" play/pause"); got != " пауза" {
		t.Errorf("got %q", got)
	}
	if got := T("not in the table"); got != "not in the table" {
		t.Errorf("untranslated text must pass through, got %q", got)
	}
	Set("en")
	if got := T("play/pause"); got != "play/pause" {
		t.Errorf("english = %q", got)
	}
}
