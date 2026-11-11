package signin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec // a local test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestRender(t *testing.T) {
	w := httptest.NewRecorder()
	Render(w, Page{Service: "Spotify", Colors: Spotify, Text: "Sign in.", Button: "Continue", ButtonURL: "https://accounts.spotify.com/authorize?a=1&b=2"})
	body := w.Body.String()
	for _, want := range []string{"utuimp · Spotify", "[[30,215,96],[255,255,255]]", `href="https://accounts.spotify.com/authorize?a=1&amp;b=2"`, "Continue"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	w = httptest.NewRecorder()
	Render(w, Page{Service: "Spotify", Colors: Spotify, Error: "<b>refused</b>"})
	if body := w.Body.String(); !strings.Contains(body, "&lt;b&gt;refused") || !strings.Contains(body, `class="box bad show"`) {
		t.Error("an error should be shown, escaped")
	}
}

func TestServeWaiting(t *testing.T) {
	url, finish, err := ServeWaiting(Page{Service: "Yandex Music", Colors: Yandex, Code: "abc-123", Button: "Open Yandex", ButtonURL: "https://ya.ru/device", NewTab: true})
	if err != nil {
		t.Fatal(err)
	}
	if body := get(t, url); !strings.Contains(body, "abc-123") || !strings.Contains(body, `target="_blank"`) {
		t.Error("the page should show the code and a button that opens a new tab")
	}
	var st struct {
		Done  bool
		Error string
	}
	_ = json.Unmarshal([]byte(get(t, url+"status")), &st)
	if st.Done {
		t.Error("not done yet")
	}
	finish(errors.New("expired"))
	_ = json.Unmarshal([]byte(get(t, url+"status")), &st)
	if !st.Done || st.Error != "expired" {
		t.Errorf("status after finish: %+v", st)
	}
	if body := get(t, url); strings.Contains(body, "abc-123") || !strings.Contains(body, "expired") {
		t.Error("a reloaded page should show how it ended, not the code")
	}
}
