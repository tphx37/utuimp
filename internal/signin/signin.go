// Package signin is the browser pages of the services' sign-ins: one look
// for all of them (see page.html), and the small local server behind the
// pages that only show a code and wait.
package signin

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"sync"
	"time"
)

//go:embed page.html
var pageHTML string

var base = template.Must(template.New("signin").Parse(pageHTML))

// Base is a template set holding the shared parts ("style", "logo",
// "ribbons"), for a page with a script of its own to be parsed into.
func Base() *template.Template { return template.Must(base.Clone()) }

// The ribbons' colours of each service: its own and white.
var (
	Apple      = [][3]int{{250, 36, 60}, {255, 255, 255}}
	Spotify    = [][3]int{{30, 215, 96}, {255, 255, 255}}
	SoundCloud = [][3]int{{255, 85, 0}, {255, 255, 255}}
	Yandex     = [][3]int{{255, 204, 0}, {255, 255, 255}}
)

// Page is what a sign-in page shows.
type Page struct {
	Service string   // "Spotify"
	Colors  [][3]int // the ribbons' colours
	Text    string   // what to do
	Code    string   // a code to type on the service's page, if any
	// The button that goes to the service's page (in a new tab when the
	// page itself has to stay, to show the code and wait).
	Button, ButtonURL string
	NewTab            bool
	Poll              bool   // ask /status until the sign-in is over
	Done              bool   // it is over, and went well
	Error             string // it is over, and didn't
}

// Render writes the page.
func Render(w http.ResponseWriter, p Page) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	if err := base.ExecuteTemplate(w, "page", p); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// ServeWaiting serves p on a port of its own on this computer, for a
// sign-in that is finished elsewhere (a code typed on the service's page):
// the page asks how it went until finish is called, with the error or nil.
// The server goes away a little after that, when the page has had time to
// say so.
func ServeWaiting(p Page) (url string, finish func(error), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	var (
		mu    sync.Mutex
		done  bool
		fail  string
		once  sync.Once
		p0    = p
		serve = http.NewServeMux()
	)
	p0.Poll = true
	serve.HandleFunc("/{$}", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		page := p0
		if done {
			page.Poll, page.Done, page.Error = false, fail == "", fail
			page.Code, page.ButtonURL = "", ""
		}
		mu.Unlock()
		Render(w, page)
	})
	serve.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		st := map[string]any{"done": done, "error": fail}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(st)
	})
	srv := &http.Server{Handler: serve, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	finish = func(e error) {
		once.Do(func() {
			mu.Lock()
			done = true
			if e != nil {
				fail = e.Error()
			}
			mu.Unlock()
			go func() {
				time.Sleep(20 * time.Second)
				_ = srv.Close()
			}()
		})
	}
	return "http://" + ln.Addr().String() + "/", finish, nil
}
