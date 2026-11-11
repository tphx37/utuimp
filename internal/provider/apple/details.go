package apple

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/tphx37/utuimp/internal/provider"
)

type namedResource struct {
	ID         string `json:"id"`
	Attributes struct {
		Name       string       `json:"name"`
		ArtistName string       `json:"artistName"`
		Artwork    artworkAttrs `json:"artwork"`
	} `json:"attributes"`
}

type songDetailsResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			URL        string `json:"url"`
			ArtistName string `json:"artistName"`
			Name       string `json:"name"`
		} `json:"attributes"`
		Relationships struct {
			Albums struct {
				Data []namedResource `json:"data"`
			} `json:"albums"`
			Artists struct {
				Data []namedResource `json:"data"`
			} `json:"artists"`
		} `json:"relationships"`
	} `json:"data"`
}

// GetSongDetails looks up a track's album, every credited artist (including
// ones only named in "A & B" or "(feat. C)"), and its music.apple.com link.
func (a *AppleProvider) GetSongDetails(ctx context.Context, t provider.Track) (provider.SongDetails, error) {
	var out provider.SongDetails
	catalogID := t.CatalogID
	if catalogID == "" && !strings.HasPrefix(t.ID, "i.") {
		catalogID = t.ID
	}

	var endpoint string
	if catalogID != "" {
		sf, err := a.storefront(ctx)
		if err != nil {
			return out, err
		}
		endpoint = fmt.Sprintf("/catalog/%s/songs/%s?include=albums,artists", sf, url.PathEscape(catalogID))
	} else if strings.HasPrefix(t.ID, "i.") {
		// A library-only song (uploaded or matched): its album is a library
		// album, and it has no public link.
		endpoint = "/me/library/songs/" + url.PathEscape(t.ID) + "?include=albums"
		out.LibraryAlbum = true
	} else {
		return out, errors.New("track has no ID")
	}

	req, err := a.newRequest(ctx, http.MethodGet, endpoint)
	if err != nil {
		return out, err
	}
	var resp songDetailsResponse
	if err := a.do(req, &resp); err != nil {
		return out, err
	}
	if len(resp.Data) == 0 {
		return out, errors.New("song not found")
	}
	song := resp.Data[0]
	out.URL = song.Attributes.URL
	if albums := song.Relationships.Albums.Data; len(albums) > 0 {
		al := albums[0]
		out.Album = &provider.Album{
			ID:         al.ID,
			Title:      provider.Clean(al.Attributes.Name),
			Artist:     provider.Clean(al.Attributes.ArtistName),
			ArtworkURL: al.Attributes.Artwork.formatted(300),
		}
	}
	if out.LibraryAlbum {
		return out, nil
	}

	seen := map[string]bool{}
	for _, ar := range song.Relationships.Artists.Data {
		name := provider.Clean(ar.Attributes.Name)
		out.Artists = append(out.Artists, provider.Artist{ID: ar.ID, Name: name})
		seen[strings.ToLower(name)] = true
	}
	// Apple lists only the main artist(s); find the others by name.
	artistName := song.Attributes.ArtistName
	if artistName == "" {
		artistName = t.Artist
	}
	title := song.Attributes.Name
	if title == "" {
		title = t.Title
	}
	for _, name := range creditedArtists(artistName, title) {
		if seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		if ar, err := a.findArtist(ctx, name); err == nil {
			out.Artists = append(out.Artists, ar)
		}
	}
	return out, nil
}

var (
	featRE      = regexp.MustCompile(`(?i)[(\[]\s*(?:feat\.?|ft\.?|featuring|with)\s+([^)\]]+)[)\]]`)
	artistSplit = regexp.MustCompile(`\s*(?:,|&|\band\b|\bx\b|\bvs\.?)\s*`)
)

// creditedArtists splits "A, B & C" and "Title (feat. D & E)" into names.
func creditedArtists(artistName, title string) []string {
	var names []string
	add := func(s string) {
		for _, n := range artistSplit.Split(s, -1) {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
	}
	add(artistName)
	for _, m := range featRE.FindAllStringSubmatch(title, -1) {
		add(m[1])
	}
	return names
}

// findArtist returns the catalog artist whose name matches exactly.
func (a *AppleProvider) findArtist(ctx context.Context, name string) (provider.Artist, error) {
	sf, err := a.storefront(ctx)
	if err != nil {
		return provider.Artist{}, err
	}
	q := url.Values{}
	q.Set("term", name)
	q.Set("types", "artists")
	q.Set("limit", "5")
	req, err := a.newRequest(ctx, http.MethodGet, fmt.Sprintf("/catalog/%s/search?%s", sf, q.Encode()))
	if err != nil {
		return provider.Artist{}, err
	}
	var resp struct {
		Results struct {
			Artists struct {
				Data []namedResource `json:"data"`
			} `json:"artists"`
		} `json:"results"`
	}
	if err := a.do(req, &resp); err != nil {
		return provider.Artist{}, err
	}
	for _, ar := range resp.Results.Artists.Data {
		if strings.EqualFold(strings.TrimSpace(ar.Attributes.Name), name) {
			return provider.Artist{ID: ar.ID, Name: provider.Clean(ar.Attributes.Name)}, nil
		}
	}
	return provider.Artist{}, fmt.Errorf("artist %q not found", name)
}

// Lookup implements provider.TrackLookup for a catalog song ID.
func (a *AppleProvider) Lookup(ctx context.Context, id string) (provider.Track, error) {
	ts, err := a.GetCatalogTracks(ctx, []string{id})
	if err != nil {
		return provider.Track{ID: id}, err
	}
	if len(ts) == 0 {
		return provider.Track{ID: id}, errors.New("song not found")
	}
	return ts[0], nil
}

var _ provider.TrackLookup = (*AppleProvider)(nil)
