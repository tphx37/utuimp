package radio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolvePlaylist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, "<html>station page</html>")
		case "/station.m3u":
			_, _ = fmt.Fprint(w, "#EXTM3U\n/live.mp3\n")
		case "/station.pls":
			_, _ = fmt.Fprint(w, "[playlist]\nFile1=https://example.org/stream\n")
		case "/live.m3u8":
			_, _ = fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:10\nsegment.ts\n")
		}
	}))
	defer srv.Close()
	for _, tc := range []struct{ path, want string }{{"/station.m3u", srv.URL + "/live.mp3"}, {"/station.pls", "https://example.org/stream"}, {"/live.m3u8", srv.URL + "/live.m3u8"}, {"/live.mp3", srv.URL + "/live.mp3"}} {
		got, err := resolvePlaylist(context.Background(), srv.URL+tc.path)
		if err != nil || got != tc.want {
			t.Errorf("%s: %q %v", tc.path, got, err)
		}
	}
}

func TestWebPageIsNotTreatedAsAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}))
	defer srv.Close()
	for _, link := range []string{srv.URL + "/station"} {
		if _, err := resolvePlaylist(context.Background(), link); err == nil {
			t.Errorf("%s: expected a useful error for a web page", link)
		}
	}
}

func TestHTMLPlayerLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/audio":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<audio controls src="/stream?x=1&amp;y=2"></audio>`)
		case "/source-untyped":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<audio><source src="/stream"></audio>`)
		case "/source":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<audio><source type="audio/mpeg" src="https://stream.example/radio"></audio>`)
		case "/meta":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<meta content="/stream.mp3" property="og:audio">`)
		case "/headless":
			if r.Method == "HEAD" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<audio src="/live.mp3"></audio>`)
		}
	}))
	defer srv.Close()
	for _, tc := range []struct{ input, want string }{{"/audio", srv.URL + "/stream?x=1&y=2"}, {"/source", "https://stream.example/radio"}, {"/source-untyped", srv.URL + "/stream"}, {"/meta", srv.URL + "/stream.mp3"}, {"/headless", srv.URL + "/live.mp3"}} {
		got, err := resolvePlaylist(context.Background(), srv.URL+tc.input)
		if err != nil || got != tc.want {
			t.Errorf("%s: %q %v", tc.input, got, err)
		}
	}
}
