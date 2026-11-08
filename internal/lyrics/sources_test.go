package lyrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseYRCWordTimingAndCredits(t *testing.T) {
	raw := `{"t":0,"c":[{"tx":"作词: "},{"tx":"Someone"}]}
[0,1000](0,1000,0) 作词 : Someone
[6820,1590](6820,570,0)Coming (7390,210,0)out (7600,90,0)of (7690,180,0)my (7870,540,0)cage
[9000,500](9000,500,0)Time: (9500,500,0)now`
	lines := parseYRC(raw)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %+v", len(lines), lines)
	}
	l := lines[0]
	if l.Text != "Coming out of my cage" || l.Start != 6820*time.Millisecond {
		t.Errorf("line 0 = %q @ %v", l.Text, l.Start)
	}
	if len(l.Words) != 5 || l.Words[1].Text != "out " || l.Words[1].Start != 7390*time.Millisecond || l.Words[1].Dur != 210*time.Millisecond {
		t.Errorf("words = %+v", l.Words)
	}
	if l.Words[4].Text != "cage" {
		t.Errorf("last word should be right-trimmed, got %q", l.Words[4].Text)
	}
	// A Latin "role: …" line is a lyric, not a credit.
	if lines[1].Text != "Time: now" {
		t.Errorf("line 1 = %q", lines[1].Text)
	}
}

func TestPickRicherPrefersWordSync(t *testing.T) {
	line := []Line{{Text: "x"}}
	plain := &Result{Lines: line, Source: SourceLRCLIB}
	synced := &Result{Lines: line, Synced: true, Source: SourceLRCLIB}
	word := &Result{Lines: line, Synced: true, WordSynced: true, Source: SourceNetEase}
	if got := pickRicher(synced, word); got != word {
		t.Error("word sync should beat line sync")
	}
	if got := pickRicher(synced, &Result{Lines: line, Synced: true}); got != synced {
		t.Error("ties should keep the first (LRCLIB) result")
	}
	if got := pickRicher(nil, plain); got != plain {
		t.Error("anything beats nothing")
	}
	if pickRicher(nil, nil) != nil {
		t.Error("nothing from nothing")
	}
}

func TestFetchNetEaseMatchesArtistAndPrefersYRC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search/get":
			_, _ = w.Write([]byte(`{"result":{"songs":[
				{"id":1,"name":"Song","duration":200000,"artists":[{"name":"Somebody Else"}]},
				{"id":2,"name":"Song","duration":181000,"artists":[{"name":"The Band"}]},
				{"id":3,"name":"Song (Live)","duration":180000,"artists":[{"name":"The Band"}]}]}}`))
		case "/api/song/lyric":
			switch r.URL.Query().Get("id") {
			case "2":
				_, _ = w.Write([]byte(`{"lrc":{"lyric":"[00:01.00]hello world"},"yrc":{"lyric":""}}`))
			case "3":
				_, _ = w.Write([]byte(`{"lrc":{"lyric":"[00:01.00]hello world"},"yrc":{"lyric":"[1000,800](1000,400,0)hello (1400,400,0)world"}}`))
			default:
				t.Errorf("fetched lyrics for unexpected id %s", r.URL.Query().Get("id"))
				_, _ = w.Write([]byte(`{}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewClient()
	c.neteaseURL = srv.URL
	res, err := c.FetchFrom(context.Background(), SourceNetEase, Query{Artist: "The Band", Title: "Song", Duration: 180 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !res.WordSynced || res.Source != SourceNetEase || len(res.Lines[0].Words) != 2 {
		t.Errorf("want word-synced NetEase lyrics, got %+v", res)
	}
}

func TestTranslatePerLineLanguages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tl") != "ru" {
			t.Errorf("tl = %q", r.URL.Query().Get("tl"))
		}
		_ = r.ParseForm()
		if q := r.PostForm["q"]; strings.Join(q, "|") != "bonjour|I love you|привет|J.B.G." {
			t.Errorf("q = %q", q)
		}
		// Each line comes back with its own detected language.
		_, _ = w.Write([]byte(`[["привет","fr"],["я люблю тебя","en"],["привет","ru"],["J.B.G.","en"]]`))
	}))
	defer srv.Close()
	c := NewClient()
	c.translateURL = srv.URL
	got, err := c.Translate(context.Background(), []Line{
		{Text: "bonjour"}, {Text: ""}, {Text: "I love you"}, {Text: "привет"}, {Text: "J.B.G."},
	}, "ru")
	if err != nil {
		t.Fatal(err)
	}
	// English lines in a French song are translated too; the Russian line
	// (already the target) and the unchanged name get nothing.
	if strings.Join(got.Lines, "|") != "привет||я люблю тебя||" {
		t.Errorf("lines = %q", got.Lines)
	}
	if got.Source != "fr" {
		t.Errorf("source = %q, want fr (non-English preferred)", got.Source)
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[["привет","ru"]]`))
	}))
	defer srv2.Close()
	c.translateURL = srv2.URL
	if _, err := c.Translate(context.Background(), []Line{{Text: "привет"}}, "ru"); err != ErrSameLanguage {
		t.Errorf("err = %v, want ErrSameLanguage", err)
	}
}

func TestSongLanguagePrefersNonEnglish(t *testing.T) {
	if got := songLanguage(map[string]int{"en": 6, "fr": 3}); got != "fr" {
		t.Errorf("got %q, want fr", got)
	}
	if got := songLanguage(map[string]int{"en": 20, "fr": 1}); got != "en" {
		t.Errorf("a stray line shouldn't flip an English song, got %q", got)
	}
	if got := songLanguage(map[string]int{"ja": 5, "ko": 2}); got != "ja" {
		t.Errorf("got %q", got)
	}
}

func TestDefaultTranslateLangPrefersNonEnglishLocale(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ADDRESS", "ru_RU.UTF-8")
	if got := DefaultTranslateLang(); got != "ru" {
		t.Errorf("got %q, want ru", got)
	}
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	if got := DefaultTranslateLang(); got != "de" {
		t.Errorf("LC_ALL should win, got %q", got)
	}
}
