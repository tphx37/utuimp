package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// Keeping settings across versions.
//
// A build only knows the fields of its own Config. Run an older build (an
// installed release, a rebuild of an older commit) and its Save would drop
// every newer field: the SoundCloud app's ID and secret, the YouTube cookie
// setting. Two things prevent that:
//
//   - Load keeps the keys it doesn't know and Save writes them back as they
//     were, so from now on no build loses another's settings.
//   - Credentials (IDs, secrets, tokens) are also mirrored into
//     credentials.json next to the config. Load fills any of them missing
//     from config.json from there, which also recovers them after a build
//     that predates the first point rewrote the config.

// credentialKeys are the config keys mirrored into credentials.json.
var credentialKeys = []string{ //nolint:gochecknoglobals // fixed list
	"spotify_client_id", "spotify_token", "spotify_refresh_token", "spotify_token_expiry",
	"soundcloud_client_id", "soundcloud_client_secret", "soundcloud_token",
	"soundcloud_refresh_token", "soundcloud_token_expiry",
	"yandex_token", "yandex_refresh_token", "yandex_token_expiry",
	"apple_user_token",
	"lastfm_api_key", "lastfm_api_secret", "lastfm_session_key",
	"youtube_cookies", "ytdlp_path", "ffmpeg_path", "librespot_path",
}

const credentialsFile = "credentials.json"

func credentialsPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), credentialsFile)
}

// knownKeys lists the JSON keys of Config's fields.
func knownKeys() map[string]bool {
	out := map[string]bool{}
	t := reflect.TypeFor[Config]()
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("json")
		if name, _, _ := strings.Cut(tag, ","); name != "" && name != "-" {
			out[name] = true
		}
	}
	return out
}

// absorb records the unknown keys of the file's raw JSON and fills
// credentials missing from it from credentials.json.
func (c *Config) absorb(data []byte, path string) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	known := knownKeys()
	for k, v := range raw {
		if !known[k] {
			if c.extra == nil {
				c.extra = map[string]json.RawMessage{}
			}
			c.extra[k] = v
		}
	}
	creds, err := os.ReadFile(credentialsPath(path)) //nolint:gosec // next to the config
	if err != nil {
		return
	}
	var saved map[string]json.RawMessage
	if json.Unmarshal(creds, &saved) != nil {
		return
	}
	for k, v := range saved {
		if _, inFile := raw[k]; inFile {
			continue
		}
		if known[k] {
			one, _ := json.Marshal(map[string]json.RawMessage{k: v})
			_ = json.Unmarshal(one, c)
		} else {
			if c.extra == nil {
				c.extra = map[string]json.RawMessage{}
			}
			c.extra[k] = v
		}
	}
}

// marshal is the config's JSON: its own fields in order, then the unknown
// keys it was loaded with.
func (c *Config) marshal() ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil || len(c.extra) == 0 {
		return data, err
	}
	known := knownKeys()
	keys := make([]string, 0, len(c.extra))
	for k := range c.extra {
		if !known[k] {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return data, nil
	}
	sort.Strings(keys)
	var b bytes.Buffer
	body := bytes.TrimRight(bytes.TrimSuffix(bytes.TrimSpace(data), []byte("}")), " \n")
	b.Write(body)
	for i, k := range keys {
		if i > 0 || !bytes.HasSuffix(body, []byte("{")) {
			b.WriteByte(',')
		}
		kj, _ := json.Marshal(k)
		var v bytes.Buffer
		if json.Indent(&v, c.extra[k], "  ", "  ") != nil {
			v.Reset()
			v.Write(c.extra[k])
		}
		b.WriteString("\n  ")
		b.Write(kj)
		b.WriteString(": ")
		b.Write(v.Bytes())
	}
	b.WriteString("\n}")
	return b.Bytes(), nil
}

// credentialsJSON picks the credential keys out of the config's JSON.
func credentialsJSON(configJSON []byte) []byte {
	var all map[string]json.RawMessage
	if json.Unmarshal(configJSON, &all) != nil {
		return nil
	}
	out := map[string]json.RawMessage{}
	for _, k := range credentialKeys {
		if v, ok := all[k]; ok {
			out[k] = v
		}
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return b
}
