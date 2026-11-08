package auth

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Upstream token refresh.
//
// utuimp builds carry no developer token of their own. The upstream project,
// simonepelosi/vibez, ships release binaries that fetch a short-lived token
// from its token broker on start-up and cache it before the UI opens. When
// utuimp's token is about to expire it downloads that official release binary
// (just the executable, checksum-verified), runs it inside a throwaway home
// directory — no browser, no updates, no desktop entries, no access to the
// real config — waits for it to write its token cache, stops it, and keeps
// the token.

const (
	upstreamRepo     = "simonepelosi/vibez"
	upstreamName     = "vibez"
	upstreamMaxBin   = 128 << 20 // largest executable accepted from the archive
	upstreamMaxAsset = 128 << 20
	upstreamBinTTL   = 7 * 24 * time.Hour // re-download to pick up new releases
	upstreamRunLimit = 30 * time.Second
)

// upstreamBaseURL serves release assets without the (rate-limited) GitHub API.
var upstreamBaseURL = "https://github.com/" + upstreamRepo + "/releases/latest/download/" //nolint:gochecknoglobals // overridden in tests

// jwtExpiry reads the exp claim of a JWT; zero when it can't be read.
func jwtExpiry(tok string) time.Time {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

// refreshFromUpstream obtains a fresh developer token via the upstream
// release binary, downloading it first if needed.
func refreshFromUpstream(ctx context.Context, cacheDir string) (cachedToken, error) {
	if cacheDir == "" {
		return cachedToken{}, errors.New("no cache directory")
	}
	bin, fresh, err := upstreamBinary(ctx, cacheDir)
	if err != nil {
		return cachedToken{}, err
	}
	tok, err := runUpstreamForToken(ctx, bin)
	if err != nil && !fresh {
		// A stale download may be an old release the broker no longer
		// serves; fetch the current one and try once more.
		_ = os.Remove(bin)
		if bin, _, err = upstreamBinary(ctx, cacheDir); err != nil {
			return cachedToken{}, err
		}
		tok, err = runUpstreamForToken(ctx, bin)
	}
	return tok, err
}

// upstreamBinary returns the cached upstream executable, downloading it
// when missing or older than upstreamBinTTL. fresh reports a new download.
func upstreamBinary(ctx context.Context, cacheDir string) (path string, fresh bool, err error) {
	exe := upstreamName
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	dir := filepath.Join(cacheDir, "upstream")
	path = filepath.Join(dir, exe)
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) < upstreamBinTTL {
		return path, false, nil
	}

	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	asset := fmt.Sprintf("%s_%s_%s%s", upstreamName, runtime.GOOS, runtime.GOARCH, ext)
	logBroker("[upstream] downloading " + upstreamRepo + " " + asset)
	archive, err := httpGet(ctx, upstreamBaseURL+asset, upstreamMaxAsset)
	if err != nil {
		return "", false, fmt.Errorf("download %s: %w", asset, err)
	}
	sums, err := httpGet(ctx, upstreamBaseURL+"checksums.txt", 1<<20)
	if err != nil {
		return "", false, fmt.Errorf("download checksums: %w", err)
	}
	if err := verifyChecksum(archive, asset, sums); err != nil {
		return "", false, err
	}
	data, err := extractExecutable(archive, exe, ext == ".zip")
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o700); err != nil { //nolint:gosec // an executable we run ourselves
		return "", false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func httpGet(ctx context.Context, url string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "utuimp")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("response too large")
	}
	return data, nil
}

// verifyChecksum checks archive against its line in a goreleaser checksums.txt.
func verifyChecksum(archive []byte, asset string, sums []byte) error {
	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])
	for line := range strings.SplitSeq(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == asset {
			if !strings.EqualFold(fields[0], got) {
				return fmt.Errorf("checksum mismatch for %s", asset)
			}
			return nil
		}
	}
	return fmt.Errorf("no checksum listed for %s", asset)
}

// extractExecutable pulls the single top-level file named exe out of a
// .tar.gz or .zip release archive, ignoring everything else in it.
func extractExecutable(archive []byte, exe string, isZip bool) ([]byte, error) {
	if isZip {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name != exe {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer func() { _ = rc.Close() }()
			return readLimited(rc)
		}
		return nil, fmt.Errorf("%s not found in archive", exe)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s not found in archive", exe)
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && strings.TrimPrefix(h.Name, "./") == exe {
			return readLimited(tr)
		}
	}
}

func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, upstreamMaxBin+1))
	if err != nil {
		return nil, err
	}
	if len(data) > upstreamMaxBin {
		return nil, errors.New("executable too large")
	}
	return data, nil
}

// runUpstreamForToken runs bin in a sandbox home and returns the developer
// token it caches. The process is stopped as soon as the cache appears (it
// also exits on its own once it finds no terminal).
func runUpstreamForToken(ctx context.Context, bin string) (cachedToken, error) {
	home, err := os.MkdirTemp("", "utuimp-token-*")
	if err != nil {
		return cachedToken{}, err
	}
	defer func() { _ = os.RemoveAll(home) }()

	cfgDir := filepath.Join(home, ".config")
	cacheDir := filepath.Join(home, ".cache")
	if runtime.GOOS == "darwin" {
		cacheDir = filepath.Join(home, "Library", "Caches")
	}
	env := []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + cfgDir,
		"XDG_CACHE_HOME=" + cacheDir,
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"PATH=" + filepath.Join(home, "no-bin"), // nothing to launch (no browser)
		"TERM=dumb",
	}
	upCfgDir := filepath.Join(cfgDir, upstreamName)
	if runtime.GOOS == "windows" {
		cfgDir = filepath.Join(home, "AppData", "Roaming")
		cacheDir = filepath.Join(home, "AppData", "Local")
		upCfgDir = filepath.Join(cfgDir, upstreamName)
		env = append(env, "APPDATA="+cfgDir, "LOCALAPPDATA="+cacheDir, "USERPROFILE="+home,
			"SystemRoot="+os.Getenv("SystemRoot"))
	}
	if err := os.MkdirAll(upCfgDir, 0o700); err != nil {
		return cachedToken{}, err
	}
	// A placeholder user token keeps it off the sign-in flow; nothing ever
	// reaches Apple with it because the process is stopped first.
	cfg := `{"apple_user_token":"sandbox","storefront":"us","provider":"apple","discord_rpc":false}`
	if err := os.WriteFile(filepath.Join(upCfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		return cachedToken{}, err
	}
	tokenFile := filepath.Join(cacheDir, upstreamName, "devtoken.json")

	ctx, cancel := context.WithTimeout(ctx, upstreamRunLimit)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--no-update") //nolint:gosec // our verified download
	cmd.Env = env
	cmd.Dir = home
	// nil streams go straight to the null device: no pipes for a child of
	// the process to hold open after it is stopped.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return cachedToken{}, err
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	defer func() {
		_ = cmd.Process.Kill()
		<-done
	}()

	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	exited := false
	for {
		if c, err := readCache(tokenFile); err == nil && c.validFor(time.Now(), time.Hour) {
			logBroker("[upstream] obtained a fresh developer token")
			return c, nil
		}
		if exited {
			return cachedToken{}, errors.New("upstream binary exited without caching a token")
		}
		select {
		case <-ctx.Done():
			return cachedToken{}, fmt.Errorf("upstream token: %w", ctx.Err())
		case <-done:
			exited = true // check the cache one last time
		case <-tick.C:
		}
	}
}
