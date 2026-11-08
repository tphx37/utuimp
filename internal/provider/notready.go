package provider

import (
	"context"
	"errors"
)

// ErrNotReady is returned while a service is still connecting.
var ErrNotReady = errors.New("still connecting — try again in a moment")

// NotReady stands in for a service's provider until its engine is up, so
// anything the UI does meanwhile (opening the feed or library, searching)
// gets an error instead of a nil-pointer crash.
type NotReady struct{}

func (NotReady) Name() string { return "" }
func (NotReady) Search(context.Context, string) (*SearchResult, error) {
	return nil, ErrNotReady
}
func (NotReady) GetLibraryTracks(context.Context) ([]Track, error)       { return nil, ErrNotReady }
func (NotReady) GetLibraryPlaylists(context.Context) ([]Playlist, error) { return nil, ErrNotReady }
func (NotReady) GetPlaylistTracks(context.Context, string) ([]Track, error) {
	return nil, ErrNotReady
}
func (NotReady) GetAlbumTracks(context.Context, string) ([]Track, error) { return nil, ErrNotReady }
func (NotReady) GetLibraryAlbumTracks(context.Context, string) ([]Track, error) {
	return nil, ErrNotReady
}
func (NotReady) GetArtistAlbums(context.Context, string) ([]Album, error) { return nil, ErrNotReady }
func (NotReady) GetCatalogPlaylistTracks(context.Context, string) ([]Track, error) {
	return nil, ErrNotReady
}
func (NotReady) CreatePlaylist(context.Context, string, []string) (Playlist, error) {
	return Playlist{}, ErrNotReady
}
func (NotReady) LoveSong(context.Context, string, bool) error        { return ErrNotReady }
func (NotReady) GetSongRating(context.Context, string) (bool, error) { return false, ErrNotReady }
func (NotReady) GetRecommendations(context.Context) ([]RecommendationGroup, error) {
	return nil, ErrNotReady
}
func (NotReady) GetStationTracks(context.Context, string) ([]Track, error) { return nil, ErrNotReady }
func (NotReady) AddToPlaylist(context.Context, string, string) error       { return ErrNotReady }
func (NotReady) IsAuthenticated() bool                                     { return false }

var _ Provider = NotReady{}
