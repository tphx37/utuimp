package ytmusic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMusicAccountPaginationAndAuthentication(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.Count(r.Header.Get("Cookie"), "SAPISID=secret") != 1 {
			t.Error("duplicate cookies must be removed")
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "SAPISIDHASH ") || !strings.Contains(r.Header.Get("Cookie"), "SAPISID=secret") {
			t.Error("missing account authentication")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"contents":[{"musicResponsiveListItemRenderer":{"flexColumns":[{"runs":[{"text":"Song"}]},{"runs":[{"text":"Artist"}]}],"watchEndpoint":{"videoId":"v1"}}},{"continuationCommand":{"token":"next"}}]}`))
			return
		}
		if body["continuation"] != "next" {
			t.Errorf("continuation: %v", body)
		}
		_, _ = w.Write([]byte(`{"contents":[{"musicResponsiveListItemRenderer":{"flexColumns":[{"runs":[{"text":"Second"}]}],"watchEndpoint":{"videoId":"v2"}}}]}`))
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(path, []byte("#HttpOnly_.youtube.com\tTRUE\t/\tTRUE\t0\tSAPISID\tsecret\n.youtube.com\tTRUE\t/\tTRUE\t0\tSAPISID\tsecret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := newMusicAccount(path)
	a.endpoint = srv.URL
	a.client = srv.Client()
	entries, err := a.browse(context.Background(), "VLLM")
	if err != nil || len(entries) != 2 || entries[0].ID != "v1" || entries[0].Title != "Song" || entries[1].ID != "v2" {
		t.Fatalf("entries: %+v, %v", entries, err)
	}
}

func TestMusicEntryReadsPlaylistItemData(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(`{"playlistItemData":{"videoId":"track123"},"flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Track"}]}}},{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Artist"}]}}}]}`), &raw); err != nil {
		t.Fatal(err)
	}
	entry := musicEntry(raw, false)
	if entry.ID != "track123" || entry.Title != "Track" || entry.Artist != "Artist" {
		t.Fatalf("entry: %+v", entry)
	}
}

func TestMusicEntryPreservesCoverThumbnail(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(`{"playlistItemData":{"videoId":"track123"},"thumbnail":{"musicThumbnailRenderer":{"thumbnail":{"thumbnails":[{"url":"https://example.com/art.jpg","width":300,"height":300}]}}}}`), &raw); err != nil {
		t.Fatal(err)
	}
	entry := musicEntry(raw, false)
	if len(entry.Thumbnails) != 1 || entry.Thumbnails[0].URL != "https://example.com/art.jpg" {
		t.Fatalf("thumbnails: %+v", entry.Thumbnails)
	}
	p := testProvider(t, nil)
	if track := p.toTrack(entry); track.ArtworkURL != "https://example.com/art.jpg" {
		t.Fatalf("artwork: %q", track.ArtworkURL)
	}
}
