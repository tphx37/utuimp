package radio

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// resolvePlaylist follows ordinary M3U/PLS station shortcuts. HLS playlists
// are left to the audio backend, which needs their segment information.
func resolvePlaylist(ctx context.Context, link string) (string, error) {
	u, err := url.Parse(link)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(path.Ext(u.Path))
	if ext != ".m3u" && ext != ".pls" && ext != ".m3u8" {
		switch ext {
		case ".mp3", ".aac", ".ogg", ".opus", ".flac", ".m4a", ".wav":
			return link, nil
		}
		// A share page is not an audio stream. HEAD is advisory: many
		// radio servers do not implement it, so only reject clear HTML.
		req, e := http.NewRequestWithContext(ctx, http.MethodHead, link, nil)
		if e == nil {
			client := &http.Client{Timeout: 5 * time.Second}
			if res, e := client.Do(req); e == nil {
				_ = res.Body.Close()
				if res.StatusCode == http.StatusMethodNotAllowed || res.StatusCode == http.StatusNotImplemented || (res.StatusCode >= 200 && res.StatusCode < 300 && strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "text/html")) {
					return resolveHTML(ctx, res.Request.URL.String())
				}
			}
		}
		return link, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	u = res.Request.URL
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("radio playlist: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 256<<10))
	if err != nil {
		return "", err
	}
	if strings.Contains(string(data), "#EXT-X-") {
		return link, nil
	}
	scan := bufio.NewScanner(strings.NewReader(string(data)))
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if ext == ".pls" {
			key, val, ok := strings.Cut(line, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "File1") {
				continue
			}
			line = strings.TrimSpace(val)
		}
		ref, e := url.Parse(line)
		if e != nil {
			continue
		}
		resolved := u.ResolveReference(ref)
		if (resolved.Scheme == "https" || resolved.Scheme == "http") && resolved.Host != "" {
			return resolved.String(), nil
		}
	}
	if err = scan.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("radio playlist contains no stream URL")
}
