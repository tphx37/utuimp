package soundcloud

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/localstore"
	"github.com/tphx37/utuimp/internal/provider/ytdlp"
)

// Public plays SoundCloud without an API app, through yt-dlp, the way
// YouTube Music is played. Tracks are identified by their permalink path
// ("artist/track"); favourites and playlists are kept in a local file.
type Public struct {
	run    ytdlp.Runner
	stream ytdlp.Streamer
	store  *localstore.Store

	mu    sync.Mutex
	cache map[string]provider.Track
	users map[string]provider.Artist // track ID → its uploader
}

// NewPublic returns the yt-dlp backed provider.
func NewPublic(ytdlpPath, ffmpeg string, store *localstore.Store) *Public {
	return &Public{
		run:    ytdlp.ExecRunner(ytdlpPath, nil),
		stream: ytdlp.Streamer{YtDlp: ytdlpPath, FFmpeg: ffmpeg},
		store:  store,
		cache:  map[string]provider.Track{}, users: map[string]provider.Artist{},
	}
}

func (p *Public) Name() string          { return "SoundCloud" }
func (p *Public) IsAuthenticated() bool { return true }

const (
	pubRelated    = "utuimp:related:"    // + track path: tracks like it
	pubUserTracks = "utuimp:usertracks:" // + user path: everything they posted
	// previewLength is how long SoundCloud's previews of Go+ tracks are;
	// they are left out, since only the snippet would play.
	previewLength = 30.0
)

func pageURL(path string) string { return "https://soundcloud.com/" + strings.Trim(path, "/") }

// pathOf returns the permalink path of a soundcloud.com URL ("" otherwise).
func pathOf(u string) string {
	pu, err := url.Parse(u)
	if err != nil || (pu.Host != "soundcloud.com" && pu.Host != "www.soundcloud.com" && pu.Host != "m.soundcloud.com") {
		return ""
	}
	return strings.Trim(pu.Path, "/")
}

func entryPath(e ytdlp.Entry) string {
	if p := pathOf(e.WebpageURL); p != "" {
		return p
	}
	return pathOf(e.URL)
}

// largeArt swaps SoundCloud's full-size artwork for the 500px one.
func largeArt(u string) string {
	for _, from := range []string{"-original.", "-large.", "-t300x300."} {
		if strings.Contains(u, from) {
			return strings.Replace(u, from, "-t500x500.", 1)
		}
	}
	return u
}

// toTrack converts an entry and caches it; ok is false for entries that
// aren't playable tracks.
func (p *Public) toTrack(e ytdlp.Entry) (provider.Track, bool) {
	id := entryPath(e)
	if id == "" || strings.Contains(id, "/sets/") {
		return provider.Track{}, false
	}
	t := provider.Track{
		ID: id, CatalogID: id,
		Title:      provider.Clean(e.Title),
		Artist:     provider.Clean(e.Uploader),
		Duration:   time.Duration(e.Duration * float64(time.Second)),
		ArtworkURL: largeArt(e.Thumbnail),
		Album:      provider.Clean(e.Album),
	}
	if t.ArtworkURL == "" {
		t.ArtworkURL = largeArt(ytdlp.BestThumb(e.Thumbnails))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if old, ok := p.cache[id]; ok {
		if t.Title == "" {
			t.Title = old.Title
		}
		if t.Artist == "" {
			t.Artist = old.Artist
		}
		if t.Duration == 0 {
			t.Duration = old.Duration
		}
		if t.ArtworkURL == "" {
			t.ArtworkURL = old.ArtworkURL
		}
	}
	p.cache[id] = t
	if up := pathOf(e.UploaderURL); up != "" && e.Uploader != "" {
		p.users[id] = provider.Artist{ID: up, Name: provider.Clean(e.Uploader)}
	}
	return t, true
}

// toTracks converts entries, resolving the ones listed without a title (set
// entries), and drops Go+ previews.
func (p *Public) toTracks(ctx context.Context, es []ytdlp.Entry) []provider.Track {
	full := make([]ytdlp.Entry, len(es))
	copy(full, es)
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, e := range es {
		if e.Title != "" {
			continue
		}
		path := entryPath(e)
		if path == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if r, err := p.lookupEntry(ctx, path); err == nil {
				full[i] = *r
			}
		}()
	}
	wg.Wait()
	out := make([]provider.Track, 0, len(full))
	for _, e := range full {
		if e.Duration == previewLength {
			continue
		}
		if t, ok := p.toTrack(e); ok && t.Title != "" {
			out = append(out, t)
		}
	}
	return out
}

func (p *Public) lookupEntry(ctx context.Context, path string) (*ytdlp.Entry, error) {
	out, err := p.run(ctx, "-J", "--skip-download", "--no-playlist", pageURL(path))
	if err != nil {
		return nil, err
	}
	var e ytdlp.Entry
	if err := json.Unmarshal(out, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// listing returns the tracks of a SoundCloud page (a set, a user's tracks,
// related tracks).
func (p *Public) listing(ctx context.Context, u string, limit int) ([]provider.Track, error) {
	d, err := ytdlp.Flat(ctx, p.run, u, limit)
	if err != nil {
		return nil, err
	}
	ts := p.toTracks(ctx, d.Entries)
	if len(ts) == 0 && len(d.Entries) > 0 {
		return nil, errors.New(i18n.T("only 30-second previews here (SoundCloud Go+ tracks)"))
	}
	return ts, nil
}

func (p *Public) Search(ctx context.Context, query string) (*provider.SearchResult, error) {
	d, err := ytdlp.Flat(ctx, p.run, "scsearch20:"+query, 0)
	if err != nil {
		return nil, err
	}
	res := &provider.SearchResult{Tracks: p.toTracks(ctx, d.Entries)}
	seen := map[string]bool{}
	for _, e := range d.Entries {
		up := pathOf(e.UploaderURL)
		if up == "" || e.Uploader == "" || seen[up] || len(res.Artists) == 6 {
			continue
		}
		seen[up] = true
		res.Artists = append(res.Artists, provider.Artist{ID: up, Name: provider.Clean(e.Uploader)})
	}
	return res, nil
}

func (p *Public) remember(ts []provider.Track) {
	p.mu.Lock()
	for _, t := range ts {
		if _, ok := p.cache[t.ID]; !ok {
			p.cache[t.ID] = t
		}
	}
	p.mu.Unlock()
}

// GetLibraryTracks returns the favourite tracks.
func (p *Public) GetLibraryTracks(context.Context) ([]provider.Track, error) {
	fav := p.store.Favorites()
	p.remember(fav)
	return fav, nil
}

func (p *Public) GetLibraryPlaylists(context.Context) ([]provider.Playlist, error) {
	out := []provider.Playlist{{ID: provider.FavoritesPlaylistID, Name: i18n.T("Favorites"),
		TrackCount: len(p.store.Favorites()), ReadOnly: true}}
	for _, pl := range p.store.Playlists() {
		out = append(out, provider.Playlist{ID: pl.ID, Name: pl.Name, TrackCount: len(pl.Tracks)})
	}
	return out, nil
}

func (p *Public) GetPlaylistTracks(ctx context.Context, id string) ([]provider.Track, error) {
	switch {
	case id == provider.FavoritesPlaylistID:
		return p.GetLibraryTracks(ctx)
	case strings.HasPrefix(id, localstore.PlaylistPrefix):
		pl, ok := p.store.Playlist(id)
		if !ok {
			return nil, errors.New("no such playlist")
		}
		p.remember(pl.Tracks)
		return pl.Tracks, nil
	case strings.HasPrefix(id, pubRelated):
		return p.listing(ctx, pageURL(strings.TrimPrefix(id, pubRelated))+"/recommended", 30)
	case strings.HasPrefix(id, pubUserTracks):
		return p.listing(ctx, pageURL(strings.TrimPrefix(id, pubUserTracks))+"/tracks", 200)
	}
	return p.listing(ctx, pageURL(id), 500)
}

func (p *Public) GetCatalogPlaylistTracks(ctx context.Context, id string) ([]provider.Track, error) {
	return p.GetPlaylistTracks(ctx, id)
}

func (p *Public) GetAlbumTracks(ctx context.Context, id string) ([]provider.Track, error) {
	ts, err := p.GetPlaylistTracks(ctx, id)
	for i := range ts {
		ts[i].TrackNumber = i + 1
	}
	return ts, err
}

func (p *Public) GetLibraryAlbumTracks(ctx context.Context, id string) ([]provider.Track, error) {
	return p.GetAlbumTracks(ctx, id)
}

// GetArtistAlbums lists the user's albums and sets, after an entry with all
// their tracks.
func (p *Public) GetArtistAlbums(ctx context.Context, user string) ([]provider.Album, error) {
	out := []provider.Album{{ID: pubUserTracks + user, Title: i18n.T("All tracks")}}
	seen := map[string]bool{}
	for _, tab := range []string{"/albums", "/sets"} {
		d, err := ytdlp.Flat(ctx, p.run, pageURL(user)+tab, 100)
		if err != nil {
			continue
		}
		for _, e := range d.Entries {
			path := entryPath(e)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, provider.Album{ID: path, Title: provider.Clean(e.Title), Artist: provider.Clean(e.Uploader)})
		}
	}
	return out, nil
}

func (p *Public) CreatePlaylist(ctx context.Context, name string, ids []string) (provider.Playlist, error) {
	ts := make([]provider.Track, 0, len(ids))
	for _, id := range ids {
		t, _ := p.Lookup(ctx, id)
		ts = append(ts, t)
	}
	pl, err := p.store.CreatePlaylist(name, ts)
	return provider.Playlist{ID: pl.ID, Name: pl.Name, TrackCount: len(pl.Tracks)}, err
}

func (p *Public) AddToPlaylist(ctx context.Context, playlistID, trackID string) error {
	if playlistID == provider.FavoritesPlaylistID {
		return p.LoveSong(ctx, trackID, true)
	}
	t, _ := p.Lookup(ctx, trackID)
	return p.store.AddToPlaylist(playlistID, t)
}

func (p *Public) LoveSong(ctx context.Context, id string, loved bool) error {
	t, _ := p.Lookup(ctx, id)
	return p.store.SetFavorite(t, loved)
}

func (p *Public) GetSongRating(_ context.Context, id string) (bool, error) {
	return p.store.IsFavorite(id), nil
}

// GetRecommendations offers tracks like each recent favourite, and the
// saved playlists.
func (p *Public) GetRecommendations(context.Context) ([]provider.RecommendationGroup, error) {
	more := provider.RecommendationGroup{Title: i18n.T("More like your favorites")}
	for _, t := range p.store.Favorites() {
		if len(more.Items) == 12 {
			break
		}
		more.Items = append(more.Items, provider.RecommendationItem{ID: pubRelated + t.ID, Kind: "playlist",
			Title: i18n.T("Like: ") + t.Title, Subtitle: t.Artist})
	}
	if len(more.Items) == 0 {
		more.Title = i18n.T("Press f on tracks you like: similar ones show up here")
	}
	groups := []provider.RecommendationGroup{more}
	if pls := p.store.Playlists(); len(pls) > 0 {
		g := provider.RecommendationGroup{Title: i18n.T("Your playlists")}
		for _, pl := range pls {
			g.Items = append(g.Items, provider.RecommendationItem{ID: pl.ID, Kind: "playlist", Title: pl.Name})
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// GetStationTracks returns SoundCloud's recommendations for the seed.
func (p *Public) GetStationTracks(ctx context.Context, seed string) ([]provider.Track, error) {
	ts, err := p.listing(ctx, pageURL(seed)+"/recommended", 30)
	return slices.DeleteFunc(ts, func(t provider.Track) bool { return t.ID == seed }), err
}

// Lookup returns what is known about a track, asking yt-dlp when nothing is.
func (p *Public) Lookup(ctx context.Context, id string) (provider.Track, error) {
	p.mu.Lock()
	t, ok := p.cache[id]
	p.mu.Unlock()
	if ok {
		return t, nil
	}
	e, err := p.lookupEntry(ctx, id)
	if err != nil {
		return provider.Track{ID: id, CatalogID: id}, err
	}
	t, _ = p.toTrack(*e)
	return t, nil
}

// GetSongDetails implements provider.SongDetailer.
func (p *Public) GetSongDetails(_ context.Context, t provider.Track) (provider.SongDetails, error) {
	d := provider.SongDetails{URL: pageURL(t.ID)}
	p.mu.Lock()
	if a, ok := p.users[t.ID]; ok {
		d.Artists = []provider.Artist{a}
	}
	p.mu.Unlock()
	return d, nil
}

// StreamURL is not used: the stream comes from OpenStream.
func (p *Public) StreamURL(context.Context, string) (string, string, error) {
	return "", "", errors.New("soundcloud streams through OpenStream")
}

// OpenStream implements ystream.Opener: yt-dlp fetches the HLS stream,
// ffmpeg remuxes it to ADTS AAC.
func (p *Public) OpenStream(ctx context.Context, id string) (io.ReadCloser, string, *provider.Track, error) {
	e, info, err := ytdlp.Info(ctx, p.run, pageURL(id))
	if err != nil {
		return nil, "", nil, err
	}
	fid, copyAAC, err := ytdlp.PickFormat(e.Formats)
	if err != nil {
		return nil, "", nil, err
	}
	t, _ := p.toTrack(*e)
	rc, err := p.stream.Open(ctx, info, fid, copyAAC)
	if err != nil {
		return nil, "", nil, err
	}
	return rc, "aac", &t, nil
}

var (
	_ provider.Provider     = (*Public)(nil)
	_ provider.SongDetailer = (*Public)(nil)
)

// RemoveFromPlaylist, RenamePlaylist and DeletePlaylist edit the playlists
// kept on disk.
func (p *Public) RemoveFromPlaylist(_ context.Context, playlistID string, index int, t provider.Track) error {
	if playlistID == provider.FavoritesPlaylistID {
		return p.store.SetFavorite(t, false)
	}
	return p.store.RemoveFromPlaylist(playlistID, index, t.ID)
}

func (p *Public) RenamePlaylist(_ context.Context, playlistID, name string) error {
	return p.store.RenamePlaylist(playlistID, name)
}

func (p *Public) DeletePlaylist(_ context.Context, playlistID string) error {
	return p.store.DeletePlaylist(playlistID)
}

var _ provider.PlaylistEditor = (*Public)(nil)

// Saved albums and followed artists are kept on disk, like the playlists.
func (p *Public) GetSavedAlbums(context.Context) ([]provider.Album, error) {
	return p.store.SavedAlbums(), nil
}

func (p *Public) GetSavedArtists(context.Context) ([]provider.Artist, error) {
	return p.store.SavedArtists(), nil
}

func (p *Public) SaveAlbum(_ context.Context, a provider.Album, save bool) error {
	return p.store.SetAlbumSaved(a, save)
}

func (p *Public) FollowArtist(_ context.Context, a provider.Artist, follow bool) error {
	return p.store.SetArtistFollowed(a, follow)
}

var (
	_ provider.LibraryCollections = (*Public)(nil)
	_ provider.CollectionSaver    = (*Public)(nil)
)
