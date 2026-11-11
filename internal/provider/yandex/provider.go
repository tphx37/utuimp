package yandex

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
)

// likedPlaylistID is the synthetic "liked tracks" playlist.
const likedPlaylistID = provider.FavoritesPlaylistID

// Provider implements provider.Provider for Yandex Music.
type Provider struct {
	c   *Client
	uid string

	mu    sync.Mutex
	cache map[string]provider.Track // by full id and by numeric id
	liked map[string]bool           // numeric track ids

	stations map[string]*station // rotor sessions by seed
}

// station is a running rotor session and the tracks it has handed out.
type station struct {
	sid     string
	batchID string
	queue   []string
}

// New returns a provider for the signed-in account (uid from AccountUID).
func New(c *Client, uid string) *Provider {
	return &Provider{c: c, uid: uid, cache: map[string]provider.Track{}, liked: map[string]bool{}, stations: map[string]*station{}}
}

// Client exposes the API client (for streaming and lyrics).
func (p *Provider) Client() *Client { return p.c }

func (p *Provider) Name() string          { return "Yandex Music" }
func (p *Provider) IsAuthenticated() bool { return p.uid != "" }

// ── conversion ─────────────────────────────────────────────────────────────

func coverURL(uri string, size int) string {
	if uri == "" {
		return ""
	}
	return "https://" + strings.ReplaceAll(uri, "%%", fmt.Sprintf("%dx%d", size, size))
}

func artistNames(as []ymArtist) string {
	names := make([]string, 0, len(as))
	for _, a := range as {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return strings.Join(names, ", ")
}

func numericID(id string) string {
	n, _, _ := strings.Cut(id, ":")
	return n
}

func (p *Provider) toTrack(t ymTrack) provider.Track {
	id := t.idString()
	title := provider.Clean(t.Title)
	if v := strings.TrimSpace(t.Version); v != "" {
		title += " (" + provider.Clean(v) + ")"
	}
	tr := provider.Track{
		Title:      title,
		Artist:     provider.Clean(artistNames(t.Artists)),
		Duration:   time.Duration(t.DurationMs) * time.Millisecond,
		ArtworkURL: coverURL(t.CoverURI, 400),
	}
	if t.Explicit || t.ContentWarning == "explicit" {
		tr.ContentRating = "explicit"
	}
	if len(t.Albums) > 0 {
		al := t.Albums[0]
		id += ":" + al.ID.String()
		tr.Album = provider.Clean(al.Title)
		if al.Year > 0 {
			tr.ReleaseDate = strconv.Itoa(al.Year)
		}
		if al.Genre != "" {
			tr.Genres = []string{al.Genre}
		}
		if al.TrackPosition != nil {
			tr.DiscNumber, tr.TrackNumber = al.TrackPosition.Volume, al.TrackPosition.Index
		}
		if tr.ArtworkURL == "" {
			tr.ArtworkURL = coverURL(al.CoverURI, 400)
		}
	}
	tr.ID, tr.CatalogID = id, id
	p.mu.Lock()
	p.cache[id] = tr
	p.cache[numericID(id)] = tr
	p.mu.Unlock()
	return tr
}

func (p *Provider) toTracks(ts []ymTrack) []provider.Track {
	out := make([]provider.Track, 0, len(ts))
	for _, t := range ts {
		if t.Available != nil && !*t.Available {
			continue
		}
		out = append(out, p.toTrack(t))
	}
	return out
}

func toAlbum(a ymAlbum) provider.Album {
	title := provider.Clean(a.Title)
	if v := strings.TrimSpace(a.Version); v != "" {
		title += " (" + provider.Clean(v) + ")"
	}
	rd := a.ReleaseDate
	if len(rd) >= 10 {
		rd = rd[:10]
	} else if a.Year > 0 {
		rd = strconv.Itoa(a.Year)
	}
	return provider.Album{
		ID: a.ID.String(), Title: title, Artist: provider.Clean(artistNames(a.Artists)),
		ArtworkURL: coverURL(a.CoverURI, 400), TrackCount: a.TrackCount, ReleaseDate: rd,
	}
}

func (p *Provider) toPlaylist(pl ymPlaylist) provider.Playlist {
	uid := pl.Owner.UID.String()
	if uid == "" {
		uid = pl.UID.String()
	}
	art := ""
	if pl.Cover != nil {
		if pl.Cover.URI != "" {
			art = coverURL(pl.Cover.URI, 400)
		} else if len(pl.Cover.ItemsURIs) > 0 {
			art = coverURL(pl.Cover.ItemsURIs[0], 400)
		}
	}
	return provider.Playlist{ID: uid + ":" + pl.Kind.String(), Name: provider.Clean(pl.Title), TrackCount: pl.TrackCount, ArtworkURL: art}
}

// Lookup returns cached metadata for a track id (full or numeric), fetching
// it when unknown. The player uses it to describe what it plays.
func (p *Provider) Lookup(ctx context.Context, id string) (provider.Track, error) {
	p.mu.Lock()
	t, ok := p.cache[id]
	if !ok {
		t, ok = p.cache[numericID(id)]
	}
	p.mu.Unlock()
	if ok {
		return t, nil
	}
	ts, err := p.c.tracksByIDs(ctx, []string{id})
	if err != nil {
		return provider.Track{ID: id}, err
	}
	if len(ts) == 0 {
		return provider.Track{ID: id}, errors.New("track not found")
	}
	return p.toTrack(ts[0]), nil
}

// ── provider.Provider ──────────────────────────────────────────────────────

func (p *Provider) Search(ctx context.Context, query string) (*provider.SearchResult, error) {
	r, err := p.c.search(ctx, query)
	if err != nil {
		return nil, err
	}
	res := &provider.SearchResult{}
	if r.Tracks != nil {
		res.Tracks = p.toTracks(r.Tracks.Results)
	}
	if r.Albums != nil {
		for _, a := range r.Albums.Results {
			res.Albums = append(res.Albums, toAlbum(a))
		}
	}
	if r.Artists != nil {
		for _, a := range r.Artists.Results {
			res.Artists = append(res.Artists, provider.Artist{ID: a.ID.String(), Name: provider.Clean(a.Name)})
		}
	}
	if r.Playlists != nil {
		for _, pl := range r.Playlists.Results {
			res.Playlists = append(res.Playlists, p.toPlaylist(pl))
		}
	}
	return res, nil
}

// GetLibraryTracks returns liked tracks, most recently liked first.
func (p *Provider) GetLibraryTracks(ctx context.Context) ([]provider.Track, error) {
	ids, err := p.c.likedTrackIDs(ctx, p.uid)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.liked = make(map[string]bool, len(ids))
	for _, id := range ids {
		p.liked[numericID(id)] = true
	}
	p.mu.Unlock()
	ts, err := p.c.tracksByIDs(ctx, ids)
	if err != nil && len(ts) == 0 {
		return nil, err
	}
	return p.toTracks(ts), nil
}

func (p *Provider) GetLibraryPlaylists(ctx context.Context) ([]provider.Playlist, error) {
	pls, err := p.c.userPlaylists(ctx, p.uid)
	if err != nil {
		return nil, err
	}
	out := []provider.Playlist{{ID: likedPlaylistID, Name: "Мне нравится", ReadOnly: true}}
	for _, pl := range pls {
		out = append(out, p.toPlaylist(pl))
	}
	return out, nil
}

func (p *Provider) GetPlaylistTracks(ctx context.Context, playlistID string) ([]provider.Track, error) {
	if playlistID == likedPlaylistID {
		return p.GetLibraryTracks(ctx)
	}
	if playlistID == waveID {
		return p.MyWave(ctx)
	}
	uid, kind, ok := strings.Cut(playlistID, ":")
	if !ok {
		uid, kind = p.uid, playlistID
	}
	pl, err := p.c.playlist(ctx, uid, kind)
	if err != nil {
		return nil, err
	}
	var full []ymTrack
	var missing []string
	for _, ts := range pl.Tracks {
		if ts.Track != nil {
			full = append(full, *ts.Track)
			continue
		}
		id := rawID(ts.ID)
		if a := rawID(ts.AlbumID); a != "" && a != "null" {
			id += ":" + a
		}
		missing = append(missing, id)
	}
	if len(missing) > 0 {
		more, err := p.c.tracksByIDs(ctx, missing)
		if err != nil && len(full) == 0 {
			return nil, err
		}
		full = append(full, more...)
	}
	return p.toTracks(full), nil
}

func (p *Provider) GetCatalogPlaylistTracks(ctx context.Context, playlistID string) ([]provider.Track, error) {
	return p.GetPlaylistTracks(ctx, playlistID)
}

func (p *Provider) GetAlbumTracks(ctx context.Context, albumID string) ([]provider.Track, error) {
	al, err := p.c.albumWithTracks(ctx, albumID)
	if err != nil {
		return nil, err
	}
	var out []provider.Track
	for disc, vol := range al.Volumes {
		for i, t := range vol {
			if len(t.Albums) == 0 {
				t.Albums = []ymAlbum{{ID: al.ID, Title: al.Title, Year: al.Year, CoverURI: al.CoverURI, Genre: al.Genre}}
			}
			tr := p.toTrack(t)
			if tr.TrackNumber == 0 {
				tr.DiscNumber, tr.TrackNumber = disc+1, i+1
			}
			if t.Available == nil || *t.Available {
				out = append(out, tr)
			}
		}
	}
	return out, nil
}

func (p *Provider) GetLibraryAlbumTracks(ctx context.Context, albumID string) ([]provider.Track, error) {
	return p.GetAlbumTracks(ctx, albumID)
}

func (p *Provider) GetArtistAlbums(ctx context.Context, artistID string) ([]provider.Album, error) {
	as, err := p.c.artistAlbums(ctx, artistID)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Album, 0, len(as))
	for _, a := range as {
		out = append(out, toAlbum(a))
	}
	return out, nil
}

func (p *Provider) CreatePlaylist(ctx context.Context, name string, trackIDs []string) (provider.Playlist, error) {
	pl, err := p.c.createPlaylist(ctx, p.uid, name)
	if err != nil {
		return provider.Playlist{}, err
	}
	if len(trackIDs) > 0 {
		if err := p.c.insertTracks(ctx, p.uid, pl.Kind.String(), 0, pl.Revision, trackIDs); err != nil {
			return p.toPlaylist(*pl), err
		}
	}
	return p.toPlaylist(*pl), nil
}

func (p *Provider) AddToPlaylist(ctx context.Context, playlistID, trackID string) error {
	uid, kind, ok := strings.Cut(playlistID, ":")
	if !ok || playlistID == likedPlaylistID {
		return errors.New("cannot add to this playlist")
	}
	pl, err := p.c.playlist(ctx, uid, kind)
	if err != nil {
		return err
	}
	return p.c.insertTracks(ctx, uid, kind, pl.TrackCount, pl.Revision, []string{trackID})
}

func (p *Provider) LoveSong(ctx context.Context, trackID string, loved bool) error {
	if err := p.c.setLike(ctx, p.uid, trackID, loved); err != nil {
		return err
	}
	p.mu.Lock()
	p.liked[numericID(trackID)] = loved
	p.mu.Unlock()
	return nil
}

func (p *Provider) GetSongRating(ctx context.Context, trackID string) (bool, error) {
	p.mu.Lock()
	known := len(p.liked) > 0
	liked := p.liked[numericID(trackID)]
	p.mu.Unlock()
	if known {
		return liked, nil
	}
	ids, err := p.c.likedTrackIDs(ctx, p.uid)
	if err != nil {
		return false, err
	}
	p.mu.Lock()
	for _, id := range ids {
		p.liked[numericID(id)] = true
	}
	liked = p.liked[numericID(trackID)]
	p.mu.Unlock()
	return liked, nil
}

// waveID is the pseudo playlist that plays the user's "My Wave".
const waveID = "wave:onyourwave"

// GetRecommendations offers "My Wave" plus the user's own playlists.
func (p *Provider) GetRecommendations(ctx context.Context) ([]provider.RecommendationGroup, error) {
	groups := []provider.RecommendationGroup{{
		Title: "Для вас",
		Items: []provider.RecommendationItem{{ID: waveID, Kind: "playlist", Title: "Моя волна", Subtitle: "Яндекс Музыка"}},
	}}
	if pls, err := p.c.userPlaylists(ctx, p.uid); err == nil && len(pls) > 0 {
		g := provider.RecommendationGroup{Title: "Ваши плейлисты"}
		for _, pl := range pls {
			ppl := p.toPlaylist(pl)
			g.Items = append(g.Items, provider.RecommendationItem{ID: ppl.ID, Kind: "playlist", Title: ppl.Name, Subtitle: fmt.Sprintf("%d треков", ppl.TrackCount)})
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// IsWave reports whether a seed is the user's "My Wave".
func IsWave(seed string) bool { return strings.HasPrefix(seed, "wave:") }

// GetStationTracks returns the next batch of a station: "My Wave" for a
// "wave:" seed, else radio seeded by a track. The rotor session is kept, so
// each call continues the same station rather than starting a new one.
func (p *Provider) GetStationTracks(ctx context.Context, seed string) ([]provider.Track, error) {
	rotorSeed := "track:" + numericID(seed)
	if IsWave(seed) {
		rotorSeed = "user:onyourwave"
	}
	p.mu.Lock()
	st := p.stations[rotorSeed]
	p.mu.Unlock()

	var batch rotorBatch
	var err error
	if st != nil {
		batch, err = p.c.rotorNext(ctx, st.sid, lastN(st.queue, 30))
	}
	if st == nil || err != nil || len(batch.Tracks) == 0 {
		// New (or expired) session.
		batch, err = p.c.rotorNew(ctx, []string{rotorSeed})
		if err != nil {
			return nil, err
		}
		st = &station{sid: batch.SessionID}
		if st.sid != "" {
			_ = p.c.rotorFeedback(ctx, st.sid, batch.BatchID, "radioStarted", "", 0)
		}
	}
	st.batchID = batch.BatchID
	tracks := p.toTracks(batch.Tracks)
	for _, t := range tracks {
		st.queue = append(st.queue, t.ID)
	}
	p.mu.Lock()
	if st.sid != "" {
		p.stations[rotorSeed] = st
	}
	p.mu.Unlock()
	return tracks, nil
}

// ResetStation forgets a station's session so the next call starts fresh
// (picking "My Wave" again gives a new wave).
func (p *Provider) ResetStation(seed string) {
	key := "track:" + numericID(seed)
	if IsWave(seed) {
		key = "user:onyourwave"
	}
	p.mu.Lock()
	delete(p.stations, key)
	p.mu.Unlock()
}

// RadioTrackStarted tells the station seeded by seed that a track began
// playing, so it learns what was listened to.
func (p *Provider) RadioTrackStarted(ctx context.Context, seed, trackID string) error {
	return p.stationFeedback(ctx, seed, "trackStarted", trackID, 0)
}

// RadioTrackEnded tells the station that a track ended after played:
// listened to the end (finished), or skipped. A skip teaches the station
// to send fewer songs like it.
func (p *Provider) RadioTrackEnded(ctx context.Context, seed, trackID string, played time.Duration, finished bool) error {
	event := "skip"
	if finished {
		event = "trackFinished"
	}
	return p.stationFeedback(ctx, seed, event, trackID, played)
}

func (p *Provider) stationFeedback(ctx context.Context, seed, event, trackID string, played time.Duration) error {
	key := "track:" + numericID(seed)
	if IsWave(seed) {
		key = "user:onyourwave"
	}
	p.mu.Lock()
	st := p.stations[key]
	p.mu.Unlock()
	if st == nil {
		return nil // no session (yet): nothing to tell
	}
	return p.c.rotorFeedback(ctx, st.sid, st.batchID, event, trackID, played)
}

func lastN(s []string, n int) []string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// MyWave returns the first batch of a fresh "My Wave".
func (p *Provider) MyWave(ctx context.Context) ([]provider.Track, error) {
	p.ResetStation(waveID)
	return p.GetStationTracks(ctx, waveID)
}

// GetSongDetails implements provider.SongDetailer.
func (p *Provider) GetSongDetails(ctx context.Context, t provider.Track) (provider.SongDetails, error) {
	ts, err := p.c.tracksByIDs(ctx, []string{t.ID})
	if err != nil {
		return provider.SongDetails{}, err
	}
	if len(ts) == 0 {
		return provider.SongDetails{}, errors.New("track not found")
	}
	yt := ts[0]
	var d provider.SongDetails
	for _, a := range yt.Artists {
		d.Artists = append(d.Artists, provider.Artist{ID: a.ID.String(), Name: provider.Clean(a.Name)})
	}
	if len(yt.Albums) > 0 {
		al := toAlbum(yt.Albums[0])
		d.Album = &al
		d.URL = fmt.Sprintf("https://music.yandex.ru/album/%s/track/%s", al.ID, yt.idString())
	} else {
		d.URL = "https://music.yandex.ru/track/" + yt.idString()
	}
	return d, nil
}

var (
	_ provider.Provider     = (*Provider)(nil)
	_ provider.SongDetailer = (*Provider)(nil)
)

// StreamURL returns a fresh direct link to the track's audio and its codec.
func (p *Provider) StreamURL(ctx context.Context, id string) (string, string, error) {
	return p.c.StreamURL(ctx, id)
}

// TrackLyrics returns the track's lyrics from Yandex Music: LRC when synced.
func (p *Provider) TrackLyrics(ctx context.Context, trackID string, synced bool) (string, error) {
	return p.c.Lyrics(ctx, trackID, synced)
}

// ownPlaylist splits a playlist ID of the user's own into uid and kind.
func (p *Provider) ownPlaylist(playlistID string) (uid, kind string, err error) {
	uid, kind, ok := strings.Cut(playlistID, ":")
	if !ok || playlistID == likedPlaylistID || playlistID == waveID {
		return "", "", errors.New("this playlist can't be changed")
	}
	return uid, kind, nil
}

// RemoveFromPlaylist takes the song out of the playlist: from its place
// there (looked up again, as the list utuimp shows may be ordered
// otherwise), with the playlist's current revision.
func (p *Provider) RemoveFromPlaylist(ctx context.Context, playlistID string, _ int, t provider.Track) error {
	if playlistID == likedPlaylistID {
		return p.LoveSong(ctx, t.ID, false)
	}
	uid, kind, err := p.ownPlaylist(playlistID)
	if err != nil {
		return err
	}
	pl, err := p.c.playlist(ctx, uid, kind)
	if err != nil {
		return err
	}
	want := numericID(t.ID)
	for i, ts := range pl.Tracks {
		id := rawID(ts.ID)
		if ts.Track != nil {
			id = ts.Track.idString()
		}
		if numericID(id) == want {
			return p.c.deleteTrack(ctx, uid, kind, i, pl.Revision)
		}
	}
	return errors.New("the song is not in this playlist")
}

func (p *Provider) RenamePlaylist(ctx context.Context, playlistID, name string) error {
	uid, kind, err := p.ownPlaylist(playlistID)
	if err != nil {
		return err
	}
	return p.c.renamePlaylist(ctx, uid, kind, name)
}

func (p *Provider) DeletePlaylist(ctx context.Context, playlistID string) error {
	uid, kind, err := p.ownPlaylist(playlistID)
	if err != nil {
		return err
	}
	return p.c.deletePlaylist(ctx, uid, kind)
}

var _ provider.PlaylistEditor = (*Provider)(nil)

// GetSavedAlbums returns the liked albums.
func (p *Provider) GetSavedAlbums(ctx context.Context) ([]provider.Album, error) {
	as, err := p.c.likedAlbums(ctx, p.uid)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Album, 0, len(as))
	for _, a := range as {
		out = append(out, toAlbum(a))
	}
	return out, nil
}

// GetSavedArtists returns the liked artists.
func (p *Provider) GetSavedArtists(ctx context.Context) ([]provider.Artist, error) {
	as, err := p.c.likedArtists(ctx, p.uid)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Artist, 0, len(as))
	for _, a := range as {
		out = append(out, provider.Artist{ID: a.ID.String(), Name: provider.Clean(a.Name)})
	}
	return out, nil
}

func (p *Provider) SaveAlbum(ctx context.Context, a provider.Album, save bool) error {
	return p.c.setLikeOf(ctx, p.uid, "album", a.ID, save)
}

func (p *Provider) FollowArtist(ctx context.Context, a provider.Artist, follow bool) error {
	return p.c.setLikeOf(ctx, p.uid, "artist", a.ID, follow)
}

var (
	_ provider.LibraryCollections = (*Provider)(nil)
	_ provider.CollectionSaver    = (*Provider)(nil)
)
