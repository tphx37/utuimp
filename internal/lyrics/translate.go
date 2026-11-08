package lyrics

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
)

const defaultTranslateURL = "https://clients5.google.com/translate_a/t"

// translateHTTP speaks HTTP/1.1 only: the translate endpoint answers Go's
// HTTP/2 client with 429 Too Many Requests while serving HTTP/1.1 normally.
var translateHTTP = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy:        http.ProxyFromEnvironment,
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
	},
}

// ErrSameLanguage is returned by Translate when the lyrics are already in the
// target language.
var ErrSameLanguage = errors.New("lyrics are already in the target language")

// DefaultTranslateLang derives a target language code from the locale
// environment ("ru_RU.UTF-8" → "ru"). LC_ALL wins outright; otherwise the
// first non-English locale among LC_MESSAGES, LANG and the regional LC_*
// variables is used, because translating into English is rarely what someone
// with an English UI but a Russian region wants. Falls back to "en".
func DefaultTranslateLang() string {
	if code := localeCode(os.Getenv("LC_ALL")); code != "" {
		return code
	}
	for _, env := range []string{"LC_MESSAGES", "LANG", "LC_ADDRESS", "LC_TELEPHONE", "LC_NAME", "LC_TIME", "LC_PAPER"} {
		if code := localeCode(os.Getenv(env)); code != "" && code != "en" {
			return code
		}
	}
	return "en"
}

func localeCode(v string) string {
	if v == "" || v == "C" || v == "POSIX" || strings.HasPrefix(v, "C.") {
		return ""
	}
	code := strings.ToLower(v)
	if i := strings.IndexAny(code, "_.@-"); i > 0 {
		code = code[:i]
	}
	if len(code) < 2 || len(code) > 3 {
		return ""
	}
	return code
}

// Translation is a line-by-line translation of a song's lyrics.
type Translation struct {
	// Lines holds one translation per input line; "" where there is
	// nothing to show (empty line, or the line is already in the target).
	Lines []string
	// Source is the song's language: the most common language among its
	// lines, preferring a non-English one when the song mixes in English.
	Source string
}

// translateBatch caps how many lines go in one request.
const translateBatch = 40

// Translate translates lyric lines into target. Each line is detected and
// translated on its own, so a song that switches language is translated
// throughout, and lines already in the target language are left alone.
// It uses Google Translate's public web endpoint, which needs no key.
func (c *Client) Translate(ctx context.Context, lines []Line, target string) (Translation, error) {
	if target == "" {
		target = "en"
	}
	// Only non-empty lines are sent; idx maps them back.
	var idx []int
	var texts []string
	for i, l := range lines {
		if t := strings.TrimSpace(l.Text); t != "" && !strings.HasPrefix(t, "♪") {
			idx = append(idx, i)
			texts = append(texts, t)
		}
	}
	out := Translation{Lines: make([]string, len(lines))}
	if len(texts) == 0 {
		return out, nil
	}

	langCount := map[string]int{}
	for start := 0; start < len(texts); start += translateBatch {
		end := min(start+translateBatch, len(texts))
		got, err := c.translateLines(ctx, texts[start:end], target)
		if err != nil {
			return Translation{}, err
		}
		for j, g := range got {
			if g.lang != "" {
				langCount[g.lang]++
			}
			if strings.EqualFold(g.lang, target) {
				continue // already readable
			}
			tr := strings.TrimSpace(provider.Clean(g.text))
			if strings.EqualFold(tr, texts[start+j]) {
				continue // names, "oh oh", … come back unchanged
			}
			out.Lines[idx[start+j]] = tr
		}
	}
	out.Source = songLanguage(langCount)
	if strings.EqualFold(out.Source, target) && !slices.ContainsFunc(out.Lines, func(s string) bool { return s != "" }) {
		return Translation{}, ErrSameLanguage
	}
	return out, nil
}

// songLanguage picks the song's language from per-line detections: the most
// common one, except that English yields to another language that covers at
// least a quarter of the lines (English choruses and short lines that read
// as English shouldn't make a French song "English").
func songLanguage(count map[string]int) string {
	total := 0
	best, bestN := "", 0
	nonEn, nonEnN := "", 0
	for lang, n := range count {
		total += n
		if n > bestN || (n == bestN && lang < best) {
			best, bestN = lang, n
		}
		if !strings.HasPrefix(lang, "en") && (n > nonEnN || (n == nonEnN && lang < nonEn)) {
			nonEn, nonEnN = lang, n
		}
	}
	if strings.HasPrefix(best, "en") && nonEn != "" && nonEnN*4 >= total {
		return nonEn
	}
	return best
}

type translatedLine struct {
	text string
	lang string
}

// translateLines translates each text on its own, returning the translation
// and the detected source language of each.
func (c *Client) translateLines(ctx context.Context, texts []string, target string) ([]translatedLine, error) {
	base := defaultTranslateURL
	if c.translateURL != "" {
		base = c.translateURL
	}
	form := url.Values{}
	for _, t := range texts {
		form.Add("q", t)
	}
	q := url.Values{}
	q.Set("client", "dict-chrome-ex")
	q.Set("sl", "auto")
	q.Set("tl", target)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"?"+q.Encode(), strings.NewReader(form.Encode())) //nolint:gosec // G107: constant base URL
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := translateHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("translate: status %d", resp.StatusCode)
	}

	// Response: [["translation","detected-lang"], …] — or, for a single q,
	// sometimes a bare ["translation","lang"] / "translation".
	var raw json.RawMessage
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxLyricsResponseBytes)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("translate: %w", err)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		var single string
		if json.Unmarshal(raw, &single) != nil {
			return nil, fmt.Errorf("translate: unexpected response")
		}
		items = []json.RawMessage{raw}
	}
	if len(texts) == 1 && len(items) == 2 {
		// A bare pair for one input.
		var s string
		if json.Unmarshal(items[0], &s) == nil {
			items = []json.RawMessage{raw}
		}
	}
	out := make([]translatedLine, len(texts))
	for i := range min(len(items), len(texts)) {
		var pair []string
		if json.Unmarshal(items[i], &pair) == nil {
			if len(pair) > 0 {
				out[i].text = pair[0]
			}
			if len(pair) > 1 {
				out[i].lang = strings.ToLower(pair[1])
			}
			continue
		}
		_ = json.Unmarshal(items[i], &out[i].text)
	}
	return out, nil
}
