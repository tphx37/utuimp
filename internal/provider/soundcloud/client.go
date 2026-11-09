package soundcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrUnauthorized means the sign-in no longer works.
var ErrUnauthorized = errors.New("soundcloud: signed out (run utuimp --relogin=soundcloud)")

// Client calls the SoundCloud API, refreshing the token when it expires.
type Client struct {
	http *http.Client
	base string
	app  App

	mu      sync.Mutex
	tok     Token
	onToken func(Token) // persists refreshed tokens
}

// NewClient returns a client for a signed-in user. onToken is called with
// every refreshed token.
func NewClient(app App, tok Token, onToken func(Token)) *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second}, base: "https://api.soundcloud.com",
		app: app, tok: tok, onToken: onToken}
}

// SetBaseURL points the client at another API host (tests).
func (c *Client) SetBaseURL(u string) { c.base = u }

func (c *Client) accessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.tok.AccessToken != "" && (c.tok.Expiry.IsZero() || time.Until(c.tok.Expiry) > time.Minute) {
		return c.tok.AccessToken, nil
	}
	if c.tok.RefreshToken == "" {
		if c.tok.AccessToken == "" {
			return "", ErrUnauthorized
		}
		return c.tok.AccessToken, nil
	}
	nt, err := Refresh(ctx, c.app, c.tok.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
	c.tok = *nt
	if c.onToken != nil {
		c.onToken(*nt)
	}
	return nt.AccessToken, nil
}

// isAPI reports whether u is on the API host (only it gets the token).
func (c *Client) isAPI(u string) bool {
	pu, err := url.Parse(u)
	bu, berr := url.Parse(c.base)
	return err == nil && berr == nil && pu.Host == bu.Host
}

// do performs an authorized request and returns the response (the caller
// closes the body). A 401 refreshes the token and retries once.
func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any) (*http.Response, error) {
	u := path
	if !strings.HasPrefix(u, "http") {
		u = c.base + path
	}
	if len(q) > 0 {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + q.Encode()
	}
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		if c.isAPI(u) {
			tok, err := c.accessToken(ctx, attempt > 0)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "OAuth "+tok)
		}
		req.Header.Set("Accept", "application/json; charset=utf-8")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 && c.isAPI(u) {
			_ = resp.Body.Close()
			continue
		}
		return resp, nil
	}
	return nil, ErrUnauthorized
}

// call performs a request; out (if not nil) receives the decoded JSON.
func (c *Client) call(ctx context.Context, method, path string, q url.Values, body, out any) error {
	resp, err := c.do(ctx, method, path, q, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return errors.New("soundcloud: too many requests, try again later")
	case resp.StatusCode >= 300:
		var e struct {
			Message string `json:"message"`
			Error   string `json:"error"`
			Errors  []struct {
				ErrorMessage string `json:"error_message"`
			} `json:"errors"`
		}
		_ = json.Unmarshal(data, &e)
		msg := e.Message
		if msg == "" {
			msg = e.Error
		}
		if msg == "" && len(e.Errors) > 0 {
			msg = e.Errors[0].ErrorMessage
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("soundcloud: %s (HTTP %d)", msg, resp.StatusCode)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	return c.call(ctx, http.MethodGet, path, q, nil, out)
}

// Do performs an API request (for the sign-in check).
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	return c.call(ctx, method, path, q, body, out)
}
