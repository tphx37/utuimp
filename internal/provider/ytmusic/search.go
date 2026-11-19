package ytmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/ytdlp"
)

// searchRows reads Music results, including metadata omitted by yt-dlp's flat extractor. It does not require an account.
func (p *Provider) searchRows(ctx context.Context, query, params string) ([]map[string]any, error) {
	payload, err := json.Marshal(map[string]any{"context": map[string]any{"client": map[string]string{"clientName": "WEB_REMIX", "clientVersion": "1." + time.Now().UTC().Format("20060102") + ".01.00"}}, "query": query, "params": params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.searchEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	res, err := p.searchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YouTube Music search: HTTP %d", res.StatusCode)
	}
	var root any
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&root); err != nil {
		return nil, err
	}
	var rows []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				walk(item)
			}
		case map[string]any:
			if raw, ok := x["musicResponsiveListItemRenderer"].(map[string]any); ok {
				rows = append(rows, raw)
				return
			}
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(root)
	return rows, nil
}

func columnRuns(raw map[string]any, index int) []any {
	cols, _ := raw["flexColumns"].([]any)
	if index >= len(cols) {
		return nil
	}
	col, _ := cols[index].(map[string]any)
	render, _ := col["musicResponsiveListItemFlexColumnRenderer"].(map[string]any)
	txt, _ := render["text"].(map[string]any)
	runs, _ := txt["runs"].([]any)
	return runs
}
func runText(v any) string { r, _ := v.(map[string]any); s, _ := r["text"].(string); return s }
func runBrowseID(v any) string {
	r, _ := v.(map[string]any)
	nav, _ := r["navigationEndpoint"].(map[string]any)
	b, _ := nav["browseEndpoint"].(map[string]any)
	id, _ := b["browseId"].(string)
	return id
}
func itemBrowseID(raw map[string]any) string {
	nav, _ := raw["navigationEndpoint"].(map[string]any)
	b, _ := nav["browseEndpoint"].(map[string]any)
	id, _ := b["browseId"].(string)
	return id
}

func (p *Provider) searchSongs(ctx context.Context, query string) ([]provider.Track, error) {
	rows, err := p.searchRows(ctx, query, "EgWKAQIIAWoKEAoQAxAEEAkQBQ==")
	if err != nil {
		return nil, err
	}
	out := make([]provider.Track, 0, len(rows))
	seen := map[string]bool{}
	for _, raw := range rows {
		e := musicEntry(raw, false)
		if e.ID == "" || e.Title == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		var artists []string
		for _, run := range columnRuns(raw, 1) {
			text, id := runText(run), runBrowseID(run)
			switch {
			case strings.HasPrefix(id, "UC"):
				artists = append(artists, text)
				if e.ChannelID == "" {
					e.ChannelID = id
				}
			case strings.HasPrefix(id, "MPRE"):
				e.Album = text
			default:
				if d := searchDuration(text); d > 0 {
					e.Duration = d.Seconds()
				}
			}
		}
		if len(artists) > 0 {
			e.Artist = strings.Join(artists, ", ")
		} else {
			e.Artist = strings.TrimSpace(strings.Split(e.Artist, " • ")[0])
		}
		if e.Thumbnail == "" {
			e.Thumbnail = ytdlp.BestThumb(e.Thumbnails)
		}
		out = append(out, p.toTrack(e))
	}
	return out, nil
}

func (p *Provider) searchCollections(ctx context.Context, query, kind string) ([]provider.Album, []provider.Artist, error) {
	params := "EgWKAQIYAWoKEAoQAxAEEAkQBQ=="
	if kind == "artists" {
		params = "EgWKAQIgAWoKEAoQAxAEEAkQBQ=="
	}
	rows, err := p.searchRows(ctx, query, params)
	if err != nil {
		return nil, nil, err
	}
	var albums []provider.Album
	var artists []provider.Artist
	seen := map[string]bool{}
	for _, raw := range rows {
		id := itemBrowseID(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var title strings.Builder
		for _, run := range columnRuns(raw, 0) {
			title.WriteString(runText(run))
		}
		name := provider.Clean(title.String())
		if name == "" {
			continue
		}
		if kind == "artists" {
			if strings.HasPrefix(id, "UC") {
				artists = append(artists, provider.Artist{ID: id, Name: name})
			}
			continue
		}
		if !strings.HasPrefix(id, "MPRE") {
			continue
		}
		a := provider.Album{ID: id, Title: name}
		e := musicEntry(raw, false)
		a.ArtworkURL = ytdlp.BestThumb(e.Thumbnails)
		for _, run := range columnRuns(raw, 1) {
			text := runText(run)
			if strings.HasPrefix(runBrowseID(run), "UC") {
				if a.Artist != "" {
					a.Artist += ", "
				}
				a.Artist += text
			} else if len(text) == 4 {
				if _, e := strconv.Atoi(text); e == nil {
					a.ReleaseDate = text
				}
			}
		}
		albums = append(albums, a)
	}
	return albums, artists, nil
}

func searchDuration(s string) time.Duration {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	total := 0
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return 0
		}
		total = total*60 + n
	}
	return time.Duration(total) * time.Second
}
