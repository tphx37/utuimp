package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrPremiumRequired is returned by playback calls on a free account.
var ErrPremiumRequired = errors.New("Spotify Premium is required to control playback")

// ErrNoDevice means no Spotify app or device is available to play on.
var ErrNoDevice = errors.New("no Spotify device: open Spotify on this computer or your phone (or run spotifyd)")

// Client calls the Spotify Web API, refreshing the token when it expires.
type Client struct {
	http     *http.Client
	base     string
	clientID string

	mu      sync.Mutex
	tok     Token
	onToken func(Token) // persists refreshed tokens

	// blockedUntil is when Spotify allows requests again after a 429.
	blockedUntil time.Time
}

// maxRetryWait is the longest a request waits out a rate limit by itself;
// longer limits are reported instead.
const maxRetryWait = 15 * time.Second

// ErrClientLimited means Spotify refuses this sign-in app outright (a 429
// with no Retry-After): it throttles the shared public client for everyone.
// The cure is the user's own app Client ID.
var ErrClientLimited = errors.New("Spotify is refusing requests from the shared sign-in app")

// ErrRateLimited is returned while Spotify is rate-limiting this app.
type ErrRateLimited struct{ Wait time.Duration }

func (e ErrRateLimited) Error() string {
	return fmt.Sprintf("spotify: too many requests, try again in %s", e.Wait.Round(time.Second))
}

// waitTurn blocks until any rate-limit pause is over (or fails if it is
// longer than maxRetryWait).
func (c *Client) waitTurn(ctx context.Context) error {
	c.mu.Lock()
	wait := time.Until(c.blockedUntil)
	c.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	if wait > maxRetryWait {
		return ErrRateLimited{Wait: wait}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

// rateLimited records a 429's Retry-After (seconds; default 5s).
func (c *Client) rateLimited(h http.Header) time.Duration {
	wait := 5 * time.Second
	if n, err := strconv.Atoi(strings.TrimSpace(h.Get("Retry-After"))); err == nil && n > 0 {
		wait = time.Duration(n) * time.Second
	}
	c.mu.Lock()
	if until := time.Now().Add(wait); until.After(c.blockedUntil) {
		c.blockedUntil = until
	}
	c.mu.Unlock()
	return wait
}

// NewClient returns a client for an authorized user. onToken is called with
// every refreshed token (to save it).
func NewClient(clientID string, tok Token, onToken func(Token)) *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second}, base: "https://api.spotify.com/v1",
		clientID: clientID, tok: tok, onToken: onToken}
}

func (c *Client) accessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.tok.AccessToken != "" && (c.tok.Expiry.IsZero() || time.Until(c.tok.Expiry) > time.Minute) {
		return c.tok.AccessToken, nil
	}
	if c.tok.RefreshToken == "" {
		return c.tok.AccessToken, nil
	}
	nt, err := Refresh(ctx, c.clientID, c.tok.RefreshToken)
	if err != nil {
		return "", err
	}
	c.tok = *nt
	if c.onToken != nil {
		c.onToken(*nt)
	}
	return nt.AccessToken, nil
}

type apiError struct {
	Error struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Reason  string `json:"reason"`
	} `json:"error"`
}

// call performs a request; body (if not nil) is sent as JSON and out (if not
// nil) receives the decoded response.
func (c *Client) call(ctx context.Context, method, path string, q url.Values, body, out any) error {
	refreshed := false
	for attempt := 0; attempt < 4; attempt++ {
		if err := c.waitTurn(ctx); err != nil {
			return err
		}
		tok, err := c.accessToken(ctx, refreshed)
		if err != nil {
			return err
		}
		u := path
		if !strings.HasPrefix(u, "http") {
			u = c.base + path
		}
		if len(q) > 0 {
			u += "?" + q.Encode()
		}
		var rdr io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized && !refreshed:
			refreshed = true
			continue // expired: refresh and retry once
		case resp.StatusCode == http.StatusTooManyRequests:
			if resp.Header.Get("Retry-After") == "" {
				return ErrClientLimited
			}
			if wait := c.rateLimited(resp.Header); wait > maxRetryWait {
				return ErrRateLimited{Wait: wait}
			}
			continue // waitTurn sits out the pause
		case resp.StatusCode == http.StatusNoContent || len(data) == 0:
			if resp.StatusCode >= 300 {
				return fmt.Errorf("spotify: HTTP %d", resp.StatusCode)
			}
			return nil
		case resp.StatusCode >= 300:
			var e apiError
			_ = json.Unmarshal(data, &e)
			if e.Error.Reason == "PREMIUM_REQUIRED" {
				return ErrPremiumRequired
			}
			if e.Error.Reason == "NO_ACTIVE_DEVICE" {
				return ErrNoDevice
			}
			msg := e.Error.Message
			if msg == "" {
				msg = http.StatusText(resp.StatusCode)
			}
			return fmt.Errorf("spotify: %s (HTTP %d)", msg, resp.StatusCode)
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(data, out)
	}
	return errors.New("spotify: request failed after retries")
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	return c.call(ctx, http.MethodGet, path, q, nil, out)
}

// Do performs an API request (for the Connect player).
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	return c.call(ctx, method, path, q, body, out)
}

// SetBaseURL points the client at another API host (tests).
func (c *Client) SetBaseURL(u string) { c.base = u }

// AccessToken returns a valid access token, refreshing it if needed.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	return c.accessToken(ctx, false)
}
