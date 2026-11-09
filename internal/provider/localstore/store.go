// Package localstore keeps favourites and playlists on disk for services
// whose account utuimp can't write to (YouTube Music through yt-dlp).
package localstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/tphx37/utuimp/internal/provider"
)

// PlaylistPrefix starts the ID of every playlist kept here.
const PlaylistPrefix = "utuimp:pl:"

// Playlist is a saved playlist.
type Playlist struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Tracks []provider.Track `json:"tracks"`
}

type data struct {
	Favorites []provider.Track `json:"favorites"`
	Playlists []Playlist       `json:"playlists"`
}

// Store is a JSON file of favourites and playlists. It is safe for
// concurrent use.
type Store struct {
	path string
	mu   sync.Mutex
	d    data
}

// Open loads the store at path (a missing file is an empty store).
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.d); err != nil {
		return nil, err
	}
	return s, nil
}

// DefaultPath is ~/.config/utuimp/<name>.json.
func DefaultPath(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "utuimp", name+".json")
}

func (s *Store) save() error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.d, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Favorites returns the favourite songs, most recent first.
func (s *Store) Favorites() []provider.Track {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.d.Favorites)
}

// IsFavorite reports whether the song with id is a favourite.
func (s *Store) IsFavorite(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.ContainsFunc(s.d.Favorites, func(t provider.Track) bool { return t.ID == id })
}

// SetFavorite adds t to the favourites (on top) or removes it.
func (s *Store) SetFavorite(t provider.Track, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.Favorites = slices.DeleteFunc(s.d.Favorites, func(x provider.Track) bool { return x.ID == t.ID })
	if on {
		s.d.Favorites = append([]provider.Track{t}, s.d.Favorites...)
	}
	return s.save()
}

// Playlists returns the saved playlists.
func (s *Store) Playlists() []Playlist {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Playlist, len(s.d.Playlists))
	for i, p := range s.d.Playlists {
		out[i] = Playlist{ID: p.ID, Name: p.Name, Tracks: slices.Clone(p.Tracks)}
	}
	return out
}

// Playlist returns the playlist with id.
func (s *Store) Playlist(id string) (Playlist, bool) {
	for _, p := range s.Playlists() {
		if p.ID == id {
			return p, true
		}
	}
	return Playlist{}, false
}

// CreatePlaylist saves a new playlist.
func (s *Store) CreatePlaylist(name string, tracks []provider.Track) (Playlist, error) {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	p := Playlist{ID: PlaylistPrefix + hex.EncodeToString(b), Name: name, Tracks: slices.Clone(tracks)}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d.Playlists = append(s.d.Playlists, p)
	return p, s.save()
}

// AddToPlaylist appends t to the playlist with id.
func (s *Store) AddToPlaylist(id string, t provider.Track) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.d.Playlists {
		if s.d.Playlists[i].ID == id {
			s.d.Playlists[i].Tracks = append(s.d.Playlists[i].Tracks, t)
			return s.save()
		}
	}
	return errors.New("no such playlist")
}
