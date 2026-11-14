package soundcloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/provider"
)

// ── API models ─────────────────────────────────────────────────────────────

type scUser struct {
	ID           int64  `json:"id"`
	URN          string `json:"urn"`
	Username     string `json:"username"`
	AvatarURL    string `json:"avatar_url"`
	PermalinkURL string `json:"permalink_url"`
}

func (u scUser) urn() string {
	if u.URN == "" && u.ID != 0 {
		return "soundcloud:users:" + strconv.FormatInt(u.ID, 10)
	}
	return u.URN
}

type scTrack struct {
	ID             int64  `json:"id"`
	URN            string `json:"urn"`
	Title          string `json:"title"`
	Duration       int64  `json:"duration"` // ms
	ArtworkURL     string `json:"artwork_url"`
	PermalinkURL   string `json:"permalink_url"`
	Access         string `json:"access"` // playable, preview, blocked
	Streamable     *bool  `json:"streamable"`
	MetadataArtist string `json:"metadata_artist"`
	Genre          string `json:"genre"`
	CreatedAt      string `json:"created_at"`
	ReleaseYear    int    `json:"release_year"`
	User           scUser `json:"user"`
}

func (t scTrack) urn() string {
	if t.URN == "" && t.ID != 0 {
		return "soundcloud:tracks:" + strconv.FormatInt(t.ID, 10)
	}
	return t.URN
}

func (t scTrack) playable() bool {
	if t.Access == "blocked" || t.Access == "preview" {
		return false
	}
	return t.Streamable == nil || *t.Streamable
}

type scPlaylist struct {
	ID           int64     `json:"id"`
	URN          string    `json:"urn"`
	Title        string    `json:"title"`
	ArtworkURL   string    `json:"artwork_url"`
	TrackCount   int       `json:"track_count"`
	PlaylistType string    `json:"playlist_type"` // album, ep, single, compilation…
	ReleaseYear  int       `json:"release_year"`
	User         scUser    `json:"user"`
	Tracks       []scTrack `json:"tracks"`
}

func (p scPlaylist) urn() string {
	if p.URN == "" && p.ID != 0 {
		return "soundcloud:playlists:" + strconv.FormatInt(p.ID, 10)
	}
	return p.URN
}

func (p scPlaylist) isAlbum() bool {
	switch strings.ToLower(p.PlaylistType) {
	case "album", "ep", "single", "compilation":
		return true
	}
	return false
}

type page[T any] struct {
	Collection []T    `json:"collection"`
	NextHref   string `json:"next_href"`
}

// ── Provider ───────────────────────────────────────────────────────────────

// Synthetic IDs for listings that aren't SoundCloud playlists.
const (
	likesID        = provider.FavoritesPlaylistID
	relatedPrefix  = "utuimp:related:"    // + track URN: songs like it
	userTracksPref = "utuimp:usertracks:" // + user URN: everything they posted
)

// Provider implements provider.Provider for SoundCloud.
type Provider struct {
	c      *Client
	ffmpeg string // for AAC-only streams; "" when not installed

	mu    sync.Mutex
	cache map[string]scTrack
	liked map[string]bool // nil until the likes are loaded
}

// New returns a provider using a signed-in client.
func New(c *Client, ffmpeg string) *Provider {
	return &Provider{c: c, ffmpeg: ffmpeg, cache: map[string]scTrack{}}
}

func (p *Provider) Name() string          { return "SoundCloud" }
func (p *Provider) IsAuthenticated() bool { return true }

// artwork returns a ~500px version of a SoundCloud image ("-large" is 100px).
func artwork(u string) string {
	return strings.Replace(u, "-large.", "-t500x500.", 1)
}

func (p *Provider) toTrack(t scTrack) provider.Track {
	artist := t.MetadataArtist
	if artist == "" {
		artist = t.User.Username
	}
	art := t.ArtworkURL
	if art == "" {
		art = t.User.AvatarURL
	}
	tr := provider.Track{
		ID: t.urn(), CatalogID: t.urn(),
		Title:      provider.Clean(t.Title),
		Artist:     provider.Clean(artist),
		Duration:   time.Duration(t.Duration) * time.Millisecond,
		ArtworkURL: artwork(art),
	}
	if t.Genre != "" {
		tr.Genres = []string{t.Genre}
	}
	switch {
	case t.ReleaseYear > 0:
		tr.ReleaseDate = strconv.Itoa(t.ReleaseYear)
	case len(t.CreatedAt) >= 4:
		tr.ReleaseDate = t.CreatedAt[:4]
	}
	p.mu.Lock()
	p.cache[tr.ID] = t
	p.mu.Unlock()
	return tr
}

func (p *Provider) toTracks(ts []scTrack) []provider.Track {
	out := make([]provider.Track, 0, len(ts))
	for _, t := range ts {
		if t.urn() != "" && t.playable() {
			out = append(out, p.toTrack(t))
		}
	}
	return out
}

func toAlbum(pl scPlaylist) provider.Album {
	a := provider.Album{ID: pl.urn(), Title: provider.Clean(pl.Title), Artist: provider.Clean(pl.User.Username),
		ArtworkURL: artwork(pl.ArtworkURL), TrackCount: pl.TrackCount}
	if pl.ReleaseYear > 0 {
		a.ReleaseDate = strconv.Itoa(pl.ReleaseYear)
	}
	return a
}

func toPlaylist(pl scPlaylist) provider.Playlist {
	return provider.Playlist{ID: pl.urn(), Name: provider.Clean(pl.Title), TrackCount: pl.TrackCount, ArtworkURL: artwork(pl.ArtworkURL)}
}

// all follows next_href links, collecting up to max items.
func all[T any](ctx context.Context, c *Client, path string, q url.Values, max int) ([]T, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("linked_partitioning", "true")
	var out []T
	var pg page[T]
	if err := c.get(ctx, path, q, &pg); err != nil {
		return nil, err
	}
	out = append(out, pg.Collection...)
	for pg.NextHref != "" && len(out) < max {
		next := pg.NextHref
		pg = page[T]{}
		if err := c.get(ctx, next, nil, &pg); err != nil {
			return out, err
		}
		out = append(out, pg.Collection...)
	}
	return out, nil
}

func (p *Provider) Search(ctx context.Context, query string) (*provider.SearchResult, error) {
	q := func(limit int) url.Values {
		v := url.Values{}
		v.Set("q", query)
		v.Set("limit", strconv.Itoa(limit))
		v.Set("access", "playable")
		v.Set("linked_partitioning", "true")
		return v
	}
	var (
		wg     sync.WaitGroup
		tracks page[scTrack]
		pls    page[scPlaylist]
		users  page[scUser]
		tErr   error
		pErr   error
		uErr   error
	)
	wg.Add(3)
	go func() { defer wg.Done(); tErr = p.c.get(ctx, "/tracks", q(30), &tracks) }()
	go func() {
		defer wg.Done()
		v := q(10)
		v.Set("show_tracks", "false")
		pErr = p.c.get(ctx, "/playlists", v, &pls)
	}()
	go func() { defer wg.Done(); uErr = p.c.get(ctx, "/users", q(5), &users) }()
	wg.Wait()
	if tErr != nil {
		return nil, tErr
	}
	res := &provider.SearchResult{Tracks: p.toTracks(tracks.Collection)}
	if pErr != nil {
		res.Warnings = append(res.Warnings, "playlists: "+pErr.Error())
	}
	if uErr != nil {
		res.Warnings = append(res.Warnings, "artists: "+uErr.Error())
	}
	for _, pl := range pls.Collection {
		if pl.isAlbum() {
			res.Albums = append(res.Albums, toAlbum(pl))
		} else {
			res.Playlists = append(res.Playlists, toPlaylist(pl))
		}
	}
	for _, u := range users.Collection {
		res.Artists = append(res.Artists, provider.Artist{ID: u.urn(), Name: provider.Clean(u.Username)})
	}
	return res, nil
}

// GetLibraryTracks returns the liked tracks.
func (p *Provider) GetLibraryTracks(ctx context.Context) ([]provider.Track, error) {
	q := url.Values{}
	q.Set("limit", "200")
	ts, err := all[scTrack](ctx, p.c, "/me/likes/tracks", q, 2000)
	if err != nil && len(ts) == 0 {
		return nil, err
	}
	liked := make(map[string]bool, len(ts))
	for _, t := range ts {
		liked[t.urn()] = true
	}
	p.mu.Lock()
	p.liked = liked
	p.mu.Unlock()
	return p.toTracks(ts), nil
}

func (p *Provider) GetLibraryPlaylists(ctx context.Context) ([]provider.Playlist, error) {
	q := url.Values{}
	q.Set("limit", "50")
	q.Set("show_tracks", "false")
	pls, err := all[scPlaylist](ctx, p.c, "/me/playlists", q, 1000)
	if err != nil && len(pls) == 0 {
		return nil, err
	}
	out := []provider.Playlist{{ID: likesID, Name: i18n.T("Likes"), ReadOnly: true}}
	for _, pl := range pls {
		out = append(out, toPlaylist(pl))
	}
	return out, nil
}

func (p *Provider) GetPlaylistTracks(ctx context.Context, id string) ([]provider.Track, error) {
	switch {
	case id == likesID:
		return p.GetLibraryTracks(ctx)
	case strings.HasPrefix(id, relatedPrefix):
		return p.related(ctx, strings.TrimPrefix(id, relatedPrefix))
	case strings.HasPrefix(id, userTracksPref):
		q := url.Values{}
		q.Set("limit", "200")
		q.Set("access", "playable")
		ts, err := all[scTrack](ctx, p.c, "/users/"+strings.TrimPrefix(id, userTracksPref)+"/tracks", q, 1000)
		if err != nil && len(ts) == 0 {
			return nil, err
		}
		return p.toTracks(ts), nil
	}
	q := url.Values{}
	q.Set("access", "playable")
	ts, err := all[scTrack](ctx, p.c, "/playlists/"+id+"/tracks", q, 5000)
	if err != nil && len(ts) == 0 {
		return nil, err
	}
	return p.toTracks(ts), nil
}

func (p *Provider) GetCatalogPlaylistTracks(ctx context.Context, id string) ([]provider.Track, error) {
	return p.GetPlaylistTracks(ctx, id)
}

func (p *Provider) GetAlbumTracks(ctx context.Context, id string) ([]provider.Track, error) {
	ts, err := p.GetPlaylistTracks(ctx, id)
	for i := range ts {
		ts[i].TrackNumber = i + 1
	}
	return ts, err
}

func (p *Provider) GetLibraryAlbumTracks(ctx context.Context, id string) ([]provider.Track, error) {
	return p.GetAlbumTracks(ctx, id)
}

// GetArtistAlbums lists the user's sets, after an entry with all their
// tracks.
func (p *Provider) GetArtistAlbums(ctx context.Context, userURN string) ([]provider.Album, error) {
	q := url.Values{}
	q.Set("limit", "50")
	q.Set("show_tracks", "false")
	pls, err := all[scPlaylist](ctx, p.c, "/users/"+userURN+"/playlists", q, 500)
	if err != nil && len(pls) == 0 {
		return nil, err
	}
	out := []provider.Album{{ID: userTracksPref + userURN, Title: i18n.T("All tracks")}}
	for _, pl := range pls {
		out = append(out, toAlbum(pl))
	}
	return out, nil
}

func trackRefs(ids []string) []map[string]string {
	out := make([]map[string]string, len(ids))
	for i, id := range ids {
		out[i] = map[string]string{"urn": id}
	}
	return out
}

func (p *Provider) CreatePlaylist(ctx context.Context, name string, ids []string) (provider.Playlist, error) {
	var pl scPlaylist
	err := p.c.call(ctx, http.MethodPost, "/playlists", nil, map[string]any{"playlist": map[string]any{
		"title": name, "sharing": "private", "tracks": trackRefs(ids),
	}}, &pl)
	return toPlaylist(pl), err
}

// AddToPlaylist writes the playlist back with the track appended (the API
// sets a playlist's whole track list).
func (p *Provider) AddToPlaylist(ctx context.Context, playlistID, trackID string) error {
	if playlistID == likesID {
		return p.LoveSong(ctx, trackID, true)
	}
	ts, err := all[scTrack](ctx, p.c, "/playlists/"+playlistID+"/tracks", nil, 5000)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(ts)+1)
	for _, t := range ts {
		ids = append(ids, t.urn())
	}
	ids = append(ids, trackID)
	return p.c.call(ctx, http.MethodPut, "/playlists/"+playlistID, nil,
		map[string]any{"playlist": map[string]any{"tracks": trackRefs(ids)}}, nil)
}

func (p *Provider) LoveSong(ctx context.Context, id string, loved bool) error {
	method := http.MethodPost
	if !loved {
		method = http.MethodDelete
	}
	if err := p.c.call(ctx, method, "/likes/tracks/"+id, nil, nil, nil); err != nil {
		return err
	}
	p.mu.Lock()
	if p.liked != nil {
		p.liked[id] = loved
	}
	p.mu.Unlock()
	return nil
}

func (p *Provider) GetSongRating(ctx context.Context, id string) (bool, error) {
	p.mu.Lock()
	loaded := p.liked != nil
	p.mu.Unlock()
	if !loaded {
		if _, err := p.GetLibraryTracks(ctx); err != nil {
			return false, err
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.liked[id], nil
}

func (p *Provider) related(ctx context.Context, urn string) ([]provider.Track, error) {
	q := url.Values{}
	q.Set("limit", "30")
	q.Set("access", "playable")
	q.Set("linked_partitioning", "true")
	var pg page[scTrack]
	if err := p.c.get(ctx, "/tracks/"+urn+"/related", q, &pg); err != nil {
		return nil, err
	}
	return p.toTracks(pg.Collection), nil
}

// GetStationTracks returns tracks related to the seed.
func (p *Provider) GetStationTracks(ctx context.Context, seed string) ([]provider.Track, error) {
	return p.related(ctx, seed)
}

// GetRecommendations offers tracks related to recent likes, and the
// user's playlists.
func (p *Provider) GetRecommendations(ctx context.Context) ([]provider.RecommendationGroup, error) {
	likes, err := p.GetLibraryTracks(ctx)
	if err != nil {
		return nil, err
	}
	more := provider.RecommendationGroup{Title: i18n.T("More like your likes")}
	for _, t := range likes {
		if len(more.Items) == 12 {
			break
		}
		more.Items = append(more.Items, provider.RecommendationItem{ID: relatedPrefix + t.ID, Kind: "playlist",
			Title: i18n.T("Like: ") + t.Title, Subtitle: t.Artist})
	}
	groups := []provider.RecommendationGroup{more}
	if pls, err := p.GetLibraryPlaylists(ctx); err == nil && len(pls) > 1 {
		g := provider.RecommendationGroup{Title: i18n.T("Your playlists")}
		for _, pl := range pls[1:] {
			g.Items = append(g.Items, provider.RecommendationItem{ID: pl.ID, Kind: "playlist", Title: pl.Name})
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (p *Provider) track(ctx context.Context, id string) (scTrack, error) {
	p.mu.Lock()
	t, ok := p.cache[id]
	p.mu.Unlock()
	if ok {
		return t, nil
	}
	if err := p.c.get(ctx, "/tracks/"+id, nil, &t); err != nil {
		return scTrack{}, err
	}
	p.toTrack(t)
	return t, nil
}

// Lookup returns a track's metadata.
func (p *Provider) Lookup(ctx context.Context, id string) (provider.Track, error) {
	t, err := p.track(ctx, id)
	if err != nil {
		return provider.Track{ID: id, CatalogID: id}, err
	}
	return p.toTrack(t), nil
}

// GetSongDetails implements provider.SongDetailer.
func (p *Provider) GetSongDetails(ctx context.Context, tr provider.Track) (provider.SongDetails, error) {
	t, err := p.track(ctx, tr.ID)
	if err != nil {
		return provider.SongDetails{}, err
	}
	d := provider.SongDetails{URL: t.PermalinkURL}
	if u := t.User.urn(); u != "" {
		d.Artists = []provider.Artist{{ID: u, Name: provider.Clean(t.User.Username)}}
	}
	return d, nil
}

// errNoStream is returned for tracks SoundCloud won't stream to this app.
var errNoStream = errors.New("soundcloud: this track can't be streamed")

var (
	_ provider.Provider     = (*Provider)(nil)
	_ provider.SongDetailer = (*Provider)(nil)
)

// RemoveFromPlaylist writes the playlist back without the song (the API
// sets a playlist's whole track list).
func (p *Provider) RemoveFromPlaylist(ctx context.Context, playlistID string, index int, t provider.Track) error {
	if playlistID == likesID {
		return p.LoveSong(ctx, t.ID, false)
	}
	ts, err := all[scTrack](ctx, p.c, "/playlists/"+playlistID+"/tracks", nil, 5000)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(ts))
	for _, x := range ts {
		ids = append(ids, x.urn())
	}
	var at int
	if index >= 0 && index < len(ids) && ids[index] == t.ID {
		at = index
	} else {
		at = slices.Index(ids, t.ID)
	}
	if at < 0 {
		return errors.New("the song is not in this playlist")
	}
	ids = slices.Delete(ids, at, at+1)
	return p.c.call(ctx, http.MethodPut, "/playlists/"+playlistID, nil,
		map[string]any{"playlist": map[string]any{"tracks": trackRefs(ids)}}, nil)
}

func (p *Provider) RenamePlaylist(ctx context.Context, playlistID, name string) error {
	if playlistID == likesID {
		return errors.New("this playlist can't be renamed")
	}
	return p.c.call(ctx, http.MethodPut, "/playlists/"+playlistID, nil,
		map[string]any{"playlist": map[string]any{"title": name}}, nil)
}

func (p *Provider) DeletePlaylist(ctx context.Context, playlistID string) error {
	if playlistID == likesID {
		return errors.New("this playlist can't be deleted")
	}
	return p.c.call(ctx, http.MethodDelete, "/playlists/"+playlistID, nil, nil, nil)
}

var _ provider.PlaylistEditor = (*Provider)(nil)

// GetSavedAlbums returns the liked albums (liked sets that are albums,
// EPs, singles or compilations).
func (p *Provider) GetSavedAlbums(ctx context.Context) ([]provider.Album, error) {
	q := url.Values{}
	q.Set("limit", "50")
	q.Set("show_tracks", "false")
	pls, err := all[scPlaylist](ctx, p.c, "/me/likes/playlists", q, 1000)
	if err != nil && len(pls) == 0 {
		return nil, err
	}
	var out []provider.Album
	for _, pl := range pls {
		if pl.isAlbum() {
			out = append(out, toAlbum(pl))
		}
	}
	return out, nil
}

// GetSavedArtists returns the users the user follows.
func (p *Provider) GetSavedArtists(ctx context.Context) ([]provider.Artist, error) {
	q := url.Values{}
	q.Set("limit", "200")
	us, err := all[scUser](ctx, p.c, "/me/followings", q, 2000)
	if err != nil && len(us) == 0 {
		return nil, err
	}
	out := make([]provider.Artist, 0, len(us))
	for _, u := range us {
		if u.urn() != "" {
			out = append(out, provider.Artist{ID: u.urn(), Name: provider.Clean(u.Username)})
		}
	}
	return out, nil
}

// SaveAlbum likes or unlikes the album (a set).
func (p *Provider) SaveAlbum(ctx context.Context, a provider.Album, save bool) error {
	method := http.MethodPost
	if !save {
		method = http.MethodDelete
	}
	return p.c.call(ctx, method, "/likes/playlists/"+a.ID, nil, nil, nil)
}

// FollowArtist follows or unfollows the user.
func (p *Provider) FollowArtist(ctx context.Context, a provider.Artist, follow bool) error {
	method := http.MethodPut
	if !follow {
		method = http.MethodDelete
	}
	return p.c.call(ctx, method, "/me/followings/"+a.ID, nil, nil, nil)
}

var (
	_ provider.LibraryCollections = (*Provider)(nil)
	_ provider.CollectionSaver    = (*Provider)(nil)
)

// ResolveLink implements provider.LinkResolver: the API knows tracks,
// sets and users by number, links name them by permalink.
func (p *Provider) ResolveLink(ctx context.Context, l provider.Link) (*provider.SearchResult, error) {
	page := l.URL
	if !strings.HasPrefix(l.ID, "http") {
		page = pageURL(l.ID)
	}
	q := url.Values{}
	q.Set("url", page)
	var raw json.RawMessage
	if err := p.c.get(ctx, "/resolve", q, &raw); err != nil {
		return nil, err
	}
	var head struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, err
	}
	switch head.Kind {
	case "track":
		var t scTrack
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, err
		}
		if !t.playable() {
			return nil, errors.New(i18n.T("this track can't be played (SoundCloud Go+ or blocked)"))
		}
		return &provider.SearchResult{Tracks: []provider.Track{p.toTrack(t)}}, nil
	case "playlist":
		var pl scPlaylist
		if err := json.Unmarshal(raw, &pl); err != nil {
			return nil, err
		}
		ts, err := p.GetPlaylistTracks(ctx, pl.urn())
		if err != nil {
			return nil, err
		}
		if pl.isAlbum() {
			return &provider.SearchResult{Albums: []provider.Album{toAlbum(pl)}, Tracks: ts}, nil
		}
		return &provider.SearchResult{Playlists: []provider.Playlist{toPlaylist(pl)}, Tracks: ts}, nil
	case "user":
		var u scUser
		if err := json.Unmarshal(raw, &u); err != nil {
			return nil, err
		}
		return &provider.SearchResult{Artists: []provider.Artist{{ID: u.urn(), Name: provider.Clean(u.Username)}}}, nil
	}
	return nil, fmt.Errorf("soundcloud: unsupported link (%s)", head.Kind)
}

var _ provider.LinkResolver = (*Provider)(nil)
