// Package radio provides internet radio discovery and locally saved stations.
package radio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/localstore"
)

// Provider uses Radio Browser for discovery and a local store for stations.
type Provider struct {
	provider.NotReady
	store    *localstore.Store
	client   *http.Client
	endpoint string
	mu       sync.RWMutex
	tracks   map[string]provider.Track
}

func New(store *localstore.Store) *Provider {
	p := &Provider{store: store, client: &http.Client{Timeout: 12 * time.Second}, endpoint: "https://de1.api.radio-browser.info", tracks: map[string]provider.Track{}}
	for _, pl := range store.Playlists() {
		for _, t := range pl.Tracks {
			p.tracks[t.ID] = t
		}
	}
	for _, t := range store.Favorites() {
		p.tracks[t.ID] = t
	}
	return p
}
func (*Provider) Name() string          { return "Radio" }
func (*Provider) IsAuthenticated() bool { return true }
func (p *Provider) RememberTrack(t provider.Track) {
	if streamURL(t.ID) {
		p.remember(t)
	}
}
func (p *Provider) remember(t provider.Track) {
	for _, saved := range p.store.Favorites() {
		if saved.ID == t.ID {
			t.Title = saved.Title
			break
		}
	}
	p.mu.Lock()
	p.tracks[t.ID] = t
	p.mu.Unlock()
}
func (p *Provider) Lookup(id string) (provider.Track, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	t, ok := p.tracks[id]
	return t, ok
}
func streamURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
func (p *Provider) Search(ctx context.Context, q string) (*provider.SearchResult, error) {
	q = strings.TrimSpace(q)
	if streamURL(q) {
		t := p.linkMetadata(ctx, q)
		p.remember(t)
		t, _ = p.Lookup(t.ID)
		return &provider.SearchResult{Tracks: []provider.Track{t}}, nil
	}
	v := url.Values{"name": {q}, "limit": {"50"}, "hidebroken": {"true"}, "order": {"clickcount"}, "reverse": {"true"}}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/json/stations/search?"+v.Encode(), nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "utuimp/1.0 (https://github.com/tphx37/utuimp)")
	resp, e := p.client.Do(req)
	if e != nil {
		return nil, e
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("radio browser: HTTP %d", resp.StatusCode)
	}
	var stations []struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Resolved string `json:"url_resolved"`
		Country  string `json:"country"`
		Tags     string `json:"tags"`
		Favicon  string `json:"favicon"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&stations); e != nil {
		return nil, e
	}
	out := &provider.SearchResult{}
	seen := map[string]bool{}
	for _, s := range stations {
		link := s.Resolved
		if !streamURL(link) {
			link = s.URL
		}
		if !streamURL(link) || seen[link] {
			continue
		}
		seen[link] = true
		t := provider.Track{ID: link, CatalogID: link, Title: provider.Clean(s.Name), ArtworkURL: s.Favicon}
		p.remember(t)
		t, _ = p.Lookup(t.ID)
		out.Tracks = append(out.Tracks, t)
	}
	return out, nil
}
func (p *Provider) GetLibraryTracks(context.Context) ([]provider.Track, error) {
	return p.store.Favorites(), nil
}
func (p *Provider) GetLibraryPlaylists(context.Context) ([]provider.Playlist, error) {
	out := []provider.Playlist{{ID: provider.FavoritesPlaylistID, Name: "Favorites", TrackCount: len(p.store.Favorites())}}
	for _, pl := range p.store.Playlists() {
		out = append(out, provider.Playlist{ID: pl.ID, Name: pl.Name, TrackCount: len(pl.Tracks)})
	}
	return out, nil
}
func (p *Provider) GetPlaylistTracks(_ context.Context, id string) ([]provider.Track, error) {
	if id == provider.FavoritesPlaylistID {
		return p.store.Favorites(), nil
	}
	pl, ok := p.store.Playlist(id)
	if !ok {
		return nil, fmt.Errorf("playlist not found")
	}
	return pl.Tracks, nil
}
func (p *Provider) GetCatalogPlaylistTracks(ctx context.Context, id string) ([]provider.Track, error) {
	return p.GetPlaylistTracks(ctx, id)
}
func (p *Provider) LoveSong(_ context.Context, id string, on bool) error {
	t, ok := p.Lookup(id)
	if !ok {
		return fmt.Errorf("station not found")
	}
	return p.store.SetFavorite(t, on)
}
func (p *Provider) GetSongRating(_ context.Context, id string) (bool, error) {
	return p.store.IsFavorite(id), nil
}
func (p *Provider) CreatePlaylist(_ context.Context, name string, ids []string) (provider.Playlist, error) {
	var ts []provider.Track
	for _, id := range ids {
		if t, ok := p.Lookup(id); ok {
			ts = append(ts, t)
		}
	}
	pl, e := p.store.CreatePlaylist(name, ts)
	return provider.Playlist{ID: pl.ID, Name: pl.Name, TrackCount: len(pl.Tracks)}, e
}
func (p *Provider) AddToPlaylist(_ context.Context, id, track string) error {
	t, ok := p.Lookup(track)
	if !ok {
		return fmt.Errorf("station not found")
	}
	return p.store.AddToPlaylist(id, t)
}
func (p *Provider) RemoveFromPlaylist(_ context.Context, id string, index int, t provider.Track) error {
	return p.store.RemoveFromPlaylist(id, index, t.ID)
}
func (p *Provider) RenamePlaylist(_ context.Context, id, name string) error {
	return p.store.RenamePlaylist(id, name)
}
func (p *Provider) DeletePlaylist(_ context.Context, id string) error {
	return p.store.DeletePlaylist(id)
}
func (p *Provider) GetRecommendations(context.Context) ([]provider.RecommendationGroup, error) {
	return nil, nil
}

// ResolveLink lets :link offer playback and queue actions for a stream URL.
func (p *Provider) ResolveLink(ctx context.Context, link provider.Link) (*provider.SearchResult, error) {
	if !streamURL(link.URL) {
		return nil, fmt.Errorf("invalid station URL")
	}
	return p.Search(ctx, link.URL)
}

// RenameStation renames a saved station without changing its stream URL.
func (p *Provider) RenameStation(_ context.Context, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("station name cannot be empty")
	}
	if err := p.store.RenameFavorite(id, name); err != nil {
		return err
	}
	p.mu.Lock()
	if t, ok := p.tracks[id]; ok {
		t.Title = name
		p.tracks[id] = t
	}
	p.mu.Unlock()
	return nil
}
