package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// LinkKind is what a share link points at.
type LinkKind string

const (
	LinkTrack    LinkKind = "track"
	LinkAlbum    LinkKind = "album"
	LinkPlaylist LinkKind = "playlist"
	LinkArtist   LinkKind = "artist"
)

// Link is a parsed share link of a music service.
type Link struct {
	// Service is the service's config key: "spotify", "apple", "yandex",
	// "youtube" or "soundcloud".
	Service string
	Kind    LinkKind
	// ID is in the form the service's provider takes it (for Yandex a
	// track is "track:album", a playlist "owner:kind"; for SoundCloud the
	// permalink path).
	ID  string
	URL string // the link as given
}

// LinkResolver is implemented by providers that resolve their links
// themselves rather than by ID (SoundCloud's API knows tracks by number,
// its links by name).
type LinkResolver interface {
	ResolveLink(ctx context.Context, l Link) (*SearchResult, error)
}

// TrackLookup is implemented by providers that fetch one track by ID.
type TrackLookup interface {
	Lookup(ctx context.Context, id string) (Track, error)
}

// ParseLink recognises a share link of one of the services. ok is false
// for anything else (a search query).
func ParseLink(raw string) (Link, bool) {
	s := strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(s, "spotify:"); ok {
		kind, id, _ := strings.Cut(rest, ":")
		if k, ok := spotifyKind(kind); ok && id != "" {
			return Link{Service: "spotify", Kind: k, ID: id, URL: s}, true
		}
		return Link{}, false
	}
	if strings.ContainsAny(s, " \t") {
		return Link{}, false
	}
	if !strings.Contains(s, "://") {
		// "open.spotify.com/track/…" pasted without the scheme.
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return Link{}, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	var parts []string
	for p := range strings.SplitSeq(u.Path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	l := Link{URL: strings.TrimSpace(raw)}
	switch {
	case host == "open.spotify.com" || host == "play.spotify.com":
		if len(parts) > 0 && strings.HasPrefix(parts[0], "intl-") {
			parts = parts[1:]
		}
		if len(parts) >= 2 {
			if k, ok := spotifyKind(parts[0]); ok {
				l.Service, l.Kind, l.ID = "spotify", k, parts[1]
			}
		}
	case host == "music.apple.com" || host == "itunes.apple.com":
		l = parseApple(l, parts, u.Query())
	case strings.HasPrefix(host, "music.yandex."):
		l = parseYandex(l, parts)
	case host == "youtu.be":
		if len(parts) >= 1 {
			l.Service, l.Kind, l.ID = "youtube", LinkTrack, parts[0]
		}
	case host == "youtube.com" || host == "music.youtube.com":
		l = parseYouTube(l, parts, u.Query())
	case host == "soundcloud.com" || host == "on.soundcloud.com":
		l = parseSoundCloud(l, host, parts)
	}
	if l.Service == "" || l.ID == "" {
		return Link{}, false
	}
	return l, true
}

func spotifyKind(s string) (LinkKind, bool) {
	switch s {
	case "track":
		return LinkTrack, true
	case "album":
		return LinkAlbum, true
	case "playlist":
		return LinkPlaylist, true
	case "artist":
		return LinkArtist, true
	}
	return "", false
}

// parseApple: /{cc}/song/{slug}/{id}, /{cc}/album/{slug}/{id}[?i={song}],
// /{cc}/playlist/{slug}/pl.…, /{cc}/artist/{slug}/{id}; the slug may be
// missing.
func parseApple(l Link, parts []string, q url.Values) Link {
	if len(parts) > 0 && len(parts[0]) == 2 {
		parts = parts[1:]
	}
	if len(parts) < 2 {
		return l
	}
	id := parts[len(parts)-1]
	l.Service = "apple"
	switch parts[0] {
	case "song":
		l.Kind, l.ID = LinkTrack, id
	case "album":
		if song := q.Get("i"); song != "" {
			l.Kind, l.ID = LinkTrack, song
		} else {
			l.Kind, l.ID = LinkAlbum, id
		}
	case "playlist":
		l.Kind, l.ID = LinkPlaylist, id
	case "artist":
		l.Kind, l.ID = LinkArtist, id
	default:
		l.Service = ""
	}
	return l
}

// parseYandex: /album/{a}/track/{t}, /track/{t}, /album/{a},
// /users/{owner}/playlists/{kind}, /artist/{id}.
func parseYandex(l Link, parts []string) Link {
	l.Service = "yandex"
	switch {
	case len(parts) >= 4 && parts[0] == "album" && parts[2] == "track":
		l.Kind, l.ID = LinkTrack, parts[3]+":"+parts[1]
	case len(parts) >= 2 && parts[0] == "track":
		l.Kind, l.ID = LinkTrack, parts[1]
	case len(parts) >= 2 && parts[0] == "album":
		l.Kind, l.ID = LinkAlbum, parts[1]
	case len(parts) >= 4 && parts[0] == "users" && parts[2] == "playlists":
		l.Kind, l.ID = LinkPlaylist, parts[1]+":"+parts[3]
	case len(parts) >= 2 && parts[0] == "artist":
		l.Kind, l.ID = LinkArtist, parts[1]
	default:
		l.Service = ""
	}
	return l
}

// parseYouTube: /watch?v=…, /shorts/…, /playlist?list=… (OLAK5uy_…
// lists are albums), /browse/MPREb… (album), /channel/UC… (artist).
func parseYouTube(l Link, parts []string, q url.Values) Link {
	l.Service = "youtube"
	switch {
	case len(parts) >= 1 && parts[0] == "watch" && q.Get("v") != "":
		l.Kind, l.ID = LinkTrack, q.Get("v")
	case len(parts) >= 2 && parts[0] == "shorts":
		l.Kind, l.ID = LinkTrack, parts[1]
	case len(parts) >= 1 && parts[0] == "playlist" && q.Get("list") != "":
		l.Kind, l.ID = LinkPlaylist, q.Get("list")
		if strings.HasPrefix(l.ID, "OLAK5uy_") {
			l.Kind = LinkAlbum
		}
	case len(parts) >= 2 && parts[0] == "browse" && strings.HasPrefix(parts[1], "MPREb"):
		l.Kind, l.ID = LinkAlbum, parts[1]
	case len(parts) >= 2 && parts[0] == "channel":
		l.Kind, l.ID = LinkArtist, parts[1]
	default:
		l.Service = ""
	}
	return l
}

// parseSoundCloud: /{user}/{track}, /{user}/sets/{set}, /{user}; the ID is
// the permalink path. Short on.soundcloud.com links are left to the
// provider to follow (their path says nothing).
func parseSoundCloud(l Link, host string, parts []string) Link {
	l.Service = "soundcloud"
	if host == "on.soundcloud.com" {
		if len(parts) == 0 {
			l.Service = ""
			return l
		}
		l.Kind, l.ID = LinkTrack, l.URL
		return l
	}
	switch {
	case len(parts) >= 3 && parts[1] == "sets":
		l.Kind, l.ID = LinkPlaylist, strings.Join(parts[:3], "/")
	case len(parts) >= 2 && !scReserved[parts[1]]:
		l.Kind, l.ID = LinkTrack, strings.Join(parts[:2], "/")
	case len(parts) >= 1 && !scReserved[parts[0]]:
		l.Kind, l.ID = LinkArtist, parts[0]
	default:
		l.Service = ""
	}
	return l
}

// scReserved are SoundCloud pages that aren't a user's track.
var scReserved = map[string]bool{
	"tracks": true, "albums": true, "sets": true, "reposts": true, "likes": true,
	"followers": true, "following": true, "popular-tracks": true, "search": true,
	"discover": true, "stream": true, "you": true, "upload": true,
}

// ExpandShortLink follows a short link (on.soundcloud.com/…) to the page it
// stands for and parses that.
func ExpandShortLink(ctx context.Context, l Link) (Link, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL, nil)
	if err != nil {
		return l, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return l, err
	}
	_ = resp.Body.Close()
	final := resp.Request.URL
	final.RawQuery = ""
	nl, ok := ParseLink(final.String())
	if !ok || strings.HasPrefix(nl.ID, "http") {
		return l, errors.New("could not follow the short link")
	}
	nl.URL = l.URL
	return nl, nil
}
