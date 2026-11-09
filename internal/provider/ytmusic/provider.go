// Package ytmusic plays YouTube Music through yt-dlp: it lists searches,
// albums, artists' releases and mixes with yt-dlp's flat extraction, and
// streams each song as yt-dlp's best AAC audio remuxed to ADTS by ffmpeg
// (opus is re-encoded). utuimp can't write to the YouTube account, so
// favourites and playlists are kept in a local file.
package ytmusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/localstore"
)

// MixPrefix starts the ID of a song's mix ("radio"): RDAMVM + video ID.
const MixPrefix = "RDAMVM"

// Runner runs yt-dlp with args and returns its standard output.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// Provider implements provider.Provider for YouTube Music.
type Provider struct {
	run    Runner
	stream streamer
	store  *localstore.Store

	mu      sync.Mutex
	cache   map[string]provider.Track
	artists map[string]provider.Artist // track ID → its artist's channel
	albums  map[string]provider.Album  // track ID → the album it was listed in
}

// New returns a provider. ytdlp and ffmpeg are the programs' paths;
// cookies is yt-dlp's cookie source: a cookies.txt path or a browser name
// for --cookies-from-browser ("" for none).
func New(ytdlp, ffmpeg, cookies string, store *localstore.Store) *Provider {
	base := cookieArgs(cookies)
	run := execRunner(ytdlp, base)
	return &Provider{
		run:    run,
		stream: streamer{ytdlp: ytdlp, ffmpeg: ffmpeg, base: base, run: run},
		store:  store,
		cache:  map[string]provider.Track{}, artists: map[string]provider.Artist{}, albums: map[string]provider.Album{},
	}
}

func (p *Provider) Name() string          { return "YouTube Music" }
func (p *Provider) IsAuthenticated() bool { return true }

// ── yt-dlp JSON ────────────────────────────────────────────────────────────

type thumb struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type entry struct {
	Type        string   `json:"_type"`
	ID          string   `json:"id"`
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Track       string   `json:"track"`
	Artist      string   `json:"artist"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album"`
	Channel     string   `json:"channel"`
	ChannelID   string   `json:"channel_id"`
	Uploader    string   `json:"uploader"`
	Duration    float64  `json:"duration"`
	ReleaseYear int      `json:"release_year"`
	ReleaseDate string   `json:"release_date"` // YYYYMMDD
	Thumbnail   string   `json:"thumbnail"`
	Thumbnails  []thumb  `json:"thumbnails"`
	Entries     []entry  `json:"entries"`
	Formats     []format `json:"formats"`
}

type format struct {
	FormatID string  `json:"format_id"`
	Acodec   string  `json:"acodec"`
	Vcodec   string  `json:"vcodec"`
	Abr      float64 `json:"abr"`
}

// cleanName drops YouTube's decorations from channel and album names.
func cleanName(s string) string {
	s = strings.TrimSpace(s)
	for _, suf := range []string{" - Topic", " playlist", " - Releases", " - Videos"} {
		s = strings.TrimSuffix(s, suf)
	}
	for _, pre := range []string{"Album - ", "EP - ", "Single - ", "Playlist - "} {
		s = strings.TrimPrefix(s, pre)
	}
	return provider.Clean(s)
}

// bestThumb picks a thumbnail about 400px wide.
func bestThumb(ts []thumb) string {
	best, bw := "", 0
	for _, t := range ts {
		if t.URL == "" {
			continue
		}
		if best == "" || (t.Width >= 300 && (bw < 300 || t.Width < bw)) || (bw < 300 && t.Width > bw) {
			best, bw = t.URL, t.Width
		}
	}
	return best
}

func (e entry) artistName() string {
	switch {
	case len(e.Artists) > 0:
		return strings.Join(e.Artists, ", ")
	case e.Artist != "":
		return e.Artist
	case e.Channel != "":
		return cleanName(e.Channel)
	default:
		return cleanName(e.Uploader)
	}
}

func releaseDate(e entry) string {
	if len(e.ReleaseDate) == 8 {
		return e.ReleaseDate[:4] + "-" + e.ReleaseDate[4:6] + "-" + e.ReleaseDate[6:]
	}
	if e.ReleaseYear > 0 {
		return fmt.Sprint(e.ReleaseYear)
	}
	return ""
}

// toTrack converts a video entry and caches it. A cached track keeps the
// fields the entry lacks (a search result has no artist).
func (p *Provider) toTrack(e entry) provider.Track {
	title := e.Track
	if title == "" {
		title = e.Title
	}
	t := provider.Track{
		ID: e.ID, CatalogID: e.ID,
		Title:       provider.Clean(title),
		Artist:      provider.Clean(e.artistName()),
		Album:       provider.Clean(e.Album),
		Duration:    time.Duration(e.Duration * float64(time.Second)),
		ArtworkURL:  e.Thumbnail,
		ReleaseDate: releaseDate(e),
	}
	if t.ArtworkURL == "" {
		t.ArtworkURL = bestThumb(e.Thumbnails)
	}
	if t.ArtworkURL == "" && e.ID != "" {
		t.ArtworkURL = "https://i.ytimg.com/vi/" + e.ID + "/hqdefault.jpg"
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if old, ok := p.cache[t.ID]; ok {
		t = merge(old, t)
	}
	p.cache[t.ID] = t
	if e.ChannelID != "" && t.Artist != "" {
		p.artists[t.ID] = provider.Artist{ID: e.ChannelID, Name: t.Artist}
	}
	return t
}

// merge fills the empty fields of t from old.
func merge(old, t provider.Track) provider.Track {
	if t.Artist == "" {
		t.Artist = old.Artist
	}
	if t.Album == "" {
		t.Album = old.Album
	}
	if t.Duration == 0 {
		t.Duration = old.Duration
	}
	if t.ReleaseDate == "" {
		t.ReleaseDate = old.ReleaseDate
	}
	if t.TrackNumber == 0 {
		t.TrackNumber = old.TrackNumber
	}
	// Album art (square) beats a video thumbnail.
	if old.ArtworkURL != "" && !strings.Contains(old.ArtworkURL, "/vi/") {
		t.ArtworkURL = old.ArtworkURL
	}
	return t
}

func (p *Provider) toTracks(es []entry) []provider.Track {
	out := make([]provider.Track, 0, len(es))
	for _, e := range es {
		if e.ID == "" || (e.Type != "" && e.Type != "url" && e.Type != "video") {
			continue
		}
		out = append(out, p.toTrack(e))
	}
	return out
}

// flat lists url without resolving each item; limit 0 means all.
func (p *Provider) flat(ctx context.Context, u string, limit int) (*entry, error) {
	args := []string{"--flat-playlist", "-J"}
	if limit > 0 {
		args = append(args, "--playlist-end", fmt.Sprint(limit))
	}
	out, err := p.run(ctx, append(args, u)...)
	if err != nil {
		return nil, err
	}
	var e entry
	if err := json.Unmarshal(out, &e); err != nil {
		return nil, fmt.Errorf("yt-dlp output: %w", err)
	}
	return &e, nil
}

func watchURL(id string) string { return "https://music.youtube.com/watch?v=" + url.QueryEscape(id) }

// listURL is the page of an album or playlist ID.
func listURL(id string) string {
	if strings.HasPrefix(id, "MPREb") {
		return "https://music.youtube.com/browse/" + url.PathEscape(id)
	}
	if v, ok := strings.CutPrefix(id, MixPrefix); ok {
		return "https://www.youtube.com/watch?v=" + url.QueryEscape(v) + "&list=" + url.QueryEscape(id)
	}
	return "https://www.youtube.com/playlist?list=" + url.QueryEscape(id)
}

// ── Provider ───────────────────────────────────────────────────────────────

func (p *Provider) Search(ctx context.Context, query string) (*provider.SearchResult, error) {
	search := func(kind string, limit int) (*entry, error) {
		return p.flat(ctx, "https://music.youtube.com/search?q="+url.QueryEscape(query)+"#"+kind, limit)
	}
	var (
		wg                 sync.WaitGroup
		songs, arts        *entry
		albums             []provider.Album
		songErr, alb, artE error
	)
	wg.Add(3)
	go func() { defer wg.Done(); songs, songErr = search("songs", 20) }()
	go func() { defer wg.Done(); albums, alb = p.searchAlbums(ctx, query, 4) }()
	go func() { defer wg.Done(); arts, artE = search("artists", 3) }()
	wg.Wait()
	if songErr != nil {
		return nil, songErr
	}
	res := &provider.SearchResult{Tracks: p.toTracks(songs.Entries), Albums: albums}
	if alb != nil {
		res.Warnings = append(res.Warnings, "albums: "+alb.Error())
	}
	if artE != nil {
		res.Warnings = append(res.Warnings, "artists: "+artE.Error())
	}

	// Artist results come as bare IDs: look up their names.
	var mu sync.Mutex
	if arts != nil {
		res.Artists = make([]provider.Artist, len(arts.Entries))
		for i, e := range arts.Entries {
			wg.Add(1)
			go func() {
				defer wg.Done()
				d, err := p.flat(ctx, "https://music.youtube.com/browse/"+url.PathEscape(e.ID), 1)
				if err != nil {
					return
				}
				name := d.Channel
				if name == "" {
					name = d.Title
				}
				mu.Lock()
				res.Artists[i] = provider.Artist{ID: e.ID, Name: cleanName(name)}
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	res.Artists = slices.DeleteFunc(res.Artists, func(a provider.Artist) bool { return a.Name == "" })
	return res, nil
}

// searchAlbums searches albums and looks up their names (results come as
// bare IDs).
func (p *Provider) searchAlbums(ctx context.Context, query string, limit int) ([]provider.Album, error) {
	d, err := p.flat(ctx, "https://music.youtube.com/search?q="+url.QueryEscape(query)+"#albums", limit)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Album, len(d.Entries))
	var wg sync.WaitGroup
	for i, e := range d.Entries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := p.flat(ctx, listURL(e.ID), 1)
			if err != nil || d.Title == "" {
				return
			}
			a := provider.Album{ID: d.ID, Title: cleanName(d.Title), ArtworkURL: bestThumb(d.Thumbnails)}
			if a.ID == "" {
				a.ID = e.ID
			}
			if len(d.Entries) > 0 {
				a.Artist = provider.Clean(d.Entries[0].artistName())
			}
			out[i] = a
		}()
	}
	wg.Wait()
	return slices.DeleteFunc(out, func(a provider.Album) bool { return a.Title == "" }), nil
}

// GetLibraryTracks returns the favourite songs.
func (p *Provider) GetLibraryTracks(context.Context) ([]provider.Track, error) {
	fav := p.store.Favorites()
	p.remember(fav)
	return fav, nil
}

func (p *Provider) remember(ts []provider.Track) {
	p.mu.Lock()
	for _, t := range ts {
		if _, ok := p.cache[t.ID]; !ok {
			p.cache[t.ID] = t
		}
	}
	p.mu.Unlock()
}

func (p *Provider) GetLibraryPlaylists(context.Context) ([]provider.Playlist, error) {
	out := []provider.Playlist{{ID: provider.FavoritesPlaylistID, Name: i18n.T("Favorites"),
		TrackCount: len(p.store.Favorites()), ReadOnly: true}}
	for _, pl := range p.store.Playlists() {
		out = append(out, provider.Playlist{ID: pl.ID, Name: pl.Name, TrackCount: len(pl.Tracks)})
	}
	return out, nil
}

func (p *Provider) GetPlaylistTracks(ctx context.Context, id string) ([]provider.Track, error) {
	if id == provider.FavoritesPlaylistID {
		return p.GetLibraryTracks(ctx)
	}
	if strings.HasPrefix(id, localstore.PlaylistPrefix) {
		pl, ok := p.store.Playlist(id)
		if !ok {
			return nil, errors.New("no such playlist")
		}
		p.remember(pl.Tracks)
		return pl.Tracks, nil
	}
	d, err := p.flat(ctx, listURL(id), 200)
	if err != nil {
		return nil, err
	}
	return p.toTracks(d.Entries), nil
}

func (p *Provider) GetCatalogPlaylistTracks(ctx context.Context, id string) ([]provider.Track, error) {
	return p.GetPlaylistTracks(ctx, id)
}

func (p *Provider) GetAlbumTracks(ctx context.Context, id string) ([]provider.Track, error) {
	d, err := p.flat(ctx, listURL(id), 0)
	if err != nil {
		return nil, err
	}
	al := provider.Album{ID: d.ID, Title: cleanName(d.Title), ArtworkURL: bestThumb(d.Thumbnails)}
	ts := make([]provider.Track, 0, len(d.Entries))
	for i, e := range d.Entries {
		if e.ID == "" {
			continue
		}
		e.Album = al.Title
		if al.ArtworkURL != "" {
			e.Thumbnail = al.ArtworkURL
		}
		t := p.toTrack(e)
		t.TrackNumber = i + 1
		if al.Artist == "" {
			al.Artist = t.Artist
		}
		p.mu.Lock()
		p.cache[t.ID] = t
		p.albums[t.ID] = al
		p.mu.Unlock()
		ts = append(ts, t)
	}
	return ts, nil
}

func (p *Provider) GetLibraryAlbumTracks(ctx context.Context, id string) ([]provider.Track, error) {
	return p.GetAlbumTracks(ctx, id)
}

// GetArtistAlbums lists the artist's releases. Auto-generated "Topic"
// channels have none: their albums are found by searching the name.
func (p *Provider) GetArtistAlbums(ctx context.Context, artistID string) ([]provider.Album, error) {
	d, err := p.flat(ctx, "https://www.youtube.com/channel/"+url.PathEscape(artistID)+"/releases", 100)
	if err != nil || len(d.Entries) == 0 {
		ch, cerr := p.flat(ctx, "https://music.youtube.com/browse/"+url.PathEscape(artistID), 1)
		if cerr != nil {
			if err == nil {
				err = cerr
			}
			return nil, err
		}
		name := cleanName(ch.Channel)
		if name == "" {
			name = cleanName(ch.Title)
		}
		all, serr := p.searchAlbums(ctx, name, 12)
		if serr != nil {
			return nil, serr
		}
		own := slices.DeleteFunc(slices.Clone(all), func(a provider.Album) bool { return !strings.EqualFold(a.Artist, name) })
		if len(own) == 0 {
			own = all
		}
		return own, nil
	}
	artist := cleanName(d.Channel)
	out := make([]provider.Album, 0, len(d.Entries))
	for _, e := range d.Entries {
		if e.ID != "" && e.Title != "" {
			out = append(out, provider.Album{ID: e.ID, Title: cleanName(e.Title), Artist: artist, ArtworkURL: bestThumb(e.Thumbnails)})
		}
	}
	return out, nil
}

// CreatePlaylist saves a playlist locally.
func (p *Provider) CreatePlaylist(ctx context.Context, name string, ids []string) (provider.Playlist, error) {
	ts := make([]provider.Track, 0, len(ids))
	for _, id := range ids {
		t, _ := p.Lookup(ctx, id)
		ts = append(ts, t)
	}
	pl, err := p.store.CreatePlaylist(name, ts)
	return provider.Playlist{ID: pl.ID, Name: pl.Name, TrackCount: len(pl.Tracks)}, err
}

func (p *Provider) AddToPlaylist(ctx context.Context, playlistID, trackID string) error {
	if playlistID == provider.FavoritesPlaylistID {
		return p.LoveSong(ctx, trackID, true)
	}
	t, _ := p.Lookup(ctx, trackID)
	return p.store.AddToPlaylist(playlistID, t)
}

// LoveSong adds the song to (or removes it from) the local favourites.
func (p *Provider) LoveSong(ctx context.Context, id string, loved bool) error {
	t, _ := p.Lookup(ctx, id)
	return p.store.SetFavorite(t, loved)
}

func (p *Provider) GetSongRating(_ context.Context, id string) (bool, error) {
	return p.store.IsFavorite(id), nil
}

// GetRecommendations offers a mix for each recent favourite and the saved
// playlists.
func (p *Provider) GetRecommendations(context.Context) ([]provider.RecommendationGroup, error) {
	mixes := provider.RecommendationGroup{Title: i18n.T("Mixes of your favorites")}
	for _, t := range p.store.Favorites() {
		if len(mixes.Items) == 12 {
			break
		}
		mixes.Items = append(mixes.Items, provider.RecommendationItem{ID: MixPrefix + t.ID, Kind: "playlist",
			Title: i18n.T("Mix: ") + t.Title, Subtitle: t.Artist})
	}
	if len(mixes.Items) == 0 {
		mixes.Title = i18n.T("Press f on songs you like: their mixes show up here")
	}
	groups := []provider.RecommendationGroup{mixes}
	if pls := p.store.Playlists(); len(pls) > 0 {
		g := provider.RecommendationGroup{Title: i18n.T("Your playlists")}
		for _, pl := range pls {
			g.Items = append(g.Items, provider.RecommendationItem{ID: pl.ID, Kind: "playlist", Title: pl.Name})
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// GetStationTracks returns the song's mix.
func (p *Provider) GetStationTracks(ctx context.Context, seed string) ([]provider.Track, error) {
	d, err := p.flat(ctx, listURL(MixPrefix+seed), 30)
	if err != nil {
		return nil, err
	}
	ts := p.toTracks(d.Entries)
	return slices.DeleteFunc(ts, func(t provider.Track) bool { return t.ID == seed }), nil
}

// Lookup returns what is known about a song, asking yt-dlp when nothing is.
func (p *Provider) Lookup(ctx context.Context, id string) (provider.Track, error) {
	p.mu.Lock()
	t, ok := p.cache[id]
	p.mu.Unlock()
	if ok {
		return t, nil
	}
	out, err := p.run(ctx, "-J", "--skip-download", "--no-playlist", watchURL(id))
	if err != nil {
		return provider.Track{ID: id, CatalogID: id}, err
	}
	var e entry
	if err := json.Unmarshal(out, &e); err != nil {
		return provider.Track{ID: id, CatalogID: id}, err
	}
	return p.toTrack(e), nil
}

// GetSongDetails implements provider.SongDetailer.
func (p *Provider) GetSongDetails(_ context.Context, t provider.Track) (provider.SongDetails, error) {
	d := provider.SongDetails{URL: watchURL(t.ID)}
	p.mu.Lock()
	if a, ok := p.artists[t.ID]; ok {
		d.Artists = []provider.Artist{a}
	}
	if al, ok := p.albums[t.ID]; ok {
		d.Album = &al
	}
	p.mu.Unlock()
	return d, nil
}

var (
	_ provider.Provider     = (*Provider)(nil)
	_ provider.SongDetailer = (*Provider)(nil)
)
