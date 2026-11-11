// Package spotify is a Spotify provider built on the Web API. Sign-in uses
// the public OAuth client that librespot uses, so the user only has to
// approve access; the same token drives librespot, which plays the audio.
package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tphx37/utuimp/internal/signin"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultClientID is the public OAuth client librespot signs in with.
const DefaultClientID = "65b708073fc0480ea92a077233ca87bd"

// RedirectURI is the loopback address librespot's client redirects to.
const RedirectURI = "http://127.0.0.1:5588/login"

var (
	accountsURL = "https://accounts.spotify.com" //nolint:gochecknoglobals // overridden in tests
	redirectURI = RedirectURI                    //nolint:gochecknoglobals // overridden in tests
)

// scopes: librespot's own list (streaming for audio, the rest for the Web
// API the library and the remote use).
var scopes = []string{
	"app-remote-control", "playlist-modify", "playlist-modify-private", "playlist-modify-public",
	"playlist-read", "playlist-read-collaborative", "playlist-read-private", "streaming",
	"user-follow-modify", "user-follow-read", "user-library-modify", "user-library-read",
	"user-modify", "user-modify-playback-state", "user-modify-private", "user-personalized",
	"user-read-currently-playing", "user-read-email", "user-read-play-history",
	"user-read-playback-position", "user-read-playback-state", "user-read-private",
	"user-read-recently-played", "user-top-read",
}

// publicScopes: what Spotify lets any other app ask for. The extra scopes
// in the list above belong to Spotify's own clients; asking for them with
// the user's own Client ID fails with invalid_scope.
var publicScopes = []string{
	"playlist-modify-private", "playlist-modify-public", "playlist-read-collaborative",
	"playlist-read-private", "streaming", "user-follow-modify", "user-follow-read",
	"user-library-modify", "user-library-read", "user-modify-playback-state",
	"user-read-currently-playing", "user-read-email", "user-read-playback-position",
	"user-read-playback-state", "user-read-private", "user-read-recently-played",
	"user-top-read",
}

// scopesFor returns the scopes to request for clientID.
func scopesFor(clientID string) []string {
	if clientID == DefaultClientID {
		return scopes
	}
	return publicScopes
}

// Token is an OAuth token pair.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int64     `json:"expires_in"`
	Expiry       time.Time `json:"-"`
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Login runs the authorization-code flow with PKCE: it opens the consent
// page with open, waits for Spotify to redirect back to RedirectURI, and
// exchanges the code for tokens.
func Login(ctx context.Context, clientID string, open func(string) error) (*Token, error) {
	if clientID == "" {
		clientID = DefaultClientID
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
	// The page the browser opens first (see package signin): its button
	// goes to Spotify's own page, which comes back to u.Path.
	var consent string
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && u.Path != "/" {
			signin.Render(w, signin.Page{
				Service: "Spotify", Colors: signin.Spotify,
				Text:   "Sign in to Spotify to play your library in the terminal.",
				Button: "Continue with Spotify", ButtonURL: consent,
			})
			return
		}
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
			res.err = fmt.Errorf("sign-in refused: %s", q.Get("error"))
		default:
			res.code = q.Get("code")
		}
		page := signin.Page{Service: "Spotify", Colors: signin.Spotify, Done: res.err == nil}
		if res.err != nil {
			page.Error = res.err.Error()
		}
		signin.Render(w, page)
		select {
		case done <- res:
		default:
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	auth := url.Values{}
	auth.Set("client_id", clientID)
	auth.Set("response_type", "code")
	auth.Set("redirect_uri", redirectURI)
	auth.Set("code_challenge_method", "S256")
	auth.Set("code_challenge", challenge)
	auth.Set("state", state)
	auth.Set("scope", strings.Join(scopesFor(clientID), " "))
	consent = accountsURL + "/authorize?" + auth.Encode()
	if open != nil {
		_ = open("http://" + u.Host + "/")
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
	form.Set("code", res.code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", clientID)
	form.Set("code_verifier", verifier)
	return tokenRequest(ctx, form)
}

// Refresh exchanges a refresh token for a new access token.
func Refresh(ctx context.Context, clientID, refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)
	tok, err := tokenRequest(ctx, form)
	if err == nil && tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	return tok, err
}

func tokenRequest(ctx context.Context, form url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, accountsURL+"/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
		return nil, fmt.Errorf("spotify token: %s %s", e.Error, e.Description)
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
