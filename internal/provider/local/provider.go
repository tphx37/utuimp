// Package local provides a Provider that serves tracks from a local music
// directory. It scans recursively for MP3, FLAC, M4A (plus OGG on Linux, and
// WAV/AIFF on Linux and macOS) and reads their metadata using the dhowden/tag
// library. No network or credentials are required.

package local

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/dhowden/tag"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/localstore"
)

// Provider scans a local directory and exposes tracks via the provider interface.
type Provider struct {
	mu     sync.RWMutex
	dirs   []string
	dir    string
	tracks []provider.Track

	// skipped counts the files the last scan ignored for an unsupported
	// extension, keyed by that extension ("" for a file that has none).
	// unreadable counts files with a supported extension that would not open.
	skipped    map[string]int
	unreadable int

	// store keeps the playlists made here (nil: none can be made).
	store *localstore.Store
}

// SetStore keeps the playlists made in utuimp in st.
func (p *Provider) SetStore(st *localstore.Store) { p.store = st }

// New creates a Provider and performs the initial directory scan.
func New(dir string) (*Provider, error) { return NewMulti([]string{dir}) }

// NewMulti scans all configured roots, ignoring duplicate files in overlapping roots.
func NewMulti(dirs []string) (*Provider, error) {
	p := &Provider{}
	if _, err := p.Refresh(context.Background(), dirs); err != nil {
		return nil, err
	}
	return p, nil
}
func (p *Provider) Name() string          { return "Local" }
func (p *Provider) IsAuthenticated() bool { return true }

// Refresh builds a replacement index off-thread and publishes it only after all roots succeed.
func (p *Provider) Refresh(ctx context.Context, dirs []string) ([]provider.Track, error) {
	next := &Provider{dirs: append([]string(nil), dirs...), skipped: map[string]int{}}
	if len(dirs) == 0 {
		return nil, errors.New("no music folders configured")
	}
	next.dir = strings.Join(dirs, ", ")
	seen := map[string]bool{}
	for _, root := range dirs {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		err = filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err != nil {
				if os.IsPermission(err) && path != abs {
					return nil
				}
				return err
			}
			if info.IsDir() {
				return nil
			}
			if seen[path] {
				return nil
			}
			seen[path] = true
			ext := strings.ToLower(filepath.Ext(path))
			if lyricsExts[ext] {
				return nil
			}
			if !supportedExts[ext] {
				next.skipped[ext]++
				return nil
			}
			t, err := trackFromFile(path)
			if err != nil {
				next.unreadable++
				return nil
			}
			next.tracks = append(next.tracks, t)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", root, err)
		}
	}
	p.mu.Lock()
	p.dirs, p.dir, p.tracks, p.skipped, p.unreadable = next.dirs, next.dir, next.tracks, next.skipped, next.unreadable
	p.mu.Unlock()
	return append([]provider.Track(nil), next.tracks...), nil
}

// ScanNotice reports what the last scan left out, or "" when every file in the
// directory was indexed. An empty library reaches the TUI looking the same
// whether the directory holds nothing, holds only formats this platform cannot
// play, or holds files that would not open, and only the format case has a
// remedy the user can act on. The supported set is not documented anywhere
// else a user would look, so the message names it.
func (p *Provider) ScanNotice() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	skipped := 0
	for _, n := range p.skipped {
		skipped += n
	}
	if skipped == 0 && p.unreadable == 0 && len(p.tracks) > 0 {
		return ""
	}

	dir := shortPath(p.dir)
	head := "no playable tracks in " + dir
	if len(p.tracks) > 0 {
		head = fmt.Sprintf("indexed %d of %d files in %s",
			len(p.tracks), len(p.tracks)+skipped+p.unreadable, dir)
	}

	var parts []string
	if skipped > 0 {
		// The playable set comes before the detail: a status bar truncates
		// from the right, and it is the half the user can act on.
		parts = append(parts,
			platformName+" plays "+supportedExtList(),
			fmt.Sprintf("skipped %s (%s)", fileCount(skipped), skippedExtList(p.skipped)))
	}
	if p.unreadable > 0 {
		parts = append(parts, fileCount(p.unreadable)+" could not be read")
	}
	if len(parts) == 0 {
		return head + ": the directory is empty"
	}
	return head + ": " + strings.Join(parts, "; ")
}

// fileCount renders a count of files with its plural.
func fileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

// skippedExtList names the skipped extensions, commonest first, capped so the
// line stays inside a status bar. A music directory routinely holds artwork,
// cue sheets and logs beside the audio, so the list can be long and the tail
// of it is never the answer.
func skippedExtList(counts map[string]int) string {
	exts := slices.Sorted(maps.Keys(counts))
	slices.SortStableFunc(exts, func(a, b string) int { return cmp.Compare(counts[b], counts[a]) })

	const named = 3
	parts := make([]string, 0, named+1)
	for i, ext := range exts {
		if i == named {
			parts = append(parts, fmt.Sprintf("+%d more", len(exts)-named))
			break
		}
		name := ext
		if name == "" {
			name = "no extension"
		}
		parts = append(parts, fmt.Sprintf("%d %s", counts[ext], name))
	}
	return strings.Join(parts, ", ")
}

// shortPath swaps the home directory for ~. A music directory is usually
// under it, and the path is the one part of the line the user already knows.
func shortPath(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if strings.HasPrefix(dir, home+string(filepath.Separator)) {
		return "~" + dir[len(home):]
	}
	return dir
}

// supportedExtList names this platform's playable extensions.
func supportedExtList() string {
	return strings.Join(slices.Sorted(maps.Keys(supportedExts)), ", ")
}

// trackFromFile reads metadata from an audio file and returns a Track.
func trackFromFile(path string) (provider.Track, error) {
	f, err := os.Open(path) //nolint:gosec // the path comes from user-configured music dir
	if err != nil {
		return provider.Track{}, err
	}
	defer func() { _ = f.Close() }()

	m, err := tag.ReadFrom(f)

	title := filepath.Base(path)
	artist := "Unknown Artist"
	album := "Unknown Album"
	var genres []string
	cover := ""

	if err == nil {
		if pic := m.Picture(); pic != nil && len(pic.Data) > 0 {
			// The cover stays in the file: the URL only says where it is
			// (the TUI's artwork loader reads it from the tags when needed).
			cover = (&url.URL{Scheme: "utuimp-cover", Path: path}).String()
		}
		if m.Title() != "" {
			title = m.Title()
		}
		if m.Artist() != "" {
			artist = m.Artist()
		}
		if m.Album() != "" {
			album = m.Album()
		}
		if m.Genre() != "" {
			genres = []string{m.Genre()}
		}
	}

	return provider.Track{
		ID:         fmt.Sprintf("local:%s", path),
		Title:      provider.Clean(title),
		Artist:     provider.Clean(artist),
		Album:      provider.Clean(album),
		Genres:     provider.CleanAll(genres),
		ArtworkURL: cover,
		// Without a length the time shows 0:00 and the progress line
		// can't move.
		Duration: fileDuration(path),
	}, nil
}

func (p *Provider) GetLibraryTracks(_ context.Context) ([]provider.Track, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]provider.Track, len(p.tracks))
	copy(out, p.tracks)
	return out, nil
}

func (p *Provider) Search(_ context.Context, query string) (*provider.SearchResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	q := strings.ToLower(query)
	var tracks []provider.Track
	var albums []provider.Album
	seen := map[string]bool{}

	for _, t := range p.tracks {
		if strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Artist), q) ||
			strings.Contains(strings.ToLower(t.Album), q) {
			tracks = append(tracks, t)
			if !seen[t.Album] {
				seen[t.Album] = true
				albums = append(albums, provider.Album{
					ID:     "local-album:" + t.Album,
					Title:  t.Album,
					Artist: t.Artist,
				})
			}
		}
	}
	return &provider.SearchResult{Tracks: tracks, Albums: albums}, nil
}

func (p *Provider) GetLibraryPlaylists(_ context.Context) ([]provider.Playlist, error) {
	if p.store == nil {
		return nil, nil
	}
	var out []provider.Playlist
	for _, pl := range p.store.Playlists() {
		out = append(out, provider.Playlist{ID: pl.ID, Name: pl.Name, TrackCount: len(pl.Tracks)})
	}
	return out, nil
}

func (p *Provider) GetPlaylistTracks(_ context.Context, id string) ([]provider.Track, error) {
	if p.store == nil {
		return nil, nil
	}
	pl, ok := p.store.Playlist(id)
	if !ok {
		return nil, errors.New("no such playlist")
	}
	return pl.Tracks, nil
}

// track finds a scanned file by ID.
func (p *Provider) track(id string) (provider.Track, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	i := slices.IndexFunc(p.tracks, func(t provider.Track) bool { return t.ID == id })
	if i < 0 {
		return provider.Track{}, false
	}
	return p.tracks[i], true
}

// lyricsExts are the files beside a song that hold its lyrics: an .lrc
// (synced, or plain lines) or a .txt of the same name.
var lyricsExts = map[string]bool{".lrc": true, ".txt": true}

// TrackLyrics reads the lyrics kept beside a song: an .lrc of the same name
// when synced lyrics are asked for, else a .txt, else the .lrc as it is.
func (p *Provider) TrackLyrics(_ context.Context, trackID string, synced bool) (string, error) {
	if _, ok := p.track(trackID); !ok {
		return "", errors.New("no such file")
	}
	path := strings.TrimPrefix(trackID, "local:")
	base := strings.TrimSuffix(path, filepath.Ext(path))
	exts := []string{".lrc"}
	if !synced {
		exts = []string{".txt", ".lrc"}
	}
	for _, ext := range exts {
		b, err := os.ReadFile(base + ext) //nolint:gosec // beside a song in the user's music dir
		if err == nil && strings.TrimSpace(string(b)) != "" {
			return string(b), nil
		}
	}
	return "", errors.New("no lyrics beside the song")
}

func (p *Provider) GetAlbumTracks(_ context.Context, albumID string) ([]provider.Track, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	albumID = strings.TrimPrefix(albumID, "local-album:")
	var out []provider.Track
	for _, t := range p.tracks {
		if t.Album == albumID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (p *Provider) GetLibraryAlbumTracks(ctx context.Context, albumID string) ([]provider.Track, error) {
	return p.GetAlbumTracks(ctx, albumID)
}

func (p *Provider) GetCatalogPlaylistTracks(_ context.Context, _ string) ([]provider.Track, error) {
	return nil, nil
}

// GetArtistAlbums returns nothing: local search returns no artists to open.
func (p *Provider) GetArtistAlbums(_ context.Context, _ string) ([]provider.Album, error) {
	return nil, nil
}

func (p *Provider) CreatePlaylist(_ context.Context, name string, ids []string) (provider.Playlist, error) {
	if p.store == nil {
		return provider.Playlist{}, errors.New("playlists can't be saved here")
	}
	var ts []provider.Track
	for _, id := range ids {
		if t, ok := p.track(id); ok {
			ts = append(ts, t)
		}
	}
	pl, err := p.store.CreatePlaylist(name, ts)
	return provider.Playlist{ID: pl.ID, Name: pl.Name, TrackCount: len(pl.Tracks)}, err
}

func (p *Provider) LoveSong(_ context.Context, _ string, _ bool) error      { return nil }
func (p *Provider) GetSongRating(_ context.Context, _ string) (bool, error) { return false, nil }

func (p *Provider) AddToPlaylist(_ context.Context, playlistID, trackID string) error {
	if p.store == nil {
		return errors.New("playlists can't be saved here")
	}
	t, ok := p.track(trackID)
	if !ok {
		return errors.New("no such file")
	}
	return p.store.AddToPlaylist(playlistID, t)
}

func (p *Provider) RemoveFromPlaylist(_ context.Context, playlistID string, index int, t provider.Track) error {
	if p.store == nil {
		return errors.New("playlists can't be saved here")
	}
	return p.store.RemoveFromPlaylist(playlistID, index, t.ID)
}

func (p *Provider) RenamePlaylist(_ context.Context, playlistID, name string) error {
	if p.store == nil {
		return errors.New("playlists can't be saved here")
	}
	return p.store.RenamePlaylist(playlistID, name)
}

func (p *Provider) DeletePlaylist(_ context.Context, playlistID string) error {
	if p.store == nil {
		return errors.New("playlists can't be saved here")
	}
	return p.store.DeletePlaylist(playlistID)
}

var _ provider.PlaylistEditor = (*Provider)(nil)

func (p *Provider) GetRecommendations(_ context.Context) ([]provider.RecommendationGroup, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	// Group tracks by genre using their metadata tags.
	// Tracks with no genre tag are skipped.
	byGenre := map[string][]provider.Track{}
	for _, t := range p.tracks {
		for _, g := range t.Genres {
			if g != "" {
				byGenre[g] = append(byGenre[g], t)
			}
		}
	}

	var groups []provider.RecommendationGroup
	for genre, tracks := range byGenre {
		if len(tracks) < 2 {
			continue
		}
		items := make([]provider.RecommendationItem, 0, len(tracks))
		for _, t := range tracks {
			items = append(items, provider.RecommendationItem{
				ID:       t.ID,
				Kind:     "track",
				Title:    t.Title,
				Subtitle: t.Artist,
			})
		}
		groups = append(groups, provider.RecommendationGroup{
			Title: genre,
			Items: items,
		})
	}
	return groups, nil
}

func (p *Provider) GetStationTracks(_ context.Context, _ string) ([]provider.Track, error) {
	return nil, nil
}
