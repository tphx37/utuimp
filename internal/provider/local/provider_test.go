package local_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tphx37/utuimp/internal/provider/localstore"

	"github.com/tphx37/utuimp/internal/provider/local"
)

// newTestProvider creates a Provider pointed at a temp directory
// containing dummy audio files with predictable names.
func newTestProvider(t *testing.T) *local.Provider {
	t.Helper()
	dir := t.TempDir()

	// Create dummy files — dhowden/tag will fail to read metadata
	// from empty files, so the provider falls back to filename-based
	// title and "Unknown Artist" / "Unknown Album". That's fine for
	// testing scan and search logic.
	files := []string{
		"Artist1 - Album1 - Track1.mp3",
		"Artist1 - Album1 - Track2.flac",
		"Artist2 - Album2 - Track1.m4a",
		"Artist2 - Album2 - Track2.ogg",
		"Artist3 - Album3 - Track1.wav",
		"Artist3 - Album3 - Track2.aiff",
		"Artist3 - Album3 - Track3.aif",
		"ignored.txt",
		"ignored.pdf",
	}
	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
			t.Fatalf("creating test file %s: %v", f, err)
		}
	}

	p, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	return p
}

// --- Name ---

func TestProvider_Name(t *testing.T) {
	p := newTestProvider(t)
	if p.Name() != "Local" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Local")
	}
}

// --- IsAuthenticated ---

func TestProvider_IsAuthenticated(t *testing.T) {
	p := newTestProvider(t)
	if !p.IsAuthenticated() {
		t.Error("IsAuthenticated() = false, want true")
	}
}

// --- GetLibraryTracks ---

func TestProvider_GetLibraryTracks_OnlyAudioFiles(t *testing.T) {
	p := newTestProvider(t)
	tracks, err := p.GetLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryTracks: %v", err)
	}

	if len(tracks) != wantAudioFileCount {
		t.Errorf("GetLibraryTracks = %d tracks, want %d", len(tracks), wantAudioFileCount)
	}
}

func TestProvider_GetLibraryTracks_ReturnsCopy(t *testing.T) {
	p := newTestProvider(t)
	tracks, _ := p.GetLibraryTracks(context.Background())
	if len(tracks) > 0 {
		tracks[0].Title = "mutated"
	}
	tracks2, _ := p.GetLibraryTracks(context.Background())
	if len(tracks2) > 0 && tracks2[0].Title == "mutated" {
		t.Error("GetLibraryTracks returned a shared slice (not a copy)")
	}
}

func TestProvider_GetLibraryTracks_IDsAreLocalPrefixed(t *testing.T) {
	p := newTestProvider(t)
	tracks, _ := p.GetLibraryTracks(context.Background())
	for _, tr := range tracks {
		if !strings.HasPrefix(tr.ID, "local:") {
			t.Errorf("track ID %q does not start with 'local:'", tr.ID)
		}
	}
}

// --- Search ---

func TestProvider_Search_NoMatch(t *testing.T) {
	p := newTestProvider(t)
	result, err := p.Search(context.Background(), "zzznomatchzzz")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Tracks) != 0 {
		t.Errorf("Search(zzznomatchzzz) = %d tracks, want 0", len(result.Tracks))
	}
}

func TestProvider_Search_CaseInsensitive(t *testing.T) {
	p := newTestProvider(t)
	upper, _ := p.Search(context.Background(), "TRACK1")
	lower, _ := p.Search(context.Background(), "track1")
	if len(upper.Tracks) != len(lower.Tracks) {
		t.Errorf("case-insensitive mismatch: upper=%d lower=%d", len(upper.Tracks), len(lower.Tracks))
	}
}

func TestProvider_Search_DeduplicatesAlbums(t *testing.T) {
	p := newTestProvider(t)
	result, _ := p.Search(context.Background(), "track")
	seen := map[string]int{}
	for _, a := range result.Albums {
		seen[a.Title]++
		if seen[a.Title] > 1 {
			t.Errorf("album %q appears %d times in results", a.Title, seen[a.Title])
		}
	}
}

// --- GetLibraryPlaylists ---

func TestProvider_GetLibraryPlaylists_ReturnsNil(t *testing.T) {
	p := newTestProvider(t)
	playlists, err := p.GetLibraryPlaylists(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryPlaylists: %v", err)
	}
	if playlists != nil {
		t.Errorf("GetLibraryPlaylists = %v, want nil", playlists)
	}
}

// --- GetAlbumTracks ---

func TestProvider_GetAlbumTracks_UnknownAlbum(t *testing.T) {
	p := newTestProvider(t)
	tracks, err := p.GetAlbumTracks(context.Background(), "local-album:nonexistent")
	if err != nil {
		t.Fatalf("GetAlbumTracks: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("GetAlbumTracks(nonexistent) = %d tracks, want 0", len(tracks))
	}
}

// --- New with invalid directory ---

func TestNew_InvalidDir(t *testing.T) {
	_, err := local.New("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("New() with invalid dir should return error, got nil")
	}
}

// --- CreatePlaylist ---

func TestProvider_CreatePlaylist_NeedsAStore(t *testing.T) {
	p := newTestProvider(t)
	if _, err := p.CreatePlaylist(context.Background(), "test", nil); err == nil {
		t.Error("without a store a playlist can't be saved, and must say so")
	}
}

// Playlists of local files are kept in the store: made, filled, edited.
func TestProvider_PlaylistsInStore(t *testing.T) {
	ctx := context.Background()
	p := newTestProvider(t)
	st, err := localstore.Open(filepath.Join(t.TempDir(), "pl.json"))
	if err != nil {
		t.Fatal(err)
	}
	p.SetStore(st)
	tracks, _ := p.GetLibraryTracks(ctx)
	if len(tracks) == 0 {
		t.Skip("no test tracks")
	}
	pl, err := p.CreatePlaylist(ctx, "Mine", []string{tracks[0].ID})
	if err != nil || pl.ID == "" {
		t.Fatalf("create: %+v %v", pl, err)
	}
	if err := p.AddToPlaylist(ctx, pl.ID, tracks[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ := p.GetPlaylistTracks(ctx, pl.ID)
	if len(got) != 2 {
		t.Fatalf("tracks = %d, want 2", len(got))
	}
	if err := p.RemoveFromPlaylist(ctx, pl.ID, 1, got[1]); err != nil {
		t.Fatal(err)
	}
	if err := p.RenamePlaylist(ctx, pl.ID, "Ours"); err != nil {
		t.Fatal(err)
	}
	pls, _ := p.GetLibraryPlaylists(ctx)
	if len(pls) != 1 || pls[0].Name != "Ours" || pls[0].TrackCount != 1 {
		t.Fatalf("playlists = %+v", pls)
	}
	if err := p.DeletePlaylist(ctx, pl.ID); err != nil {
		t.Fatal(err)
	}
	if pls, _ := p.GetLibraryPlaylists(ctx); len(pls) != 0 {
		t.Fatalf("after delete: %+v", pls)
	}
}

// --- LoveSong ---

func TestProvider_LoveSong_NoError(t *testing.T) {
	p := newTestProvider(t)
	if err := p.LoveSong(context.Background(), "local:test", true); err != nil {
		t.Errorf("LoveSong: %v", err)
	}
}

// --- GetSongRating ---

func TestProvider_GetSongRating_ReturnsFalse(t *testing.T) {
	p := newTestProvider(t)
	loved, err := p.GetSongRating(context.Background(), "local:test")
	if err != nil {
		t.Fatalf("GetSongRating: %v", err)
	}
	if loved {
		t.Error("GetSongRating() = true, want false")
	}
}

// TestTrackLyrics reads a song's lyrics from the files beside it, and does
// not count them as files it skipped.
func TestTrackLyrics(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.mp3": "",
		"a.lrc": "[00:01.00]first line\n[00:03.00]second line\n",
		"b.mp3": "",
		"b.txt": "plain words\n",
		"c.mp3": "",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := p.ScanNotice(); n != "" {
		t.Errorf("lyrics files were counted as skipped: %q", n)
	}
	ctx := context.Background()
	id := func(name string) string { return "local:" + filepath.Join(dir, name) }

	if got, err := p.TrackLyrics(ctx, id("a.mp3"), true); err != nil || !strings.Contains(got, "[00:03.00]second line") {
		t.Errorf("synced a: %q, %v", got, err)
	}
	if _, err := p.TrackLyrics(ctx, id("b.mp3"), true); err == nil {
		t.Error("synced b: a .txt is not synced lyrics")
	}
	if got, err := p.TrackLyrics(ctx, id("b.mp3"), false); err != nil || strings.TrimSpace(got) != "plain words" {
		t.Errorf("plain b: %q, %v", got, err)
	}
	if _, err := p.TrackLyrics(ctx, id("c.mp3"), false); err == nil {
		t.Error("c has no lyrics beside it")
	}
	if _, err := p.TrackLyrics(ctx, "local:"+filepath.Join(dir, "..", "x.mp3"), false); err == nil {
		t.Error("a song not in the library has no lyrics here")
	}
}

func TestMultipleRootsRefreshAndFailurePreservesLibrary(t *testing.T) {
	ctx := context.Background()
	first, second := t.TempDir(), t.TempDir()
	nested := filepath.Join(first, "nested")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(dir, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(nested, "one.mp3")
	write(second, "two.mp3")
	p, err := local.NewMulti([]string{first, nested, second})
	if err != nil {
		t.Fatal(err)
	}
	tracks, _ := p.GetLibraryTracks(ctx)
	if len(tracks) != 2 {
		t.Fatalf("overlapping roots yielded %d tracks", len(tracks))
	}
	write(second, "three.mp3")
	tracks, err = p.Refresh(ctx, []string{first, second})
	if err != nil || len(tracks) != 3 {
		t.Fatalf("refresh: %d, %v", len(tracks), err)
	}
	if _, err = p.Refresh(ctx, []string{first, filepath.Join(second, "missing")}); err == nil {
		t.Fatal("missing root should fail")
	}
	tracks, _ = p.GetLibraryTracks(ctx)
	if len(tracks) != 3 {
		t.Fatalf("failed scan replaced index: %d", len(tracks))
	}
}

func TestRefreshCanRunWhileLibraryIsRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "song.mp3"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	p, err := local.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 20 {
			_, _ = p.Search(context.Background(), "song")
			_, _ = p.GetLibraryTracks(context.Background())
			_ = p.ScanNotice()
		}
	}()
	for range 10 {
		if _, err = p.Refresh(context.Background(), []string{dir}); err != nil {
			t.Fatal(err)
		}
	}
	<-done
}
