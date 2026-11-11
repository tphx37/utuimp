// Package yandex is a Yandex Music provider built on the unofficial API that
// the official apps use (documented by the reverse-engineered libraries
// MarshalX/yandex-music-api and oklookat/goym).
package yandex

import (
	"context"
	"crypto/hmac"
	"crypto/md5" //nolint:gosec // required by the download signing scheme, not for security
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	apiBase       = "https://api.music.yandex.net"
	clientHeader  = "YandexMusicAndroid/24023621"
	userAgent     = "Yandex-Music-API"
	maxBody       = 32 << 20
	downloadSalt  = "XGRlBW9FXlekgbPrRHuSiA"
	lyricsSignKey = "p93jhgh689SBReK6ghtw62"
)

// ErrUnauthorized means the OAuth token was rejected.
var ErrUnauthorized = errors.New("yandex music: unauthorized")

// Client talks to api.music.yandex.net.
type Client struct {
	http  *http.Client
	base  string
	token string
	lang  string
}

// NewClient returns a client authenticated with an OAuth token.
func NewClient(token string) *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second}, base: apiBase, token: token, lang: "ru"}
}

type apiError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

type envelope struct {
	Result json.RawMessage `json:"result"`
	Error  *apiError       `json:"error"`
}

func (c *Client) newRequest(ctx context.Context, method, path string, q url.Values, form url.Values) (*http.Request, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "OAuth "+c.token)
	}
	req.Header.Set("X-Yandex-Music-Client", clientHeader)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", c.lang)
	return req, nil
}

// do performs a request and decodes its "result" into out.
func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("yandex music: HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	if env.Error != nil && (env.Error.Name != "" || env.Error.Message != "") {
		if env.Error.Message == "session-expired" {
			return ErrUnauthorized
		}
		return fmt.Errorf("yandex music: %s %s", env.Error.Name, env.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("yandex music: HTTP %d", resp.StatusCode)
	}
	if out == nil || len(env.Result) == 0 {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	req, err := c.newRequest(ctx, http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, form url.Values, out any) error {
	if form == nil {
		form = url.Values{}
	}
	req, err := c.newRequest(ctx, http.MethodPost, path, nil, form)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// ── API models ─────────────────────────────────────────────────────────────

type ymArtist struct {
	ID   json.Number `json:"id"`
	Name string      `json:"name"`
}

type ymAlbum struct {
	ID            json.Number `json:"id"`
	Title         string      `json:"title"`
	Version       string      `json:"version"`
	Year          int         `json:"year"`
	ReleaseDate   string      `json:"releaseDate"`
	CoverURI      string      `json:"coverUri"`
	TrackCount    int         `json:"trackCount"`
	Artists       []ymArtist  `json:"artists"`
	Genre         string      `json:"genre"`
	Volumes       [][]ymTrack `json:"volumes"`
	TrackPosition *struct {
		Volume int `json:"volume"`
		Index  int `json:"index"`
	} `json:"trackPosition"`
}

type ymTrack struct {
	ID             json.RawMessage `json:"id"` // string or number
	Title          string          `json:"title"`
	Version        string          `json:"version"`
	DurationMs     int64           `json:"durationMs"`
	Available      *bool           `json:"available"`
	Explicit       bool            `json:"explicit"`
	ContentWarning string          `json:"contentWarning"`
	CoverURI       string          `json:"coverUri"`
	Artists        []ymArtist      `json:"artists"`
	Albums         []ymAlbum       `json:"albums"`
	LyricsInfo     *struct {
		HasAvailableSyncLyrics bool `json:"hasAvailableSyncLyrics"`
		HasAvailableTextLyrics bool `json:"hasAvailableTextLyrics"`
	} `json:"lyricsInfo"`
}

// idString returns the track's numeric id as a string.
func (t ymTrack) idString() string {
	s := strings.Trim(string(t.ID), `"`)
	return s
}

type ymPlaylist struct {
	UID        json.Number `json:"uid"`
	Kind       json.Number `json:"kind"`
	Title      string      `json:"title"`
	TrackCount int         `json:"trackCount"`
	Revision   int         `json:"revision"`
	Owner      struct {
		UID   json.Number `json:"uid"`
		Login string      `json:"login"`
	} `json:"owner"`
	Cover *struct {
		URI       string   `json:"uri"`
		ItemsURIs []string `json:"itemsUri"`
	} `json:"cover"`
	OgImage string         `json:"ogImage"`
	Tracks  []ymTrackShort `json:"tracks"`
}

type ymTrackShort struct {
	ID      json.RawMessage `json:"id"`
	AlbumID json.RawMessage `json:"albumId"`
	Track   *ymTrack        `json:"track"`
}

func rawID(r json.RawMessage) string { return strings.Trim(string(r), `"`) }

// ── endpoints ──────────────────────────────────────────────────────────────

// AccountUID returns the signed-in user's id.
func (c *Client) AccountUID(ctx context.Context) (string, error) {
	var res struct {
		Account struct {
			UID json.Number `json:"uid"`
		} `json:"account"`
	}
	if err := c.get(ctx, "/account/status", nil, &res); err != nil {
		return "", err
	}
	if res.Account.UID == "" {
		return "", ErrUnauthorized
	}
	return res.Account.UID.String(), nil
}

type searchResult struct {
	Tracks *struct {
		Results []ymTrack `json:"results"`
	} `json:"tracks"`
	Albums *struct {
		Results []ymAlbum `json:"results"`
	} `json:"albums"`
	Artists *struct {
		Results []ymArtist `json:"results"`
	} `json:"artists"`
	Playlists *struct {
		Results []ymPlaylist `json:"results"`
	} `json:"playlists"`
}

func (c *Client) search(ctx context.Context, text string) (*searchResult, error) {
	q := url.Values{}
	q.Set("text", text)
	q.Set("type", "all")
	q.Set("page", "0")
	q.Set("nocorrect", "False")
	q.Set("playlist-in-best", "True")
	var res searchResult
	return &res, c.get(ctx, "/search", q, &res)
}

// tracksByIDs fetches full track objects ("id" or "id:albumId"), in batches.
func (c *Client) tracksByIDs(ctx context.Context, ids []string) ([]ymTrack, error) {
	var out []ymTrack
	for start := 0; start < len(ids); start += 200 {
		end := min(start+200, len(ids))
		form := url.Values{}
		form.Set("track-ids", strings.Join(ids[start:end], ","))
		form.Set("with-positions", "True")
		var batch []ymTrack
		if err := c.post(ctx, "/tracks", form, &batch); err != nil {
			return out, err
		}
		out = append(out, batch...)
	}
	return out, nil
}

func (c *Client) likedTrackIDs(ctx context.Context, uid string) ([]string, error) {
	var res struct {
		Library struct {
			Tracks []struct {
				ID        json.RawMessage `json:"id"`
				AlbumID   json.RawMessage `json:"albumId"`
				Timestamp string          `json:"timestamp"`
			} `json:"tracks"`
		} `json:"library"`
	}
	q := url.Values{}
	q.Set("if-modified-since-revision", "0")
	if err := c.get(ctx, "/users/"+uid+"/likes/tracks", q, &res); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(res.Library.Tracks))
	for _, t := range res.Library.Tracks {
		id := rawID(t.ID)
		if a := rawID(t.AlbumID); a != "" && a != "null" {
			id += ":" + a
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (c *Client) setLike(ctx context.Context, uid, trackID string, liked bool) error {
	action := "add-multiple"
	if !liked {
		action = "remove"
	}
	form := url.Values{}
	form.Set("track-ids", trackID)
	return c.post(ctx, "/users/"+uid+"/likes/tracks/"+action, form, nil)
}

func (c *Client) userPlaylists(ctx context.Context, uid string) ([]ymPlaylist, error) {
	var res []ymPlaylist
	return res, c.get(ctx, "/users/"+uid+"/playlists/list", nil, &res)
}

func (c *Client) playlist(ctx context.Context, uid, kind string) (*ymPlaylist, error) {
	var res ymPlaylist
	return &res, c.get(ctx, "/users/"+url.PathEscape(uid)+"/playlists/"+url.PathEscape(kind), nil, &res)
}

func (c *Client) albumWithTracks(ctx context.Context, id string) (*ymAlbum, error) {
	var res ymAlbum
	return &res, c.get(ctx, "/albums/"+url.PathEscape(id)+"/with-tracks", nil, &res)
}

func (c *Client) artistAlbums(ctx context.Context, id string) ([]ymAlbum, error) {
	q := url.Values{}
	q.Set("page", "0")
	q.Set("page-size", "200")
	q.Set("sort-by", "year")
	var res struct {
		Albums []ymAlbum `json:"albums"`
	}
	return res.Albums, c.get(ctx, "/artists/"+url.PathEscape(id)+"/direct-albums", q, &res)
}

// rotorBatch is a rotor session's next tracks.
type rotorBatch struct {
	SessionID string
	BatchID   string
	Tracks    []ymTrack
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, out any) error {
	body, _ := json.Marshal(payload)
	req, err := c.newRequest(ctx, http.MethodPost, path, nil, nil)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

type rotorResponse struct {
	RadioSessionID string `json:"radioSessionId"`
	BatchID        string `json:"batchId"`
	Sequence       []struct {
		Track *ymTrack `json:"track"`
	} `json:"sequence"`
}

func (r rotorResponse) batch(sid string) rotorBatch {
	b := rotorBatch{SessionID: r.RadioSessionID, BatchID: r.BatchID}
	if b.SessionID == "" {
		b.SessionID = sid
	}
	for _, s := range r.Sequence {
		if s.Track != nil {
			b.Tracks = append(b.Tracks, *s.Track)
		}
	}
	return b
}

// rotorNew starts a rotor session ("My Wave", track/artist radio…).
func (c *Client) rotorNew(ctx context.Context, seeds []string) (rotorBatch, error) {
	var res rotorResponse
	err := c.postJSON(ctx, "/rotor/session/new", map[string]any{"seeds": seeds, "includeTracksInResponse": true}, &res)
	return res.batch(""), err
}

// rotorNext asks a session for its next tracks; queue lists the tracks
// already handed out (most recent last) so the station continues instead of
// starting over.
func (c *Client) rotorNext(ctx context.Context, sid string, queue []string) (rotorBatch, error) {
	var res rotorResponse
	err := c.postJSON(ctx, "/rotor/session/"+url.PathEscape(sid)+"/tracks", map[string]any{"queue": queue}, &res)
	return res.batch(sid), err
}

// rotorFeedback reports a playback event ("trackStarted", "trackFinished",
// "skip") so the station learns from what was played.
func (c *Client) rotorFeedback(ctx context.Context, sid, batchID, event, trackID string, played time.Duration) error {
	ev := map[string]any{
		"type":      event,
		"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"from":      "radio-mobile-user-onyourwave-default",
	}
	if trackID != "" {
		ev["trackId"] = trackID
	}
	if event == "trackFinished" || event == "skip" {
		ev["totalPlayedSeconds"] = played.Seconds()
	}
	body := map[string]any{"event": ev}
	if batchID != "" {
		body["batchId"] = batchID
	}
	return c.postJSON(ctx, "/rotor/session/"+url.PathEscape(sid)+"/feedback", body, nil)
}

func (c *Client) createPlaylist(ctx context.Context, uid, title string) (*ymPlaylist, error) {
	form := url.Values{}
	form.Set("title", title)
	form.Set("visibility", "private")
	var res ymPlaylist
	return &res, c.post(ctx, "/users/"+uid+"/playlists/create", form, &res)
}

// insertTracks adds tracks ("id:albumId") at the end of a playlist.
func (c *Client) insertTracks(ctx context.Context, uid, kind string, at, revision int, trackIDs []string) error {
	type item struct {
		ID      string `json:"id"`
		AlbumID string `json:"albumId,omitempty"`
	}
	items := make([]item, 0, len(trackIDs))
	for _, id := range trackIDs {
		tid, aid, _ := strings.Cut(id, ":")
		items = append(items, item{ID: tid, AlbumID: aid})
	}
	diff, _ := json.Marshal([]map[string]any{{"op": "insert", "at": at, "tracks": items}})
	form := url.Values{}
	form.Set("diff", string(diff))
	form.Set("revision", strconv.Itoa(revision))
	return c.post(ctx, "/users/"+uid+"/playlists/"+kind+"/change-relative", form, nil)
}

// ── streaming ──────────────────────────────────────────────────────────────

type downloadInfo struct {
	Codec           string `json:"codec"`
	BitrateInKbps   int    `json:"bitrateInKbps"`
	Preview         bool   `json:"preview"`
	DownloadInfoURL string `json:"downloadInfoUrl"`
}

// StreamURL returns a direct, short-lived (about a minute) link to the
// track's audio, preferring AAC (ADTS) and then MP3, highest bitrate first.
// codec reports which one was chosen.
func (c *Client) StreamURL(ctx context.Context, trackID string) (link, codec string, err error) {
	var infos []downloadInfo
	if err := c.get(ctx, "/tracks/"+url.PathEscape(trackID)+"/download-info", nil, &infos); err != nil {
		return "", "", err
	}
	best := pickDownload(infos)
	if best == nil {
		return "", "", errors.New("no downloadable version of this track")
	}
	req, err := c.newRequest(ctx, http.MethodGet, "", nil, nil)
	if err != nil {
		return "", "", err
	}
	req.URL, err = url.Parse(best.DownloadInfoURL)
	if err != nil {
		return "", "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var x struct {
		Host string `xml:"host"`
		Path string `xml:"path"`
		TS   string `xml:"ts"`
		S    string `xml:"s"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&x); err != nil {
		return "", "", fmt.Errorf("download info: %w", err)
	}
	return signedDownloadURL(x.Host, x.Path, x.TS, x.S), best.Codec, nil
}

// pickDownload prefers a full version over a preview, then AAC over MP3,
// then the highest bitrate.
func pickDownload(infos []downloadInfo) *downloadInfo {
	rank := func(d downloadInfo) int {
		r := d.BitrateInKbps
		switch d.Codec {
		case "aac":
			r += 10000
		case "mp3":
			r += 5000
		default:
			return -1
		}
		if !d.Preview {
			r += 100000
		}
		return r
	}
	var best *downloadInfo
	for i := range infos {
		if rank(infos[i]) < 0 {
			continue
		}
		if best == nil || rank(infos[i]) > rank(*best) {
			best = &infos[i]
		}
	}
	return best
}

// signedDownloadURL builds the final link from download-info XML fields.
func signedDownloadURL(host, path, ts, s string) string {
	sum := md5.Sum([]byte(downloadSalt + strings.TrimPrefix(path, "/") + s)) //nolint:gosec // protocol requirement
	return "https://" + host + "/get-mp3/" + hex.EncodeToString(sum[:]) + "/" + ts + path
}

// ── lyrics ─────────────────────────────────────────────────────────────────

// Lyrics returns the track's lyrics in LRC (synced) or plain text form.
func (c *Client) Lyrics(ctx context.Context, trackID string, synced bool) (string, error) {
	numeric, _, _ := strings.Cut(trackID, ":")
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(lyricsSignKey))
	mac.Write([]byte(numeric + ts))
	q := url.Values{}
	if synced {
		q.Set("format", "LRC")
	} else {
		q.Set("format", "TEXT")
	}
	q.Set("timeStamp", ts)
	q.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	var res struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := c.get(ctx, "/tracks/"+url.PathEscape(numeric)+"/lyrics", q, &res); err != nil {
		return "", err
	}
	if res.DownloadURL == "" {
		return "", errors.New("no lyrics")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, res.DownloadURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lyrics: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(data), err
}

// deleteTrack removes the track at position at from a playlist.
func (c *Client) deleteTrack(ctx context.Context, uid, kind string, at, revision int) error {
	diff, _ := json.Marshal([]map[string]any{{"op": "delete", "from": at, "to": at + 1}})
	form := url.Values{}
	form.Set("diff", string(diff))
	form.Set("revision", strconv.Itoa(revision))
	return c.post(ctx, "/users/"+url.PathEscape(uid)+"/playlists/"+url.PathEscape(kind)+"/change-relative", form, nil)
}

// renamePlaylist gives a playlist a new title.
func (c *Client) renamePlaylist(ctx context.Context, uid, kind, title string) error {
	form := url.Values{}
	form.Set("value", title)
	return c.post(ctx, "/users/"+url.PathEscape(uid)+"/playlists/"+url.PathEscape(kind)+"/name", form, nil)
}

// deletePlaylist deletes a playlist.
func (c *Client) deletePlaylist(ctx context.Context, uid, kind string) error {
	return c.post(ctx, "/users/"+url.PathEscape(uid)+"/playlists/"+url.PathEscape(kind)+"/delete", url.Values{}, nil)
}

// likedAlbums returns the albums the user liked.
func (c *Client) likedAlbums(ctx context.Context, uid string) ([]ymAlbum, error) {
	q := url.Values{}
	q.Set("rich", "true")
	var res []struct {
		Album *ymAlbum    `json:"album"`
		ID    json.Number `json:"id"`
	}
	if err := c.get(ctx, "/users/"+url.PathEscape(uid)+"/likes/albums", q, &res); err != nil {
		return nil, err
	}
	out := make([]ymAlbum, 0, len(res))
	for _, r := range res {
		switch {
		case r.Album != nil:
			out = append(out, *r.Album)
		case r.ID != "":
			out = append(out, ymAlbum{ID: r.ID})
		}
	}
	return out, nil
}

// likedArtists returns the artists the user liked.
func (c *Client) likedArtists(ctx context.Context, uid string) ([]ymArtist, error) {
	q := url.Values{}
	q.Set("with-timestamps", "true")
	var res []json.RawMessage
	if err := c.get(ctx, "/users/"+url.PathEscape(uid)+"/likes/artists", q, &res); err != nil {
		return nil, err
	}
	out := make([]ymArtist, 0, len(res))
	for _, raw := range res {
		var wrapped struct {
			Artist *ymArtist `json:"artist"`
		}
		if json.Unmarshal(raw, &wrapped) == nil && wrapped.Artist != nil {
			out = append(out, *wrapped.Artist)
			continue
		}
		var a ymArtist
		if json.Unmarshal(raw, &a) == nil && a.ID != "" {
			out = append(out, a)
		}
	}
	return out, nil
}

// setLikeOf likes or unlikes an album or an artist (kind "album", "artist").
func (c *Client) setLikeOf(ctx context.Context, uid, kind, id string, liked bool) error {
	action := "add-multiple"
	if !liked {
		action = "remove"
	}
	form := url.Values{}
	form.Set(kind+"-ids", id)
	return c.post(ctx, "/users/"+url.PathEscape(uid)+"/likes/"+kind+"s/"+action, form, nil)
}
