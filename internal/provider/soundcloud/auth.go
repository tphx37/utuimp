// Package soundcloud is a SoundCloud provider built on the official API
// (api.soundcloud.com). It needs the user's own app (Client ID and
// Client Secret from soundcloud.com/you/apps); sign-in is OAuth 2.1 with
// PKCE through a loopback redirect.
package soundcloud

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RedirectURI is where SoundCloud sends the browser back after sign-in; the
// app must list it as its redirect URI.
const RedirectURI = "http://127.0.0.1:5590/soundcloud"

var (
	secureURL   = "https://secure.soundcloud.com" //nolint:gochecknoglobals // overridden in tests
	redirectURI = RedirectURI                     //nolint:gochecknoglobals // overridden in tests
)

// Token is an OAuth token pair. SoundCloud refresh tokens are single use:
// save the new one after every refresh.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int64     `json:"expires_in"`
	Expiry       time.Time `json:"-"`
}

// App is the user's registered SoundCloud app.
type App struct {
	ClientID, ClientSecret string
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Login opens the consent page with open, waits for the redirect to
// RedirectURI and exchanges the code for tokens.
func Login(ctx context.Context, app App, open func(string) error) (*Token, error) {
	if app.ClientID == "" || app.ClientSecret == "" {
		return nil, errors.New("soundcloud: no app Client ID / Client Secret (run utuimp setup)")
	}
	u, err := url.Parse(redirectURI)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", u.Host)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", u.Host, err)
	}
	defer func() { _ = ln.Close() }()

	verifier := randomString(48)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randomString(16)

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != u.Path {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		var res result
		switch {
		case q.Get("state") != state:
			res.err = errors.New("sign-in state mismatch")
		case q.Get("error") != "":
			res.err = fmt.Errorf("sign-in refused: %s %s", q.Get("error"), q.Get("error_description"))
		default:
			res.code = q.Get("code")
		}
		msg := "Signed in to SoundCloud. You can close this tab."
		if res.err != nil {
			msg = "SoundCloud sign-in failed: " + res.err.Error()
		}
		_, _ = fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>utuimp</title><p style=\"font:16px sans-serif;margin:3em\">%s</p>", html.EscapeString(msg))
		select {
		case done <- res:
		default:
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	auth := url.Values{}
	auth.Set("client_id", app.ClientID)
	auth.Set("redirect_uri", redirectURI)
	auth.Set("response_type", "code")
	auth.Set("code_challenge", challenge)
	auth.Set("code_challenge_method", "S256")
	auth.Set("state", state)
	if open != nil {
		_ = open(secureURL + "/authorize?" + auth.Encode())
	}

	var res result
	select {
	case res = <-done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if res.err != nil {
		return nil, res.err
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", app.ClientID)
	form.Set("client_secret", app.ClientSecret)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)
	form.Set("code", res.code)
	return tokenRequest(ctx, form)
}

// Refresh exchanges a (single-use) refresh token for new tokens.
func Refresh(ctx context.Context, app App, refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", app.ClientID)
	form.Set("client_secret", app.ClientSecret)
	form.Set("refresh_token", refreshToken)
	form.Set("redirect_uri", redirectURI)
	return tokenRequest(ctx, form)
}

func tokenRequest(ctx context.Context, form url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, secureURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json; charset=utf-8")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return nil, fmt.Errorf("soundcloud token: %s %s", e.Error, e.Description)
	}
	var tok Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	if tok.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	return &tok, nil
}
