package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

// Links: a service's share link pasted into search (/) or given to :link
// is opened in that service's tab. Search shows what the link points at as
// its results; :link offers to play it or add it to the queue.

// linkRouteMsg asks the App to open l in the tab of l.Service.
type linkRouteMsg struct {
	link   provider.Link
	search bool // open in search (/) rather than the :link offer
}

// openLinkMsg is a link for this tab to open.
type openLinkMsg struct {
	link   provider.Link
	search bool
}

// linkUnavailableMsg: the link's service has no tab here.
type linkUnavailableMsg struct{ service string }

// linkResolvedMsg is what a :link link turned out to be.
type linkResolvedMsg struct {
	url    string
	result *provider.SearchResult
	err    error
}

// linkOfferState is the :link window.
type linkOfferState struct {
	link    provider.Link
	loading bool
	err     error
	result  *provider.SearchResult
	items   []linkOfferItem
	idx     int
}

type linkOfferAction int

const (
	linkPlayNow linkOfferAction = iota
	linkPlayNext
	linkAddToQueue
	linkOpenArtist
	linkShowInSearch
)

type linkOfferItem struct {
	action linkOfferAction
	label  string
}

var serviceNames = map[string]string{
	"spotify": "Spotify", "apple": "Apple Music", "yandex": "Yandex Music",
	"youtube": "YouTube Music", "soundcloud": "SoundCloud",
}

// openLink opens l here, or hands it to the App for its own tab.
func (m *Model) openLink(l provider.Link, search bool) tea.Cmd {
	if m.serviceKey != "" && l.Service != m.serviceKey {
		if search {
			m.searchQuery, m.searchCursor = "", 0
			m.searchGen++
			m.search.SetResults(nil, false, nil)
			m.mode = modeNormal
		}
		m.appendLog(fmt.Sprintf("[link] %s → %s tab", l.URL, l.Service))
		return func() tea.Msg { return linkRouteMsg{link: l, search: search} }
	}
	return m.handleOpenLink(openLinkMsg{link: l, search: search})
}

func (m *Model) handleOpenLink(msg openLinkMsg) tea.Cmd {
	l := msg.link
	m.songMenu = nil
	m.prompt = nil
	m.appendLog(fmt.Sprintf("[link] opening %s (%s %s)", l.URL, l.Kind, l.ID))
	if msg.search {
		m.linkOffer = nil
		m.activePanel = -1
		m.mode = modeSearch
		m.searchQuery = l.URL
		m.searchCursor = len([]rune(l.URL))
		m.searchGen++
		m.search.SetResults(nil, true, nil)
		if m.engineStarting() {
			// EngineReadyMsg searches m.searchQuery again.
			return nil
		}
		return m.resolveLinkCmd(l, true)
	}
	m.mode = modeNormal
	m.linkOffer = &linkOfferState{link: l, loading: true}
	if m.engineStarting() {
		m.pendingLink = &l
		return nil
	}
	return m.resolveLinkCmd(l, false)
}

// engineStarting reports that the service is still connecting.
func (m *Model) engineStarting() bool {
	_, ok := m.provider.(provider.NotReady)
	return ok && m.engineErr == ""
}

// resolveLinkCmd looks the link up; for search the answer comes back as
// the query's search results.
func (m *Model) resolveLinkCmd(l provider.Link, search bool) tea.Cmd {
	prov := m.provider
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := resolveLink(ctx, prov, l)
		if search {
			return searchResultMsg{result: res, query: l.URL, err: err}
		}
		return linkResolvedMsg{url: l.URL, result: res, err: err}
	}
}

// resolveLink finds what l points at with the provider's own lookups.
func resolveLink(ctx context.Context, prov provider.Provider, l provider.Link) (*provider.SearchResult, error) {
	if r, ok := prov.(provider.LinkResolver); ok {
		return r.ResolveLink(ctx, l)
	}
	if strings.HasPrefix(l.ID, "http") {
		nl, err := provider.ExpandShortLink(ctx, l)
		if err != nil {
			return nil, err
		}
		l = nl
	}
	notFound := errors.New(i18n.T("nothing found at this link"))
	switch l.Kind {
	case provider.LinkTrack:
		lk, ok := prov.(provider.TrackLookup)
		if !ok {
			return nil, errors.New(i18n.T("this service can't open song links"))
		}
		t, err := lk.Lookup(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		if t.Title == "" {
			return nil, notFound
		}
		return &provider.SearchResult{Tracks: []provider.Track{t}}, nil
	case provider.LinkAlbum:
		ts, err := prov.GetAlbumTracks(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		if len(ts) == 0 {
			return nil, notFound
		}
		a := provider.Album{ID: l.ID, Title: ts[0].Album, Artist: ts[0].Artist,
			ArtworkURL: ts[0].ArtworkURL, TrackCount: len(ts), ReleaseDate: ts[0].ReleaseDate}
		if a.Title == "" {
			a.Title = i18n.T("Album")
		}
		return &provider.SearchResult{Albums: []provider.Album{a}, Tracks: ts}, nil
	case provider.LinkPlaylist:
		ts, err := prov.GetCatalogPlaylistTracks(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		if len(ts) == 0 {
			return nil, notFound
		}
		pl := provider.Playlist{ID: l.ID, Name: i18n.T("Playlist from the link"), TrackCount: len(ts)}
		return &provider.SearchResult{Playlists: []provider.Playlist{pl}, Tracks: ts}, nil
	case provider.LinkArtist:
		albums, err := prov.GetArtistAlbums(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		name := ""
		for _, a := range albums {
			if a.Artist != "" {
				name = a.Artist
				break
			}
		}
		if name == "" {
			name = i18n.T("Artist")
		}
		return &provider.SearchResult{Artists: []provider.Artist{{ID: l.ID, Name: name}}, Albums: albums}, nil
	}
	return nil, notFound
}

// linkCommand runs ":link <url>".
func (m *Model) linkCommand(arg string) tea.Cmd {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		m.note(i18n.T(":link requires a link"))
		return nil
	}
	if m.serviceKey == "radio" {
		if u, err := url.Parse(arg); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			return m.openLink(provider.Link{Service: "radio", Kind: provider.LinkTrack, ID: arg, URL: arg}, false)
		}
	}
	l, ok := provider.ParseLink(arg)
	if !ok {
		m.note(i18n.T("not a Spotify, Apple Music, Yandex Music, YouTube or SoundCloud link"))
		return nil
	}
	return m.openLink(l, false)
}

func (m *Model) handleLinkResolved(msg linkResolvedMsg) {
	lo := m.linkOffer
	if lo == nil || lo.link.URL != msg.url {
		return
	}
	lo.loading = false
	lo.err = msg.err
	lo.result = msg.result
	if msg.err != nil {
		m.appendLog(fmt.Sprintf("[link] %s: %v", msg.url, msg.err))
		return
	}
	r := msg.result
	switch {
	case r == nil:
		lo.err = errors.New(i18n.T("nothing found at this link"))
	case len(r.Tracks) > 0:
		lo.items = []linkOfferItem{
			{linkPlayNow, i18n.T("Play now")},
			{linkPlayNext, i18n.T("Play next")},
			{linkAddToQueue, i18n.T("Add to queue")},
		}
		if len(r.Tracks) > 1 {
			lo.items = append(lo.items, linkOfferItem{linkShowInSearch, i18n.T("Show the songs")})
		}
	case len(r.Artists) > 0:
		lo.items = []linkOfferItem{{linkOpenArtist, i18n.T("Open artist")}}
	default:
		lo.err = errors.New(i18n.T("nothing found at this link"))
	}
}

// linkOfferTitle is what the link points at, for the window's header.
func (lo *linkOfferState) title() (string, string) {
	r := lo.result
	switch {
	case r == nil:
		return lo.link.URL, ""
	case len(r.Albums) > 0 && len(r.Tracks) > 0:
		a := r.Albums[0]
		return a.Title, a.Artist + " · " + fmt.Sprintf(i18n.T("%d tracks"), len(r.Tracks))
	case len(r.Playlists) > 0:
		return r.Playlists[0].Name, fmt.Sprintf(i18n.T("%d tracks"), len(r.Tracks))
	case len(r.Tracks) > 0:
		return r.Tracks[0].Title, r.Tracks[0].Artist
	case len(r.Artists) > 0:
		return r.Artists[0].Name, i18n.T("artist")
	}
	return lo.link.URL, ""
}

func (m *Model) handleLinkOfferKey(k string) tea.Cmd {
	lo := m.linkOffer
	switch k {
	case "esc", "q":
		m.linkOffer = nil
		m.pendingLink = nil
	case "j", "down", "tab":
		if len(lo.items) > 0 {
			lo.idx = (lo.idx + 1) % len(lo.items)
		}
	case "k", "up", "shift+tab":
		if len(lo.items) > 0 {
			lo.idx = (lo.idx + len(lo.items) - 1) % len(lo.items)
		}
	case "enter":
		if lo.idx < len(lo.items) {
			return m.runLinkOfferItem(lo.items[lo.idx])
		}
	default:
		if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			if i := int(k[0] - '1'); i < len(lo.items) {
				return m.runLinkOfferItem(lo.items[i])
			}
		}
	}
	return nil
}

func (m *Model) runLinkOfferItem(it linkOfferItem) tea.Cmd {
	lo := m.linkOffer
	m.linkOffer = nil
	r := lo.result
	label, _ := lo.title()
	switch it.action {
	case linkPlayNow, linkPlayNext, linkAddToQueue:
		msg := searchCollectionTracksMsg{label: label, tracks: r.Tracks,
			play: it.action == linkPlayNow, playNext: it.action == linkPlayNext}
		return func() tea.Msg { return msg }
	case linkOpenArtist:
		m.activePanel = -1
		m.mode = modeSearch
		return m.openArtistCmd(r.Artists[0])
	case linkShowInSearch:
		m.activePanel = -1
		m.mode = modeSearch
		m.searchQuery = lo.link.URL
		m.searchCursor = len([]rune(lo.link.URL))
		m.searchGen++
		m.search.SetResults(r, false, nil)
	}
	return nil
}

// renderLinkOffer draws the :link window.
func (m *Model) renderLinkOffer() string {
	lo := m.linkOffer
	innerW := min(52, max(20, m.width-10))
	muted := styles.QueueItemMuted
	sep := muted.Render(strings.Repeat("─", innerW))

	title, sub := lo.title()
	if lo.loading || lo.err != nil {
		title, sub = serviceNames[lo.link.Service], lo.link.URL
	}
	lines := []string{
		styles.NowPlayingTitlePlaying.Render(truncateStr(title, innerW)),
		muted.Render(truncateStr(sub, innerW)),
		sep,
	}
	switch {
	case lo.loading:
		lines = append(lines, muted.Render(i18n.T("looking up the link…")))
	case lo.err != nil:
		// Errors (yt-dlp's especially) are long: up to three lines.
		msg := []rune(lo.err.Error())
		for i := 0; i < 3 && len(msg) > 0; i++ {
			n := min(len(msg), innerW)
			if i == 2 {
				lines = append(lines, styles.ErrorStyle.Render(truncateStr(string(msg), innerW)))
				break
			}
			lines = append(lines, styles.ErrorStyle.Render(string(msg[:n])))
			msg = msg[n:]
		}
	default:
		for i, it := range lo.items {
			num := styles.KeyName.Render(fmt.Sprintf("%d ", i+1))
			label := truncateStr(it.label, innerW-4)
			if i == lo.idx {
				lines = append(lines, styles.Playing.Render("▸ ")+num+styles.Selected.Render(label))
			} else {
				lines = append(lines, "  "+num+styles.QueueItem.Render(label))
			}
		}
	}
	lines = append(lines, sep, muted.Render(i18n.T("↑↓ choose · enter do · esc close")))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorBorderFocus).
		Padding(0, 1).
		Width(innerW + 4).
		Render(strings.Join(lines, "\n"))
}
