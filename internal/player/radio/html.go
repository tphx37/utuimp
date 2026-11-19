package radio

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/tphx37/utuimp/internal/i18n"
)

var htmlTags = regexp.MustCompile(`(?is)<(/?)(audio|source|meta|link)\b[^>]*>`)
var htmlAttrs = regexp.MustCompile(`(?is)([a-zA-Z_:][\w:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)

// resolveHTML reads public player markup, not arbitrary URLs embedded in scripts.
func resolveHTML(ctx context.Context, page string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 utuimp radio")
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("radio page: HTTP %d", res.StatusCode)
	}
	if typ := strings.ToLower(res.Header.Get("Content-Type")); typ != "" && !strings.Contains(typ, "html") {
		return res.Request.URL.String(), nil
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return "", err
	}
	base := res.Request.URL
	inAudio := false
	for _, tag := range htmlTags.FindAllStringSubmatch(string(body), -1) {
		kind := strings.ToLower(tag[2])
		if tag[1] == "/" {
			if kind == "audio" {
				inAudio = false
			}
			continue
		}
		if kind == "audio" {
			inAudio = true
		}
		attrs := map[string]string{}
		for _, m := range htmlAttrs.FindAllStringSubmatch(tag[0], -1) {
			val := m[2]
			if val == "" {
				val = m[3]
			}
			if val == "" {
				val = m[4]
			}
			attrs[strings.ToLower(m[1])] = html.UnescapeString(val)
		}
		var ref string
		switch kind {
		case "audio":
			ref = attrs["src"]
		case "source":
			typ := strings.ToLower(attrs["type"])
			u, _ := url.Parse(attrs["src"])
			ext := ""
			if u != nil {
				ext = strings.ToLower(path.Ext(u.Path))
			}
			if inAudio || strings.HasPrefix(typ, "audio/") || typ == "application/vnd.apple.mpegurl" || ext == ".mp3" || ext == ".aac" || ext == ".m3u8" || ext == ".ogg" {
				ref = attrs["src"]
			}
		case "meta":
			name := strings.ToLower(attrs["property"])
			if name == "" {
				name = strings.ToLower(attrs["name"])
			}
			if name == "og:audio" || name == "og:audio:url" || name == "og:audio:secure_url" || name == "twitter:player:stream" {
				ref = attrs["content"]
			}
		case "link":
			if strings.HasPrefix(strings.ToLower(attrs["type"]), "audio/") {
				ref = attrs["href"]
			}
		}
		if ref == "" {
			continue
		}
		u, e := url.Parse(ref)
		if e != nil {
			continue
		}
		u = base.ResolveReference(u)
		if (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			target := u.String()
			ext := strings.ToLower(path.Ext(u.Path))
			if target != page && (ext == ".m3u" || ext == ".pls" || ext == ".m3u8") {
				return resolvePlaylist(ctx, target)
			}
			return target, nil
		}
	}
	return "", fmt.Errorf("%s", i18n.T("No direct audio stream found on this page"))
}
