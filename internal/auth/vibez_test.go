package auth

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/config"
)

func jwtExpiring(exp time.Time) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"ES256"}`)) + "." + enc([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix()))) + ".sig"
}

func TestJWTExpiry(t *testing.T) {
	exp := time.Unix(1_900_000_000, 0)
	if got := jwtExpiry(jwtExpiring(exp)); !got.Equal(exp) {
		t.Errorf("jwtExpiry = %v, want %v", got, exp)
	}
	if !jwtExpiry("not.a.jwt").IsZero() {
		t.Error("garbage should have no expiry")
	}
}

func tarGz(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestExtractExecutableTakesOnlyTheBinary(t *testing.T) {
	arc := tarGz(t, map[string]string{"README.md": "readme", "assets/logo.png": "png", "vibez": "BINARY"})
	got, err := extractExecutable(arc, "vibez", false)
	if err != nil || string(got) != "BINARY" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := extractExecutable(tarGz(t, map[string]string{"sub/vibez": "x"}), "vibez", false); err == nil {
		t.Error("a nested path must not count as the binary")
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("archive")
	sum := sha256.Sum256(data)
	sums := []byte(hex.EncodeToString(sum[:]) + "  vibez_linux_amd64.tar.gz\n")
	if err := verifyChecksum(data, "vibez_linux_amd64.tar.gz", sums); err != nil {
		t.Errorf("valid checksum rejected: %v", err)
	}
	if err := verifyChecksum([]byte("tampered"), "vibez_linux_amd64.tar.gz", sums); err == nil {
		t.Error("tampered archive accepted")
	}
	if err := verifyChecksum(data, "other.tar.gz", sums); err == nil {
		t.Error("unlisted asset accepted")
	}
}

func TestRunVibezForTokenUsesSandbox(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary")
	}
	tok := jwtExpiring(time.Now().Add(7 * 24 * time.Hour))
	// The fake vibez checks it was sandboxed (HOME is not the real one,
	// --no-update passed, its config exists) and then caches a token.
	script := fmt.Sprintf(`#!/bin/sh
[ "$1" = "--no-update" ] || exit 3
[ -f "$XDG_CONFIG_HOME/vibez/config.json" ] || exit 4
case "$HOME" in */utuimp-token-*) ;; *) exit 5;; esac
/bin/mkdir -p "$XDG_CACHE_HOME/vibez"
printf '{"token":"%s","expires_at":%d}' > "$XDG_CACHE_HOME/vibez/devtoken.json"
/bin/sleep 30
`, tok, time.Now().Add(7*24*time.Hour).Unix())
	bin := filepath.Join(t.TempDir(), "vibez")
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil { //nolint:gosec // a fake program: it must run
		t.Fatal(err)
	}
	start := time.Now()
	c, err := runVibezForToken(context.Background(), bin)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != tok {
		t.Errorf("token = %q", c.Token)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("the process should be stopped as soon as the token appears")
	}
}

func TestResolveWithoutBroker(t *testing.T) {
	now := time.Now()
	t.Cleanup(func() { vibezRefresh = nil })
	calls := 0
	fresh := jwtExpiring(now.Add(7 * 24 * time.Hour))
	vibezRefresh = func(context.Context, string) (cachedToken, error) {
		calls++
		return cachedToken{Token: fresh, ExpiresAt: now.Add(7 * 24 * time.Hour).Unix()}, nil
	}
	_ = os.Remove(cachePath())

	// Plenty of life left: keep it, no refresh.
	long := jwtExpiring(now.Add(6 * 24 * time.Hour))
	cfg := &config.Config{AppleDeveloperToken: long}
	if err := resolveWithoutBroker(cfg, now); err != nil || cfg.AppleDeveloperToken != long || calls != 0 {
		t.Fatalf("long-lived token: tok changed=%v calls=%d err=%v", cfg.AppleDeveloperToken != long, calls, err)
	}

	// Expiring: refresh, and cache the result for next time.
	cfg = &config.Config{AppleTokenSource: config.AppleTokenVibez, AppleDeveloperToken: jwtExpiring(now.Add(time.Hour))}
	if err := resolveWithoutBroker(cfg, now); err != nil || cfg.AppleDeveloperToken != fresh || calls != 1 {
		t.Fatalf("expiring token: got fresh=%v calls=%d err=%v", cfg.AppleDeveloperToken == fresh, calls, err)
	}
	cfg = &config.Config{AppleTokenSource: config.AppleTokenVibez, AppleDeveloperToken: jwtExpiring(now.Add(time.Hour))}
	if err := resolveWithoutBroker(cfg, now); err != nil || cfg.AppleDeveloperToken != fresh || calls != 1 {
		t.Fatalf("second start should use the cache, calls=%d", calls)
	}

	// Refresh failing keeps the old token rather than failing start-up.
	_ = os.Remove(cachePath())
	vibezRefresh = func(context.Context, string) (cachedToken, error) { return cachedToken{}, errors.New("offline") }
	old := jwtExpiring(now.Add(time.Hour))
	cfg = &config.Config{AppleTokenSource: config.AppleTokenVibez, AppleDeveloperToken: old}
	if err := resolveWithoutBroker(cfg, now); err != nil || cfg.AppleDeveloperToken != old {
		t.Errorf("failed refresh: err=%v kept=%v", err, cfg.AppleDeveloperToken == old)
	}
}

// vibez is only asked for a token once the user has agreed to it.
func TestResolveWithoutBrokerNeedsConsent(t *testing.T) {
	now := time.Now()
	t.Cleanup(func() { vibezRefresh = nil })
	calls := 0
	fresh := jwtExpiring(now.Add(7 * 24 * time.Hour))
	vibezRefresh = func(context.Context, string) (cachedToken, error) {
		calls++
		return cachedToken{Token: fresh, ExpiresAt: now.Add(7 * 24 * time.Hour).Unix()}, nil
	}
	_ = os.Remove(cachePath())

	cfg := &config.Config{}
	if err := resolveWithoutBroker(cfg, now); !errors.Is(err, ErrVibezNotAllowed) || calls != 0 {
		t.Fatalf("not asked yet: err=%v calls=%d", err, calls)
	}
	cfg = &config.Config{AppleTokenSource: config.AppleTokenOwn}
	if err := resolveWithoutBroker(cfg, now); err == nil || errors.Is(err, ErrVibezNotAllowed) || calls != 0 {
		t.Fatalf("own key, no token: err=%v calls=%d", err, calls)
	}
	// A token that still works keeps working without asking vibez.
	old := jwtExpiring(now.Add(time.Hour))
	cfg = &config.Config{AppleDeveloperToken: old}
	if err := resolveWithoutBroker(cfg, now); err != nil || cfg.AppleDeveloperToken != old || calls != 0 {
		t.Fatalf("old token: err=%v calls=%d", err, calls)
	}
	if got := DeveloperTokenInfo().Source; got != SourceOwn {
		t.Errorf("source = %q, want own", got)
	}
	cfg = &config.Config{AppleTokenSource: config.AppleTokenVibez}
	if err := resolveWithoutBroker(cfg, now); err != nil || cfg.AppleDeveloperToken != fresh || calls != 1 {
		t.Fatalf("allowed: err=%v calls=%d", err, calls)
	}
	if got := DeveloperTokenInfo().Source; got != SourceVibez {
		t.Errorf("source = %q, want vibez", got)
	}
}

// GitHub's release CDN sometimes answers 504: the download is retried.
func TestHTTPGetRetriesServerErrors(t *testing.T) {
	old := httpRetryDelays
	httpRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { httpRetryDelays = old }()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	b, err := httpGet(context.Background(), srv.URL, 10)
	if err != nil || string(b) != "ok" || calls != 3 {
		t.Fatalf("got %q, %v after %d calls", b, err, calls)
	}

	// A 404 is not retried.
	srv404 := httptest.NewServer(http.NotFoundHandler())
	defer srv404.Close()
	if _, err := httpGet(context.Background(), srv404.URL, 10); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("404: %v", err)
	}
}
