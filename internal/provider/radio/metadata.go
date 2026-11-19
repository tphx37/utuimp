package radio

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/provider"
)

var metaTag = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
var metaAttr = regexp.MustCompile(`(?is)([a-zA-Z_:][\w:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
var pageTitle = regexp.MustCompile(`(?is)<title\b[^>]*>([^<]*)</title\s*>`)

// linkMetadata reads station headers or public HTML metadata without reading a live stream.
func (p *Provider) linkMetadata(ctx context.Context, link string) provider.Track {
	u, _ := url.Parse(link)
	t := provider.Track{ID: link, CatalogID: link, Title: u.Hostname(), Artist: "Internet radio"}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, link, nil)
	if err != nil {
		return t
	}
	req.Header.Set("User-Agent", "utuimp/1.0")
	res, err := p.client.Do(req)
	if err != nil {
		return t
	}
	_ = res.Body.Close()
	if name := provider.Clean(res.Header.Get("icy-name")); name != "" {
		t.Title = name
	}
	if !strings.Contains(strings.ToLower(res.Header.Get("Content-Type")), "text/html") && res.StatusCode != http.StatusMethodNotAllowed && res.StatusCode != http.StatusNotImplemented {
		return t
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, res.Request.URL.String(), nil)
	if err != nil {
		return t
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 utuimp radio")
	res, err = p.client.Do(req)
	if err != nil {
		return t
	}
	defer func() { _ = res.Body.Close() }()
	if !strings.Contains(strings.ToLower(res.Header.Get("Content-Type")), "text/html") {
		return t
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return t
	}
	body := string(data)
	values := map[string]string{}
	for _, tag := range metaTag.FindAllString(body, -1) {
		attrs := map[string]string{}
		for _, m := range metaAttr.FindAllStringSubmatch(tag, -1) {
			v := m[2]
			if v == "" {
				v = m[3]
			}
			if v == "" {
				v = m[4]
			}
			attrs[strings.ToLower(m[1])] = html.UnescapeString(v)
		}
		key := strings.ToLower(attrs["property"])
		if key == "" {
			key = strings.ToLower(attrs["name"])
		}
		values[key] = provider.Clean(attrs["content"])
	}
	if title := values["og:title"]; title != "" {
		t.Title = title
	} else if m := pageTitle.FindStringSubmatch(body); len(m) > 1 {
		if title := provider.Clean(html.UnescapeString(m[1])); title != "" {
			t.Title = title
		}
	}
	for _, key := range []string{"music:musician", "author", "og:site_name"} {
		if value := values[key]; value != "" && !streamURL(value) {
			t.Artist = value
			break
		}
	}
	for _, key := range []string{"music:album", "album"} {
		if value := values[key]; value != "" && !streamURL(value) {
			t.Album = value
			break
		}
	}
	return t
}
