package ytmusic

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/localstore"
)

// fakeRun answers yt-dlp calls by the URL (the last argument).
func fakeRun(t *testing.T, pages map[string]string) Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		u := args[len(args)-1]
		for k, v := range pages {
			if strings.Contains(u, k) {
				return []byte(v), nil
			}
		}
		t.Errorf("unexpected yt-dlp call %v", args)
		return []byte(`{}`), nil
	}
}

func testProvider(t *testing.T, pages map[string]string) *Provider {
	st, err := localstore.Open(filepath.Join(t.TempDir(), "lib.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := New("yt-dlp", "ffmpeg", "", st)
	p.run = fakeRun(t, pages)
	return p
}

func TestSearchResolvesNames(t *testing.T) {
	p := testProvider(t, map[string]string{
		"#songs":   `{"entries":[{"_type":"url","id":"v1","title":"Get Lucky"}]}`,
		"#albums":  `{"entries":[{"_type":"url","id":"MPREb_a"}]}`,
		"#artists": `{"entries":[{"_type":"url","id":"UCx"}]}`,
		"browse/MPREb_a": `{"id":"OLAK_a","title":"Album - Random Access Memories",
			"entries":[{"id":"v1","title":"Give Life Back","channel":"Daft Punk"}]}`,
		"browse/UCx": `{"id":"UCx","title":"Daft Punk - Topic","channel":"Daft Punk - Topic"}`,
	})
	r, err := p.Search(context.Background(), "daft punk")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tracks) != 1 || r.Tracks[0].Title != "Get Lucky" || !strings.Contains(r.Tracks[0].ArtworkURL, "v1") {
		t.Errorf("tracks = %+v", r.Tracks)
	}
	if len(r.Albums) != 1 || r.Albums[0].ID != "OLAK_a" || r.Albums[0].Title != "Random Access Memories" || r.Albums[0].Artist != "Daft Punk" {
		t.Errorf("albums = %+v", r.Albums)
	}
	if len(r.Artists) != 1 || r.Artists[0].Name != "Daft Punk" {
		t.Errorf("artists = %+v", r.Artists)
	}
}

func TestStationDropsSeedAndFavorites(t *testing.T) {
	p := testProvider(t, map[string]string{
		"list=RDAMVMv1": `{"entries":[{"id":"v1","title":"Seed"},{"id":"v2","title":"Next","channel":"Band","duration":200}]}`,
	})
	ts, err := p.GetStationTracks(context.Background(), "v1")
	if err != nil || len(ts) != 1 || ts[0].ID != "v2" || ts[0].Artist != "Band" || ts[0].Duration != 200*time.Second {
		t.Fatalf("station = %+v, %v", ts, err)
	}
	ctx := context.Background()
	if err := p.LoveSong(ctx, "v2", true); err != nil {
		t.Fatal(err)
	}
	if ok, _ := p.GetSongRating(ctx, "v2"); !ok {
		t.Error("v2 should be a favourite")
	}
	fav, _ := p.GetPlaylistTracks(ctx, provider.FavoritesPlaylistID)
	if len(fav) != 1 || fav[0].Title != "Next" {
		t.Errorf("favourites = %+v", fav)
	}
	groups, _ := p.GetRecommendations(ctx)
	if len(groups[0].Items) != 1 || groups[0].Items[0].ID != "RDAMVMv2" {
		t.Errorf("feed = %+v", groups)
	}
	pl, err := p.CreatePlaylist(ctx, "Mine", []string{"v2"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := p.GetPlaylistTracks(ctx, pl.ID); len(got) != 1 {
		t.Errorf("playlist = %+v", got)
	}
}

func TestPickFormat(t *testing.T) {
	fs := []format{
		{FormatID: "18", Acodec: "mp4a.40.2", Vcodec: "avc1"},
		{FormatID: "140", Acodec: "mp4a.40.2", Vcodec: "none", Abr: 129},
		{FormatID: "251", Acodec: "opus", Vcodec: "none", Abr: 160},
	}
	if id, cp, _ := pickFormat(fs); id != "140" || !cp {
		t.Errorf("got %s copy=%v, want 140 copied", id, cp)
	}
	if id, cp, _ := pickFormat(fs[2:]); id != "251" || cp {
		t.Errorf("got %s copy=%v, want 251 re-encoded", id, cp)
	}
}

func TestBotCheckError(t *testing.T) {
	err := ytdlpError("ERROR: [youtube] x: Sign in to confirm you’re not a bot.", nil)
	if err != ErrBotCheck {
		t.Errorf("err = %v", err)
	}
}

// The yt-dlp | ffmpeg pipeline turns an M4A into ADTS AAC. yt-dlp is
// replaced by a script that prints a file, so no network is needed.
func TestStreamPipelineMakesADTS(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	m4a := filepath.Join(dir, "a.m4a")
	if out, err := exec.Command(ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "sine=d=1",
		"-c:a", "aac", m4a).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg can't encode AAC: %v %s", err, out)
	}
	fake := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec cat "+m4a+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := streamer{ytdlp: fake, ffmpeg: ffmpeg}
	rc, err := s.open(context.Background(), []byte(`{}`), "140", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 100 || b[0] != 0xFF || b[1]&0xF0 != 0xF0 {
		t.Fatalf("not ADTS: %d bytes, % x", len(b), b[:min(4, len(b))])
	}
}

func TestStreamPipelineReportsFailure(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	fake := filepath.Join(t.TempDir(), "yt-dlp")
	_ = os.WriteFile(fake, []byte("#!/bin/sh\necho 'ERROR: [youtube] x: Video unavailable' >&2\nexit 1\n"), 0o755)
	rc, err := streamer{ytdlp: fake, ffmpeg: ffmpeg}.open(context.Background(), []byte(`{}`), "140", true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.ReadAll(rc); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("err = %v, want yt-dlp's message", err)
	}
}
