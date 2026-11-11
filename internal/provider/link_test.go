package provider

import "testing"

func TestParseLink(t *testing.T) {
	cases := []struct {
		in      string
		service string
		kind    LinkKind
		id      string
	}{
		{"https://open.spotify.com/track/4uLU6hMCjMI75M1A2tKUQC?si=abc", "spotify", LinkTrack, "4uLU6hMCjMI75M1A2tKUQC"},
		{"https://open.spotify.com/intl-ru/album/1DFixLWuPkv3KT3TnV35m3", "spotify", LinkAlbum, "1DFixLWuPkv3KT3TnV35m3"},
		{"open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M", "spotify", LinkPlaylist, "37i9dQZF1DXcBWIGoYBM5M"},
		{"spotify:track:4uLU6hMCjMI75M1A2tKUQC", "spotify", LinkTrack, "4uLU6hMCjMI75M1A2tKUQC"},
		{"https://music.apple.com/us/album/blinding-lights/1499378108?i=1499378615", "apple", LinkTrack, "1499378615"},
		{"https://music.apple.com/ru/album/after-hours/1499378108", "apple", LinkAlbum, "1499378108"},
		{"https://music.apple.com/us/song/blinding-lights/1499378615", "apple", LinkTrack, "1499378615"},
		{"https://music.apple.com/us/playlist/todays-hits/pl.f4d106fed2bd41149aaacabb233eb5eb", "apple", LinkPlaylist, "pl.f4d106fed2bd41149aaacabb233eb5eb"},
		{"https://music.apple.com/us/artist/the-weeknd/479756766", "apple", LinkArtist, "479756766"},
		{"https://music.yandex.ru/album/3192570/track/354093?utm_source=x", "yandex", LinkTrack, "354093:3192570"},
		{"https://music.yandex.com/track/354093", "yandex", LinkTrack, "354093"},
		{"https://music.yandex.ru/album/3192570", "yandex", LinkAlbum, "3192570"},
		{"https://music.yandex.ru/users/music-blog/playlists/2620", "yandex", LinkPlaylist, "music-blog:2620"},
		{"https://music.yandex.ru/artist/41075", "yandex", LinkArtist, "41075"},
		{"https://music.youtube.com/watch?v=dQw4w9WgXcQ&list=RDAMVM", "youtube", LinkTrack, "dQw4w9WgXcQ"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube", LinkTrack, "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ?si=x", "youtube", LinkTrack, "dQw4w9WgXcQ"},
		{"https://music.youtube.com/playlist?list=OLAK5uy_abc", "youtube", LinkAlbum, "OLAK5uy_abc"},
		{"https://music.youtube.com/playlist?list=PLabc", "youtube", LinkPlaylist, "PLabc"},
		{"https://music.youtube.com/browse/MPREb_abc", "youtube", LinkAlbum, "MPREb_abc"},
		{"https://music.youtube.com/channel/UCabc", "youtube", LinkArtist, "UCabc"},
		{"https://soundcloud.com/artist-name/track-name?in=x", "soundcloud", LinkTrack, "artist-name/track-name"},
		{"https://soundcloud.com/artist-name/sets/my-set", "soundcloud", LinkPlaylist, "artist-name/sets/my-set"},
		{"https://soundcloud.com/artist-name", "soundcloud", LinkArtist, "artist-name"},
		{"https://on.soundcloud.com/AbCd", "soundcloud", LinkTrack, "https://on.soundcloud.com/AbCd"},
	}
	for _, c := range cases {
		l, ok := ParseLink(c.in)
		if !ok || l.Service != c.service || l.Kind != c.kind || l.ID != c.id {
			t.Errorf("ParseLink(%q) = %+v, %v; want %s %s %s", c.in, l, ok, c.service, c.kind, c.id)
		}
	}
	for _, q := range []string{"", "the weeknd", "AC/DC", "https://example.com/track/1", "soundcloud.com/discover",
		"https://open.spotify.com/user/abc", "music.apple.com"} {
		if l, ok := ParseLink(q); ok {
			t.Errorf("ParseLink(%q) = %+v, want no link", q, l)
		}
	}
}
