package ytmusic

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // SAPISIDHASH requires SHA-1 for compatibility with YouTube
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/provider/ytdlp"
)

// musicAccount talks to YouTube Music's browser endpoint using a Netscape
// cookies file. Credentials never leave Google's music.youtube.com host.
type musicAccount struct {
	path     string
	client   *http.Client
	endpoint string
}

func newMusicAccount(path string) *musicAccount {
	if strings.HasPrefix(path, "~/") {
		if h, e := os.UserHomeDir(); e == nil {
			path = filepath.Join(h, path[2:])
		}
	}
	if !strings.ContainsAny(path, `/\`) && !strings.HasSuffix(strings.ToLower(path), ".txt") {
		return nil
	}
	return &musicAccount{path: path, client: &http.Client{Timeout: 20 * time.Second}, endpoint: "https://music.youtube.com/youtubei/v1/browse"}
}
func (a *musicAccount) cookies() (string, string, error) {
	f, err := os.Open(a.path)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()
	var pairs []string
	seenCookies := map[string]bool{}
	var sid, secureSID string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimPrefix(sc.Text(), "#HttpOnly_")
		if strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 7 || (cols[0] != "youtube.com" && !strings.HasSuffix(cols[0], ".youtube.com")) {
			continue
		}
		if !seenCookies[cols[5]] {
			pairs = append(pairs, cols[5]+"="+cols[6])
			seenCookies[cols[5]] = true
		}
		if cols[5] == "SAPISID" {
			sid = cols[6]
		}
		if cols[5] == "__Secure-3PAPISID" {
			secureSID = cols[6]
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if secureSID != "" {
		sid = secureSID
	}
	if sid == "" {
		return "", "", fmt.Errorf("cookies.txt lacks YouTube SAPISID; export a signed-in YouTube Music session")
	}
	return strings.Join(pairs, "; "), sid, nil
}
func (a *musicAccount) browse(ctx context.Context, id string) ([]ytdlp.Entry, error) {
	cookie, sid, err := a.cookies()
	if err != nil {
		return nil, err
	}
	var out []ytdlp.Entry
	seen := map[string]bool{}
	token := ""
	for range 100 {
		body := map[string]any{"context": map[string]any{"client": map[string]string{"clientName": "WEB_REMIX", "clientVersion": "1." + time.Now().UTC().Format("20060102") + ".01.00"}, "user": map[string]any{}}, "browseId": id}
		if token != "" {
			delete(body, "browseId")
			body["continuation"] = token
		}
		payload, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		timestamp := time.Now().Unix()
		sum := sha1.Sum([]byte(fmt.Sprintf("%d %s https://music.youtube.com", timestamp, sid))) //nolint:gosec // required by the SAPISIDHASH protocol
		req.Header.Set("Authorization", fmt.Sprintf("SAPISIDHASH %d_%x", timestamp, sum))
		req.Header.Set("Cookie", cookie)
		req.Header.Set("Origin", "https://music.youtube.com")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Origin", "https://music.youtube.com")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0")
		req.Header.Set("X-Goog-AuthUser", "0")
		res, err := a.client.Do(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
		_ = res.Body.Close()
		if err != nil {
			return nil, err
		}
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("YouTube Music HTTP %d: check the exported signed-in cookies", res.StatusCode)
		}
		var root any
		if err = json.Unmarshal(data, &root); err != nil {
			return nil, err
		}
		next := ""
		recognized := false
		before := len(out)
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case []any:
				for _, z := range x {
					walk(z)
				}
			case map[string]any:
				for _, kind := range []string{"musicResponsiveListItemRenderer", "musicTwoRowItemRenderer"} {
					if raw, ok := x[kind].(map[string]any); ok {
						recognized = true
						e := musicEntry(raw, id == "FEmusic_liked_playlists")
						if e.ID != "" && !seen[e.ID] {
							seen[e.ID] = true
							out = append(out, e)
						}
						return
					}
				}
				if c, ok := x["nextContinuationData"].(map[string]any); ok {
					next, _ = c["continuation"].(string)
				}
				if c, ok := x["continuationCommand"].(map[string]any); ok {
					next, _ = c["token"].(string)
				}
				for _, z := range x {
					walk(z)
				}
			}
		}
		walk(root)
		if recognized && len(out) == before && before == 0 {
			return nil, fmt.Errorf("YouTube Music returned library entries without readable IDs; check the signed-in account and response format")
		}
		if !recognized && len(out) == 0 {
			return nil, fmt.Errorf("YouTube Music requests sign-in; export fresh cookies from a private browser session")
		}
		if next == "" || next == token {
			break
		}
		token = next
	}
	return out, nil
}
func musicEntry(raw map[string]any, wantPlaylist bool) ytdlp.Entry {
	var e ytdlp.Entry
	var video, playlist string
	// Music returns the actual square cover inside the item's thumbnail.
	// Keep it rather than substituting the video's generic preview.
	var readThumb func(any)
	readThumb = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				readThumb(item)
			}
		case map[string]any:
			if raw, ok := x["thumbnails"].([]any); ok {
				for _, item := range raw {
					if obj, ok := item.(map[string]any); ok {
						u, _ := obj["url"].(string)
						w, _ := obj["width"].(float64)
						h, _ := obj["height"].(float64)
						if u != "" {
							e.Thumbnails = append(e.Thumbnails, ytdlp.Thumb{URL: u, Width: int(w), Height: int(h)})
						}
					}
				}
				return
			}
			for _, value := range x {
				readThumb(value)
			}
		}
	}
	readThumb(raw["thumbnail"])
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, z := range x {
				walk(z)
			}
		case map[string]any:
			if data, ok := x["playlistItemData"].(map[string]any); ok {
				if id, ok := data["videoId"].(string); ok && video == "" {
					video = id
				}
			}
			if w, ok := x["watchEndpoint"].(map[string]any); ok {
				if id, ok := w["videoId"].(string); ok && video == "" {
					video = id
				}
			}
			if b, ok := x["browseEndpoint"].(map[string]any); ok {
				if id, ok := b["browseId"].(string); ok && strings.HasPrefix(id, "VL") {
					playlist = strings.TrimPrefix(id, "VL")
				}
			}
			if w, ok := x["watchPlaylistEndpoint"].(map[string]any); ok {
				if id, ok := w["playlistId"].(string); ok {
					playlist = id
				}
			}
			for _, z := range x {
				walk(z)
			}
		}
	}
	walk(raw)
	if wantPlaylist {
		e.ID = playlist
	} else {
		e.ID = video
	}
	// Display text lives in the ordered flex columns; map traversal order is random.
	columnText := func(v any) string {
		var result string
		var visit func(any)
		visit = func(v any) {
			if result != "" {
				return
			}
			switch x := v.(type) {
			case []any:
				for _, z := range x {
					visit(z)
				}
			case map[string]any:
				if runs, ok := x["runs"].([]any); ok {
					var b strings.Builder
					for _, r := range runs {
						if m, ok := r.(map[string]any); ok {
							if t, ok := m["text"].(string); ok {
								b.WriteString(t)
							}
						}
					}
					result = b.String()
					return
				}
				for _, z := range x {
					visit(z)
				}
			}
		}
		visit(v)
		return result
	}
	if cols, ok := raw["flexColumns"].([]any); ok {
		if len(cols) > 0 {
			e.Title = columnText(cols[0])
		}
		if len(cols) > 1 {
			e.Artist = columnText(cols[1])
		}
	}
	if e.Title == "" {
		e.Title = columnText(raw["title"])
	}
	return e
}
