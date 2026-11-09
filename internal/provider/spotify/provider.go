package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tphx37/utuimp/internal/i18n"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
)

// ── API models ─────────────────────────────────────────────────────────────

type spImage struct {
	URL   string `json:"url"`
	Width int    `json:"width"`
}

type spArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type spAlbum struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	ReleaseDate string     `json:"release_date"`
	TotalTracks int        `json:"total_tracks"`
	Images      []spImage  `json:"images"`
	Artists     []spArtist `json:"artists"`
	Tracks      *struct {
		Items []spTrack `json:"items"`
		Next  string    `json:"next"`
	} `json:"tracks"`
}

type spTrack struct {
	ID           string     `json:"id"`
	URI          string     `json:"uri"`
	Name         string     `json:"name"`
	DurationMs   int64      `json:"duration_ms"`
	Explicit     bool       `json:"explicit"`
	DiscNumber   int        `json:"disc_number"`
	TrackNumber  int        `json:"track_number"`
	IsPlayable   *bool      `json:"is_playable"`
	IsLocal      bool       `json:"is_local"`
	Artists      []spArtist `json:"artists"`
	Album        *spAlbum   `json:"album"`
	ExternalURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

type spPlaylist struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Images []spImage `json:"images"`
	Owner  struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	Tracks *struct {
		Total int `json:"total"`
	} `json:"tracks"`
	Items *struct {
		Total int `json:"total"`
	} `json:"items"`
}

type page[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next"`
}

// ── Provider ───────────────────────────────────────────────────────────────

// likedID is the synthetic "Liked Songs" playlist.
const likedID = provider.FavoritesPlaylistID

// Provider implements provider.Provider for Spotify.
type Provider struct {
	c *Client

	mu    sync.Mutex
	cache map[string]provider.Track
}

// New returns a provider using an authorized client.
func New(c *Client) *Provider { return &Provider{c: c, cache: map[string]provider.Track{}} }

// Client exposes the API client (the Connect player uses it).
func (p *Provider) Client() *Client { return p.c }

func (p *Provider) Name() string          { return "Spotify" }
func (p *Provider) IsAuthenticated() bool { return true }

func bestImage(imgs []spImage) string {
	best := ""
	bw := 0
	for _, im := range imgs {
		// Prefer about 300px wide: big enough for the art view, small to fetch.
		if best == "" || (im.Width >= 250 && (bw < 250 || im.Width < bw)) {
			best, bw = im.URL, im.Width
		}
	}
	return best
}

func artistNames(as []spArtist) string {
	n := make([]string, 0, len(as))
	for _, a := range as {
		n = append(n, a.Name)
	}
	return strings.Join(n, ", ")
}

func (p *Provider) toTrack(t spTrack, album *spAlbum) provider.Track {
	if album == nil {
		album = t.Album
	}
	tr := provider.Track{
		ID: t.ID, CatalogID: t.ID,
		Title:       provider.Clean(t.Name),
		Artist:      provider.Clean(artistNames(t.Artists)),
		Duration:    time.Duration(t.DurationMs) * time.Millisecond,
		DiscNumber:  t.DiscNumber,
		TrackNumber: t.TrackNumber,
	}
	if t.Explicit {
		tr.ContentRating = "explicit"
	}
	if album != nil {
		tr.Album = provider.Clean(album.Name)
		tr.ArtworkURL = bestImage(album.Images)
		tr.ReleaseDate = album.ReleaseDate
	}
	p.mu.Lock()
	p.cache[tr.ID] = tr
	p.mu.Unlock()
	return tr
}

func (p *Provider) toTracks(ts []spTrack, album *spAlbum) []provider.Track {
	out := make([]provider.Track, 0, len(ts))
	for _, t := range ts {
		if t.ID == "" || t.IsLocal || (t.IsPlayable != nil && !*t.IsPlayable) {
			continue
		}
		out = append(out, p.toTrack(t, album))
	}
	return out
}

func toAlbum(a spAlbum) provider.Album {
	return provider.Album{ID: a.ID, Title: provider.Clean(a.Name), Artist: provider.Clean(artistNames(a.Artists)),
		ArtworkURL: bestImage(a.Images), TrackCount: a.TotalTracks, ReleaseDate: a.ReleaseDate}
}

func toPlaylist(pl spPlaylist) provider.Playlist {
	n := 0
	if pl.Tracks != nil {
		n = pl.Tracks.Total
	} else if pl.Items != nil {
		n = pl.Items.Total
	}
	return provider.Playlist{ID: pl.ID, Name: provider.Clean(pl.Name), TrackCount: n, ArtworkURL: bestImage(pl.Images)}
}

// Lookup returns cached metadata for a track, fetching it when unknown.
func (p *Provider) Lookup(ctx context.Context, id string) (provider.Track, error) {
	p.mu.Lock()
	t, ok := p.cache[id]
	p.mu.Unlock()
	if ok {
		return t, nil
	}
	var st spTrack
	if err := p.c.get(ctx, "/tracks/"+url.PathEscape(id), nil, &st); err != nil {
		return provider.Track{ID: id}, err
	}
	return p.toTrack(st, nil), nil
}

// maxLimit is the largest page Spotify serves development-mode apps for
// search and artist albums (since February 2026; more is "Invalid limit").
const maxLimit = 10

// all follows "next" links, collecting up to max items.
func all[T any](ctx context.Context, c *Client, path string, q url.Values, max int) ([]T, error) {
	var out []T
	var pg page[T]
	if err := c.get(ctx, path, q, &pg); err != nil {
		return nil, err
	}
	out = append(out, pg.Items...)
	for pg.Next != "" && len(out) < max {
		next := pg.Next
		pg = page[T]{}
		if err := c.get(ctx, next, nil, &pg); err != nil {
			return out, err
		}
		out = append(out, pg.Items...)
	}
	return out, nil
}

func (p *Provider) Search(ctx context.Context, query string) (*provider.SearchResult, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("type", "track,album,artist,playlist")
	q.Set("limit", strconv.Itoa(maxLimit))
	var res struct {
		Tracks    page[spTrack]     `json:"tracks"`
		Albums    page[spAlbum]     `json:"albums"`
		Artists   page[spArtist]    `json:"artists"`
		Playlists page[*spPlaylist] `json:"playlists"`
	}
	if err := p.c.get(ctx, "/search", q, &res); err != nil {
		return nil, err
	}
	out := &provider.SearchResult{Tracks: p.toTracks(res.Tracks.Items, nil)}
	for _, a := range res.Albums.Items {
		out.Albums = append(out.Albums, toAlbum(a))
	}
	for _, a := range res.Artists.Items {
		out.Artists = append(out.Artists, provider.Artist{ID: a.ID, Name: provider.Clean(a.Name)})
	}
	for _, pl := range res.Playlists.Items {
		if pl != nil { // Spotify returns nulls for unavailable playlists
			out.Playlists = append(out.Playlists, toPlaylist(*pl))
		}
	}
	return out, nil
}

// GetLibraryTracks returns Liked Songs, most recently liked first.
func (p *Provider) GetLibraryTracks(ctx context.Context) ([]provider.Track, error) {
	q := url.Values{}
	q.Set("limit", "50")
	items, err := all[struct {
		Track spTrack `json:"track"`
	}](ctx, p.c, "/me/tracks", q, 2000)
	ts := make([]spTrack, 0, len(items))
	for _, it := range items {
		ts = append(ts, it.Track)
	}
	if err != nil && len(ts) == 0 {
		return nil, err
	}
	return p.toTracks(ts, nil), nil
}

func (p *Provider) GetLibraryPlaylists(ctx context.Context) ([]provider.Playlist, error) {
	q := url.Values{}
	q.Set("limit", "50")
	pls, err := all[*spPlaylist](ctx, p.c, "/me/playlists", q, 2000)
	if err != nil && len(pls) == 0 {
		return nil, err
	}
	out := []provider.Playlist{{ID: likedID, Name: i18n.T("Liked Songs"), ReadOnly: true}}
	for _, pl := range pls {
		if pl != nil {
			out = append(out, toPlaylist(*pl))
		}
	}
	return out, nil
}

func (p *Provider) GetPlaylistTracks(ctx context.Context, playlistID string) ([]provider.Track, error) {
	if playlistID == likedID {
		return p.GetLibraryTracks(ctx)
	}
	// /playlists/{id}/tracks answers 403 since Spotify's February 2026 API
	// change; /items replaces it and puts the track under "item".
	q := url.Values{}
	q.Set("limit", "50")
	items, err := all[struct {
		Item  *spTrack `json:"item"`
		Track *spTrack `json:"track"`
	}](ctx, p.c, "/playlists/"+url.PathEscape(playlistID)+"/items", q, 5000)
	ts := make([]spTrack, 0, len(items))
	for _, it := range items {
		switch {
		case it.Item != nil:
			ts = append(ts, *it.Item)
		case it.Track != nil:
			ts = append(ts, *it.Track)
		}
	}
	if err != nil && len(ts) == 0 {
		return nil, err
	}
	return p.toTracks(ts, nil), nil
}

func (p *Provider) GetCatalogPlaylistTracks(ctx context.Context, playlistID string) ([]provider.Track, error) {
	return p.GetPlaylistTracks(ctx, playlistID)
}

func (p *Provider) GetAlbumTracks(ctx context.Context, albumID string) ([]provider.Track, error) {
	var al spAlbum
	if err := p.c.get(ctx, "/albums/"+url.PathEscape(albumID), nil, &al); err != nil {
		return nil, err
	}
	if al.Tracks == nil {
		return nil, nil
	}
	ts := append([]spTrack(nil), al.Tracks.Items...)
	for next := al.Tracks.Next; next != ""; {
		var pg page[spTrack]
		if err := p.c.get(ctx, next, nil, &pg); err != nil {
			break
		}
		ts = append(ts, pg.Items...)
		next = pg.Next
	}
	return p.toTracks(ts, &al), nil
}

func (p *Provider) GetLibraryAlbumTracks(ctx context.Context, albumID string) ([]provider.Track, error) {
	return p.GetAlbumTracks(ctx, albumID)
}

func (p *Provider) GetArtistAlbums(ctx context.Context, artistID string) ([]provider.Album, error) {
	q := url.Values{}
	q.Set("include_groups", "album,single")
	q.Set("limit", strconv.Itoa(maxLimit))
	as, err := all[spAlbum](ctx, p.c, "/artists/"+url.PathEscape(artistID)+"/albums", q, 500)
	if err != nil && len(as) == 0 {
		return nil, err
	}
	out := make([]provider.Album, 0, len(as))
	for _, a := range as {
		out = append(out, toAlbum(a))
	}
	return out, nil
}

func uris(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "spotify:track:" + id
	}
	return out
}

func (p *Provider) CreatePlaylist(ctx context.Context, name string, trackIDs []string) (provider.Playlist, error) {
	var pl spPlaylist
	if err := p.c.call(ctx, http.MethodPost, "/me/playlists", nil, map[string]any{"name": name, "public": false}, &pl); err != nil {
		return provider.Playlist{}, err
	}
	for start := 0; start < len(trackIDs); start += 100 {
		end := min(start+100, len(trackIDs))
		if err := p.c.call(ctx, http.MethodPost, "/playlists/"+url.PathEscape(pl.ID)+"/items", nil,
			map[string]any{"uris": uris(trackIDs[start:end])}, nil); err != nil {
			return toPlaylist(pl), err
		}
	}
	return toPlaylist(pl), nil
}

func (p *Provider) AddToPlaylist(ctx context.Context, playlistID, trackID string) error {
	if playlistID == likedID {
		return p.LoveSong(ctx, trackID, true)
	}
	return p.c.call(ctx, http.MethodPost, "/playlists/"+url.PathEscape(playlistID)+"/items", nil,
		map[string]any{"uris": uris([]string{trackID})}, nil)
}

// LoveSong saves or removes a track through /me/library, which replaced the
// /me/tracks write endpoints in February 2026.
func (p *Provider) LoveSong(ctx context.Context, trackID string, loved bool) error {
	q := url.Values{}
	q.Set("uris", uris([]string{trackID})[0])
	method := http.MethodPut
	if !loved {
		method = http.MethodDelete
	}
	return p.c.call(ctx, method, "/me/library", q, nil, nil)
}

func (p *Provider) GetSongRating(ctx context.Context, trackID string) (bool, error) {
	q := url.Values{}
	q.Set("uris", uris([]string{trackID})[0])
	var res []bool
	if err := p.c.get(ctx, "/me/library/contains", q, &res); err != nil {
		return false, err
	}
	return len(res) > 0 && res[0], nil
}

// GetRecommendations shows the user's playlists (Spotify retired its
// recommendations endpoints for new apps).
func (p *Provider) GetRecommendations(ctx context.Context) ([]provider.RecommendationGroup, error) {
	pls, err := p.GetLibraryPlaylists(ctx)
	if err != nil {
		return nil, err
	}
	g := provider.RecommendationGroup{Title: i18n.T("Your playlists")}
	for _, pl := range pls {
		if pl.ID != likedID {
			g.Items = append(g.Items, provider.RecommendationItem{ID: pl.ID, Kind: "playlist", Title: pl.Name})
		}
	}
	return []provider.RecommendationGroup{g}, nil
}

// GetStationTracks approximates radio with songs by the seed's artist found
// through search (Spotify's radio, recommendations and artist top-tracks
// endpoints aren't open to new apps).
func (p *Provider) GetStationTracks(ctx context.Context, seed string) ([]provider.Track, error) {
	var st spTrack
	if err := p.c.get(ctx, "/tracks/"+url.PathEscape(seed), nil, &st); err != nil {
		return nil, err
	}
	if len(st.Artists) == 0 {
		return nil, errors.New("no artist to build a radio from")
	}
	var ts []spTrack
	for offset := 0; offset < 3*maxLimit; offset += maxLimit {
		q := url.Values{}
		q.Set("q", `artist:"`+strings.ReplaceAll(st.Artists[0].Name, `"`, "")+`"`)
		q.Set("type", "track")
		q.Set("limit", strconv.Itoa(maxLimit))
		q.Set("offset", strconv.Itoa(offset))
		var res struct {
			Tracks page[spTrack] `json:"tracks"`
		}
		if err := p.c.get(ctx, "/search", q, &res); err != nil {
			if len(ts) > 0 {
				break
			}
			return nil, err
		}
		for _, t := range res.Tracks.Items {
			if t.ID != seed && hasArtist(t.Artists, st.Artists[0].ID) {
				ts = append(ts, t)
			}
		}
		if res.Tracks.Next == "" {
			break
		}
	}
	return p.toTracks(ts, nil), nil
}

func hasArtist(as []spArtist, id string) bool {
	for _, a := range as {
		if a.ID == id {
			return true
		}
	}
	return false
}

// GetSongDetails implements provider.SongDetailer.
func (p *Provider) GetSongDetails(ctx context.Context, t provider.Track) (provider.SongDetails, error) {
	var st spTrack
	if err := p.c.get(ctx, "/tracks/"+url.PathEscape(t.ID), nil, &st); err != nil {
		return provider.SongDetails{}, err
	}
	d := provider.SongDetails{URL: st.ExternalURLs.Spotify}
	if st.Album != nil {
		a := toAlbum(*st.Album)
		d.Album = &a
	}
	for _, a := range st.Artists {
		d.Artists = append(d.Artists, provider.Artist{ID: a.ID, Name: provider.Clean(a.Name)})
	}
	return d, nil
}

var (
	_ provider.Provider     = (*Provider)(nil)
	_ provider.SongDetailer = (*Provider)(nil)
)

// ParseTrack converts a track object as the Web API returns it (e.g. the
// "item" of the playback state) and caches it, so the player needs no extra
// request per song.
func (p *Provider) ParseTrack(raw []byte) (provider.Track, error) {
	var st spTrack
	if err := json.Unmarshal(raw, &st); err != nil {
		return provider.Track{}, err
	}
	if st.ID == "" {
		return provider.Track{}, errors.New("no track")
	}
	return p.toTrack(st, nil), nil
}
