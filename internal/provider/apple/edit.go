package apple

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tphx37/utuimp/internal/provider"
)

// Library playlists can only be changed through amp-api, the host of the
// Apple Music web player: the public API adds songs and playlists but never
// takes anything away or renames.

// ampRequest builds a request against amp-api, as the web player sends it.
func (a *AppleProvider) ampRequest(ctx context.Context, method, endpoint string, body []byte) (*http.Request, error) {
	u, err := resolveURL(a.catalogBaseURL, endpoint)
	if err != nil {
		return nil, err
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd) //nolint:gosec // G107: URL is validated by resolveURL against configured catalogBaseURL
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.AppleDeveloperToken)
	req.Header.Set("Music-User-Token", a.cfg.AppleUserToken)
	req.Header.Set("Origin", "https://music.apple.com")
	return req, nil
}

// AddToLibrary adds the song to the user's library (not to the favourites).
// A library song ("i.…") is in it already.
func (a *AppleProvider) AddToLibrary(ctx context.Context, t provider.Track) error {
	id := t.CatalogID
	if id == "" || strings.HasPrefix(id, "i.") {
		id = t.ID
	}
	if strings.HasPrefix(id, "i.") {
		return nil
	}
	if id == "" {
		return errors.New("this song has no Apple Music ID")
	}
	req, err := a.newRequest(ctx, http.MethodPost, "/me/library?ids[songs]="+url.QueryEscape(id))
	if err != nil {
		return err
	}
	return a.do(req, nil)
}

// RemoveFromPlaylist takes the song out of a library playlist. Only songs
// listed from the playlist itself carry the library ID this needs.
func (a *AppleProvider) RemoveFromPlaylist(ctx context.Context, playlistID string, _ int, t provider.Track) error {
	if playlistID == favoritesPlaylistID {
		id := t.CatalogID
		if id == "" {
			id = t.ID
		}
		return a.LoveSong(ctx, id, false)
	}
	if !strings.HasPrefix(t.ID, "i.") {
		return errors.New("open the playlist to remove a song from it")
	}
	ep := "/me/library/playlists/" + url.PathEscape(playlistID) + "/tracks?ids[library-songs]=" + url.QueryEscape(t.ID) + "&mode=all"
	req, err := a.ampRequest(ctx, http.MethodDelete, ep, nil)
	if err != nil {
		return err
	}
	return a.do(req, nil)
}

// RenamePlaylist renames a library playlist.
func (a *AppleProvider) RenamePlaylist(ctx context.Context, playlistID, name string) error {
	if playlistID == favoritesPlaylistID {
		return errors.New("this playlist can't be renamed")
	}
	raw, err := json.Marshal(map[string]any{"attributes": map[string]string{"name": name}})
	if err != nil {
		return err
	}
	req, err := a.ampRequest(ctx, http.MethodPatch, "/me/library/playlists/"+url.PathEscape(playlistID), raw)
	if err != nil {
		return err
	}
	return a.do(req, nil)
}

// DeletePlaylist deletes a library playlist.
func (a *AppleProvider) DeletePlaylist(ctx context.Context, playlistID string) error {
	if playlistID == favoritesPlaylistID {
		return errors.New("this playlist can't be deleted")
	}
	req, err := a.ampRequest(ctx, http.MethodDelete, "/me/library/playlists/"+url.PathEscape(playlistID), nil)
	if err != nil {
		return err
	}
	return a.do(req, nil)
}

var (
	_ provider.PlaylistEditor = (*AppleProvider)(nil)
	_ provider.LibraryAdder   = (*AppleProvider)(nil)
)
