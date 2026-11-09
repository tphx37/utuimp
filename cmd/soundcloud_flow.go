package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/openurl"
	"github.com/tphx37/utuimp/internal/player/ystream"
	"github.com/tphx37/utuimp/internal/provider/localstore"
	"github.com/tphx37/utuimp/internal/provider/soundcloud"
	"github.com/tphx37/utuimp/internal/tui"
)

func saveSoundCloudToken(cfg *config.Config, tok soundcloud.Token) {
	cfg.SoundCloudToken = tok.AccessToken
	if tok.RefreshToken != "" {
		cfg.SoundCloudRefreshToken = tok.RefreshToken
	}
	cfg.SoundCloudTokenExpiry = 0
	if !tok.Expiry.IsZero() {
		cfg.SoundCloudTokenExpiry = tok.Expiry.Unix()
	}
	if err := cfg.Save(cfgFile); err != nil {
		fmt.Fprintf(os.Stderr, "warning: saving the SoundCloud token: %v\n", err)
	}
}

// startSoundCloudEngine brings up SoundCloud: through the official API
// when the user has their own app (Client ID and Secret), else through
// yt-dlp like YouTube Music.
func startSoundCloudEngine(cfg *config.Config, send func(tea.Msg), env *engineEnv, idx int) {
	app := soundcloud.App{ClientID: cfg.SoundCloudClientID, ClientSecret: cfg.SoundCloudClientSecret}
	if app.ClientID == "" || app.ClientSecret == "" {
		startSoundCloudPublic(cfg, send, env, idx)
		return
	}
	startSoundCloudAPI(cfg, app, send, env, idx)
}

// startSoundCloudPublic plays SoundCloud through yt-dlp, no account needed.
func startSoundCloudPublic(cfg *config.Config, send func(tea.Msg), env *engineEnv, idx int) {
	send(tui.InitStatusMsg(i18n.T("Starting SoundCloud…")))
	ytdlpPath, err := findTool(cfg.YtDlpPath, "yt-dlp")
	if err != nil {
		send(tui.InitErrMsg{Err: errors.New(i18n.T("SoundCloud needs yt-dlp: ") + ytInstallHint())})
		return
	}
	ffmpeg, err := findTool(cfg.FFmpegPath, "ffmpeg")
	if err != nil {
		send(tui.InitErrMsg{Err: errors.New(i18n.T("SoundCloud needs ffmpeg: ") + ytInstallHint())})
		return
	}
	store, err := localstore.Open(localstore.DefaultPath("library-soundcloud"))
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("soundcloud library: %w", err)})
		return
	}
	prov := soundcloud.NewPublic(ytdlpPath, ffmpeg, store)
	plyr, err := ystream.New(prov, func(msg string) { send(tui.DebugLogMsg(msg)) }, ystream.WithName("soundcloud"))
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("audio engine: %w", err)})
		return
	}
	env.addCleanup(func() { _ = plyr.Close() })
	env.mux.Set(idx, plyr)
	send(tui.EngineReadyMsg{Player: plyr, Provider: prov, Backend: "SoundCloud · yt-dlp · GStreamer"})
}

// startSoundCloudAPI signs in with the user's own SoundCloud app.
func startSoundCloudAPI(cfg *config.Config, app soundcloud.App, send func(tea.Msg), env *engineEnv, idx int) {
	status := func(s string) { send(tui.InitStatusMsg(s)) }
	login := func() error {
		status(i18n.T("SoundCloud sign-in: approve access in your browser…"))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		tok, err := soundcloud.Login(ctx, app, openurl.Open)
		if err != nil {
			return err
		}
		saveSoundCloudToken(cfg, *tok)
		return nil
	}
	if cfg.SoundCloudToken == "" {
		if err := login(); err != nil {
			send(tui.InitErrMsg{Err: err})
			return
		}
	}
	newClient := func() *soundcloud.Client {
		tok := soundcloud.Token{AccessToken: cfg.SoundCloudToken, RefreshToken: cfg.SoundCloudRefreshToken}
		if cfg.SoundCloudTokenExpiry > 0 {
			tok.Expiry = time.Unix(cfg.SoundCloudTokenExpiry, 0)
		}
		return soundcloud.NewClient(app, tok, func(t soundcloud.Token) { saveSoundCloudToken(cfg, t) })
	}
	client := newClient()

	status(i18n.T("Connecting to SoundCloud…"))
	me := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var u struct {
			Username string `json:"username"`
		}
		return client.Do(ctx, http.MethodGet, "/me", nil, nil, &u)
	}
	err := me()
	if errors.Is(err, soundcloud.ErrUnauthorized) {
		status(i18n.T("The SoundCloud session has expired"))
		if err = login(); err == nil {
			client = newClient()
			err = me()
		}
	}
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("soundcloud: %w", err)})
		return
	}
	ffmpeg, _ := findTool(cfg.FFmpegPath, "ffmpeg") // only for AAC-only tracks
	prov := soundcloud.New(client, ffmpeg)
	plyr, err := ystream.New(prov, func(msg string) { send(tui.DebugLogMsg(msg)) }, ystream.WithName("soundcloud"))
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("audio engine: %w", err)})
		return
	}
	env.addCleanup(func() { _ = plyr.Close() })
	env.mux.Set(idx, plyr)
	send(tui.EngineReadyMsg{Player: plyr, Provider: prov, Backend: "SoundCloud · GStreamer"})
}
