package yandex

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth app credentials of the Yandex Music client, as used by the
// reverse-engineered libraries for the device-code sign-in flow.
const (
	oauthBase   = "https://oauth.yandex.ru"
	oauthClient = "23cabbbdc6cd418abb4b39c32c41195d"
	oauthSecret = "53bc75238f0c4d08a118e51fe9203300"
	deviceName  = "utuimp"
)

var oauthURL = oauthBase //nolint:gochecknoglobals // overridden in tests

// DeviceCode is what the user needs to approve the sign-in.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

// Token is an OAuth token pair.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	// Expiry is filled in from ExpiresIn when the token is received.
	Expiry time.Time `json:"-"`
}

type oauthError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

func oauthPost(ctx context.Context, path string, form url.Values, out any) (*oauthError, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		var oe oauthError
		if json.Unmarshal(data, &oe) == nil && oe.Error != "" {
			return &oe, nil
		}
		return nil, fmt.Errorf("oauth: HTTP %d", resp.StatusCode)
	}
	return nil, json.Unmarshal(data, out)
}

func randomDeviceID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// RequestDeviceCode starts the device sign-in flow.
func RequestDeviceCode(ctx context.Context) (*DeviceCode, error) {
	form := url.Values{}
	form.Set("client_id", oauthClient)
	form.Set("device_id", randomDeviceID())
	form.Set("device_name", deviceName)
	var dc DeviceCode
	oe, err := oauthPost(ctx, "/device/code", form, &dc)
	if err != nil {
		return nil, err
	}
	if oe != nil {
		return nil, fmt.Errorf("oauth: %s %s", oe.Error, oe.Description)
	}
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	return &dc, nil
}

// WaitForToken polls until the user approves the device code (or it expires).
func WaitForToken(ctx context.Context, dc *DeviceCode) (*Token, error) {
	deadline := time.Now().Add(time.Duration(max(dc.ExpiresIn, 60)) * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(dc.Interval) * time.Second):
		}
		form := url.Values{}
		form.Set("grant_type", "device_code")
		form.Set("code", dc.DeviceCode)
		form.Set("client_id", oauthClient)
		form.Set("client_secret", oauthSecret)
		var tok Token
		oe, err := oauthPost(ctx, "/token", form, &tok)
		if err != nil {
			return nil, err
		}
		if oe != nil {
			if oe.Error == "authorization_pending" || oe.Error == "slow_down" {
				continue
			}
			return nil, fmt.Errorf("sign-in failed: %s %s", oe.Error, oe.Description)
		}
		tok.Expiry = expiry(tok.ExpiresIn)
		return &tok, nil
	}
	return nil, errors.New("sign-in code expired")
}

// Refresh exchanges a refresh token for a new token pair.
func Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", oauthClient)
	form.Set("client_secret", oauthSecret)
	var tok Token
	oe, err := oauthPost(ctx, "/token", form, &tok)
	if err != nil {
		return nil, err
	}
	if oe != nil {
		return nil, fmt.Errorf("refresh: %s %s", oe.Error, oe.Description)
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	tok.Expiry = expiry(tok.ExpiresIn)
	return &tok, nil
}

func expiry(seconds int64) time.Time {
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(seconds) * time.Second)
}
