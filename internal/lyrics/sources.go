package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/tphx37/utuimp/internal/provider"
)

// Source names a lyrics service. SourceAuto tries every service and keeps
// the best result (word-synced > line-synced > plain).
type Source string

const (
	SourceAuto      Source = "auto"
	SourceLRCLIB    Source = "lrclib"
	SourceNetEase   Source = "netease"
	SourceLyricsOvh Source = "lyrics.ovh"
	// SourceService is the music service's own lyrics (looked up by track
	// ID, so always the right song), when the provider offers them.
	SourceService Source = "service"
)

// Sources lists every selectable source, in picker order.
var Sources = []Source{SourceAuto, SourceService, SourceLRCLIB, SourceNetEase, SourceLyricsOvh}

// ServiceName labels SourceService (e.g. "Yandex"); set with the provider.
var ServiceName = "service" //nolint:gochecknoglobals // set once at start-up

// Label is the human-readable name of a source.
func (s Source) Label() string {
	switch s {
	case SourceLRCLIB:
		return "LRCLIB"
	case SourceNetEase:
		return "NetEase"
	case SourceLyricsOvh:
		return "lyrics.ovh"
	case SourceService:
		return ServiceName
	default:
		return "auto"
	}
}

// ParseSource maps a stored name back to a Source, defaulting to auto.
func ParseSource(s string) Source {
	for _, src := range Sources {
		if string(src) == s {
			return src
		}
	}
	return SourceAuto
}

// Query identifies the track lyrics are wanted for.
type Query struct {
	TrackID  string // the provider's track ID (for SourceService)
	Artist   string
	Title    string
	Album    string
	Duration time.Duration
}

var errNotFound = errors.New("lyrics not found")

const (
	defaultNetEaseURL = "https://music.163.com"
	defaultOvhURL     = "https://api.lyrics.ovh"
)

// FetchFrom retrieves lyrics from one source, or from the best available
// source when src is SourceAuto. The returned Result has Source set.
func (c *Client) FetchFrom(ctx context.Context, src Source, q Query) (*Result, error) {
	var (
		res *Result
		err error
	)
	switch src {
	case SourceLRCLIB:
		res, err = c.Fetch(ctx, q.Artist, q.Title, q.Album, q.Duration)
	case SourceNetEase:
		res, err = c.fetchNetEase(ctx, q)
	case SourceLyricsOvh:
		res, err = c.fetchOvh(ctx, q)
	case SourceService:
		res, err = c.fetchService(ctx, q)
	default:
		return c.fetchAuto(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	res.Source = src
	return res, nil
}

// fetchAuto queries LRCLIB and NetEase concurrently and keeps the richest
// result; lyrics.ovh (plain text only) is the last resort.
func (c *Client) fetchAuto(ctx context.Context, q Query) (*Result, error) {
	type outcome struct {
		res *Result
		err error
	}
	lrcCh := make(chan outcome, 1)
	neCh := make(chan outcome, 1)
	svcCh := make(chan outcome, 1)
	go func() {
		if c.ServiceLyrics == nil || q.TrackID == "" {
			svcCh <- outcome{nil, errNotFound}
			return
		}
		r, err := c.FetchFrom(ctx, SourceService, q)
		svcCh <- outcome{r, err}
	}()
	go func() {
		r, err := c.FetchFrom(ctx, SourceLRCLIB, q)
		lrcCh <- outcome{r, err}
	}()
	go func() {
		r, err := c.FetchFrom(ctx, SourceNetEase, q)
		neCh <- outcome{r, err}
	}()
	lrc, ne, svc := <-lrcCh, <-neCh, <-svcCh

	// The service's own lyrics are matched by track ID, so they win ties.
	best := pickRicher(pickRicher(svc.res, lrc.res), ne.res)
	if best != nil {
		return best, nil
	}
	if res, err := c.FetchFrom(ctx, SourceLyricsOvh, q); err == nil {
		return res, nil
	}
	if lrc.err != nil {
		return nil, lrc.err
	}
	return nil, errNotFound
}

// ServiceLyricsFunc fetches lyrics from the music service by track ID: an
// LRC document when synced is true, else plain text.
type ServiceLyricsFunc func(ctx context.Context, trackID string, synced bool) (string, error)

func (c *Client) fetchService(ctx context.Context, q Query) (*Result, error) {
	if c.ServiceLyrics == nil || q.TrackID == "" {
		return nil, errNotFound
	}
	if lrc, err := c.ServiceLyrics(ctx, q.TrackID, true); err == nil {
		if lines, _ := parseLRC(normalizeSpaces(lrc)); len(lines) > 0 {
			ws := false
			for _, l := range lines {
				if len(l.Words) > 0 {
					ws = true
					break
				}
			}
			return &Result{Lines: lines, Synced: true, WordSynced: ws}, nil
		}
	}
	text, err := c.ServiceLyrics(ctx, q.TrackID, false)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(normalizeSpaces(strings.ReplaceAll(text, "\r\n", "\n")))
	if text == "" {
		return nil, errNotFound
	}
	var lines []Line
	for l := range strings.SplitSeq(text, "\n") {
		lines = append(lines, Line{Text: strings.TrimSpace(provider.Clean(l))})
	}
	return &Result{Lines: lines, Plain: provider.CleanLines(text)}, nil
}

// richness ranks a result: word-synced beats line-synced beats plain.
func richness(r *Result) int {
	switch {
	case r == nil || len(r.Lines) == 0:
		return 0
	case r.WordSynced:
		return 3
	case r.Synced:
		return 2
	default:
		return 1
	}
}

// pickRicher prefers b only when it is strictly richer than a, so LRCLIB
// (which matches on exact artist/title/duration) wins ties.
func pickRicher(a, b *Result) *Result {
	ra, rb := richness(a), richness(b)
	if ra == 0 && rb == 0 {
		return nil
	}
	if rb > ra {
		return b
	}
	return a
}

// ── NetEase Cloud Music ────────────────────────────────────────────────────

type neteaseSearch struct {
	Result struct {
		Songs []struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Duration int64  `json:"duration"`
			Artists  []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"songs"`
	} `json:"result"`
}

type neteaseLyric struct {
	Lrc struct {
		Lyric string `json:"lyric"`
	} `json:"lrc"`
	Yrc struct {
		Lyric string `json:"lyric"`
	} `json:"yrc"`
	NoLyric     bool `json:"nolyric"`
	Uncollected bool `json:"uncollected"`
}

func (c *Client) neteaseBase() string {
	if c.neteaseURL != "" {
		return strings.TrimRight(c.neteaseURL, "/")
	}
	return defaultNetEaseURL
}

func (c *Client) getJSON(ctx context.Context, rawURL string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil) //nolint:gosec // G107: base URL is a package constant (or test server); path/query are escaped
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "utuimp")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxLyricsResponseBytes)).Decode(out)
}

func (c *Client) fetchNetEase(ctx context.Context, q Query) (*Result, error) {
	title := cleanSearchTerm(q.Title)
	artist := cleanSearchTerm(q.Artist)
	if title == "" {
		return nil, errNotFound
	}
	hdr := map[string]string{"Referer": "https://music.163.com/", "User-Agent": "Mozilla/5.0"}

	v := url.Values{}
	v.Set("s", strings.TrimSpace(artist+" "+title))
	v.Set("type", "1")
	v.Set("limit", "10")
	var search neteaseSearch
	if err := c.getJSON(ctx, c.neteaseBase()+"/api/search/get?"+v.Encode(), hdr, &search); err != nil {
		return nil, fmt.Errorf("netease search: %w", err)
	}

	wantTitle := normalizeName(title)
	wantArtist := normalizeName(artist)
	type cand struct {
		id    int64
		score int
	}
	var cands []cand
	for _, s := range search.Result.Songs {
		score := 0
		got := normalizeName(cleanSearchTerm(s.Name))
		switch {
		case got == wantTitle:
			score += 40
		case got != "" && (strings.Contains(got, wantTitle) || strings.Contains(wantTitle, got)):
			score += 20
		default:
			continue
		}
		for _, a := range s.Artists {
			an := normalizeName(a.Name)
			if an != "" && (strings.Contains(wantArtist, an) || strings.Contains(an, wantArtist)) {
				score += 30
				break
			}
		}
		if q.Duration > 0 && s.Duration > 0 {
			diff := q.Duration - time.Duration(s.Duration)*time.Millisecond
			if diff < 0 {
				diff = -diff
			}
			switch {
			case diff <= 2*time.Second:
				score += 25
			case diff <= 5*time.Second:
				score += 10
			case diff > 15*time.Second:
				continue
			}
		}
		// Title alone is not enough: require artist or a tight duration match.
		if score < 60 {
			continue
		}
		cands = append(cands, cand{s.ID, score})
	}
	slices.SortStableFunc(cands, func(a, b cand) int { return b.score - a.score })

	// The same song is often uploaded several times and only some uploads
	// carry word timing, so check the top few matches and keep the first
	// word-synced one, else the first line-synced one.
	var fallback *Result
	for i, cd := range cands {
		if i == 3 {
			break
		}
		res, err := c.neteaseLyrics(ctx, cd.id, hdr)
		if err != nil {
			continue
		}
		if res.WordSynced {
			return res, nil
		}
		if fallback == nil || richness(res) > richness(fallback) {
			fallback = res
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, errNotFound
}

func (c *Client) neteaseLyrics(ctx context.Context, id int64, hdr map[string]string) (*Result, error) {
	lv := url.Values{}
	lv.Set("id", strconv.FormatInt(id, 10))
	lv.Set("lv", "1")
	lv.Set("yv", "1")
	lv.Set("tv", "-1")
	var lyr neteaseLyric
	if err := c.getJSON(ctx, c.neteaseBase()+"/api/song/lyric?"+lv.Encode(), hdr, &lyr); err != nil {
		return nil, fmt.Errorf("netease lyric: %w", err)
	}
	if lyr.NoLyric {
		return &Result{Lines: []Line{{Text: "♪  Instrumental  ♪"}}, Plain: "♪ Instrumental ♪"}, nil
	}
	if lines := parseYRC(lyr.Yrc.Lyric); len(lines) > 0 {
		return &Result{Lines: lines, Synced: true, WordSynced: true}, nil
	}
	if lines, _ := parseLRC(normalizeSpaces(lyr.Lrc.Lyric)); len(lines) > 0 {
		lines = dropCredits(lines)
		if len(lines) > 0 {
			return &Result{Lines: lines, Synced: true}, nil
		}
	}
	return nil, errNotFound
}

// parseYRC parses NetEase "yrc" word-timed lyrics:
//
//	[lineStartMs,lineDurMs](wordStartMs,wordDurMs,0)word (wordStartMs,wordDurMs,0)word
//
// Metadata lines (JSON objects) and credit lines are skipped.
func parseYRC(s string) []Line {
	var lines []Line
	for raw := range strings.SplitSeq(normalizeSpaces(s), "\n") {
		raw = strings.TrimSpace(raw)
		if !strings.HasPrefix(raw, "[") {
			continue
		}
		end := strings.IndexByte(raw, ']')
		if end < 0 {
			continue
		}
		hdr := strings.SplitN(raw[1:end], ",", 2)
		lineStart, err := strconv.Atoi(hdr[0])
		if err != nil {
			continue
		}
		rest := raw[end+1:]
		var words []Word
		var text strings.Builder
		for len(rest) > 0 {
			if rest[0] != '(' {
				// Text outside a timing tag: attach to the previous word.
				next := strings.IndexByte(rest, '(')
				if next < 0 {
					next = len(rest)
				}
				chunk := provider.Clean(rest[:next])
				if len(words) > 0 {
					words[len(words)-1].Text += chunk
				}
				text.WriteString(chunk)
				rest = rest[next:]
				continue
			}
			closeIdx := strings.IndexByte(rest, ')')
			if closeIdx < 0 {
				break
			}
			parts := strings.Split(rest[1:closeIdx], ",")
			rest = rest[closeIdx+1:]
			if len(parts) < 2 {
				continue
			}
			ws, err1 := strconv.Atoi(parts[0])
			wd, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil {
				continue
			}
			next := strings.IndexByte(rest, '(')
			if next < 0 {
				next = len(rest)
			}
			wordText := provider.Clean(rest[:next])
			rest = rest[next:]
			if wordText == "" {
				continue
			}
			words = append(words, Word{
				Start: time.Duration(ws) * time.Millisecond,
				Dur:   time.Duration(wd) * time.Millisecond,
				Text:  wordText,
			})
			text.WriteString(wordText)
		}
		lineText := strings.TrimSpace(text.String())
		if lineText == "" || isCreditLine(lineText) {
			continue
		}
		if len(words) > 0 {
			words[0].Text = strings.TrimLeft(words[0].Text, " \t")
			words[len(words)-1].Text = strings.TrimRight(words[len(words)-1].Text, " \t")
		}
		lines = append(lines, Line{Start: time.Duration(lineStart) * time.Millisecond, Text: lineText, Words: words})
	}
	return lines
}

// creditKeywords are the Chinese role names NetEase prefixes credit lines
// with ("作词 : Someone").
var creditKeywords = []string{
	"作词", "作曲", "编曲", "制作", "混音", "母带", "录音", "和声", "监制", "出品",
	"发行", "吉他", "贝斯", "鼓", "键盘", "弦乐", "词", "曲", "演唱", "原唱", "翻唱", "OP", "SP",
}

// isCreditLine reports whether a line is a "role : name" credit rather than
// a sung lyric. Only lines whose role contains Han characters (NetEase's own
// credits) or matches a keyword are dropped, so lyrics with colons survive.
func isCreditLine(s string) bool {
	idx := strings.IndexAny(s, ":：")
	if idx <= 0 || idx > 30 {
		return false
	}
	role := strings.TrimSpace(s[:idx])
	for _, kw := range creditKeywords {
		if strings.Contains(role, kw) && (kw != "OP" && kw != "SP" || role == kw) {
			return true
		}
	}
	for _, r := range role {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func dropCredits(lines []Line) []Line {
	out := lines[:0]
	for _, l := range lines {
		if isCreditLine(l.Text) {
			continue
		}
		out = append(out, l)
	}
	// Leading blank lines carry no information.
	for len(out) > 0 && strings.TrimSpace(out[0].Text) == "" {
		out = out[1:]
	}
	return out
}

// ── lyrics.ovh ─────────────────────────────────────────────────────────────

func (c *Client) fetchOvh(ctx context.Context, q Query) (*Result, error) {
	artist := cleanSearchTerm(q.Artist)
	title := cleanSearchTerm(q.Title)
	if artist == "" || title == "" {
		return nil, errNotFound
	}
	base := defaultOvhURL
	if c.ovhURL != "" {
		base = strings.TrimRight(c.ovhURL, "/")
	}
	u := base + "/v1/" + url.PathEscape(artist) + "/" + url.PathEscape(title)
	var data struct {
		Lyrics string `json:"lyrics"`
	}
	if err := c.getJSON(ctx, u, nil, &data); err != nil {
		return nil, fmt.Errorf("lyrics.ovh: %w", err)
	}
	text := strings.TrimSpace(normalizeSpaces(strings.ReplaceAll(data.Lyrics, "\r\n", "\n")))
	if text == "" {
		return nil, errNotFound
	}
	// lyrics.ovh sometimes prefixes "Paroles de la chanson … par …".
	if first, rest, ok := strings.Cut(text, "\n"); ok && strings.HasPrefix(first, "Paroles de la chanson") {
		text = strings.TrimSpace(rest)
	}
	var lines []Line
	for l := range strings.SplitSeq(text, "\n") {
		lines = append(lines, Line{Text: strings.TrimSpace(provider.Clean(l))})
	}
	return &Result{Lines: lines, Plain: provider.CleanLines(text)}, nil
}

// ── helpers ────────────────────────────────────────────────────────────────

// normalizeSpaces turns non-breaking spaces into plain ones.
func normalizeSpaces(s string) string {
	return strings.ReplaceAll(s, " ", " ")
}

// normalizeName lowercases s and keeps only letters and digits so names
// compare equal across punctuation differences ("J.B.G." == "JBG").
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
