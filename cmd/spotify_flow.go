package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/openurl"
	"github.com/tphx37/utuimp/internal/player/spotifyconnect"
	"github.com/tphx37/utuimp/internal/provider/spotify"
	"github.com/tphx37/utuimp/internal/tui"
)

func saveSpotifyToken(cfg *config.Config, tok spotify.Token) {
	cfg.SpotifyToken = tok.AccessToken
	if tok.RefreshToken != "" {
		cfg.SpotifyRefreshToken = tok.RefreshToken
	}
	cfg.SpotifyTokenExpiry = 0
	if !tok.Expiry.IsZero() {
		cfg.SpotifyTokenExpiry = tok.Expiry.Unix()
	}
	_ = cfg.Save(cfgFile)
}

// librespotInstallHint says how to get librespot on this system.
func librespotInstallHint() string {
	return i18n.T("install librespot (Arch: sudo pacman -S librespot; others: cargo install librespot)")
}

func spotifyCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "utuimp", "librespot")
}

// startSpotifyEngine signs in (approve in the browser), starts librespot as
// utuimp's own Spotify device, and connects the player to it.
func startSpotifyEngine(cfg *config.Config, send func(tea.Msg), env *engineEnv, idx int) {
	status := func(s string) { send(tui.InitStatusMsg(s)) }
	clientID := cfg.SpotifyClientID
	if clientID == "" {
		clientID = spotify.DefaultClientID
	}
	if cfg.SpotifyToken == "" {
		status(i18n.T("Spotify sign-in: approve access in your browser…"))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		tok, err := spotify.Login(ctx, clientID, openurl.Open)
		cancel()
		if err != nil {
			send(tui.InitErrMsg{Err: err})
			return
		}
		saveSpotifyToken(cfg, *tok)
	}
	tok := spotify.Token{AccessToken: cfg.SpotifyToken, RefreshToken: cfg.SpotifyRefreshToken}
	if cfg.SpotifyTokenExpiry > 0 {
		tok.Expiry = time.Unix(cfg.SpotifyTokenExpiry, 0)
	}
	client := spotify.NewClient(clientID, tok, func(t spotify.Token) { saveSpotifyToken(cfg, t) })

	status(i18n.T("Connecting to Spotify…"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var me struct {
		DisplayName string `json:"display_name"`
		Product     string `json:"product"`
	}
	if err := client.Do(ctx, http.MethodGet, "/me", nil, nil, &me); err != nil {
		if errors.Is(err, spotify.ErrClientLimited) {
			err = errors.New(i18n.T("Spotify is blocking the shared sign-in app right now. Use your own (free, one minute): run utuimp setup and follow the Spotify steps."))
		}
		send(tui.InitErrMsg{Err: err})
		return
	}
	prov := spotify.New(client)
	plyr := spotifyconnect.New(prov, func(msg string) { send(tui.DebugLogMsg(msg)) })
	env.addCleanup(func() { _ = plyr.Close() })

	backend := "Spotify · " + me.DisplayName
	notice := ""
	switch {
	case me.Product != "premium":
		notice = i18n.T("Spotify Premium is needed to play music in utuimp.")
	default:
		bin, err := spotifyconnect.FindLibrespot(cfg.LibrespotPath)
		if err != nil {
			notice = i18n.T("To play Spotify here,") + " " + librespotInstallHint() +
				i18n.T(". Until then utuimp controls your other Spotify devices.")
			break
		}
		status(i18n.T("Starting Spotify audio…"))
		cache := spotifyCacheDir()
		// librespot signs in with the shared app's token once and then
		// keeps its own credentials. With the user's own app, that token
		// takes one more approval.
		access := ""
		if !spotifyconnect.HasCredentials(cache) {
			if clientID == spotify.DefaultClientID {
				if access, err = client.AccessToken(ctx); err != nil {
					send(tui.InitErrMsg{Err: err})
					return
				}
			} else {
				status(i18n.T("Spotify audio: approve access once more in your browser…"))
				lctx, lcancel := context.WithTimeout(context.Background(), 5*time.Minute)
				tok, lerr := spotify.Login(lctx, spotify.DefaultClientID, openurl.Open)
				lcancel()
				if lerr != nil {
					notice = lerr.Error()
					break
				}
				access = tok.AccessToken
			}
		}
		logPath := filepath.Join(filepath.Dir(cache), "librespot.log")
		ls, err := spotifyconnect.StartLibrespot(bin, access, cache, logPath, 320)
		if err != nil {
			notice = err.Error()
			break
		}
		env.addCleanup(ls.Stop)
		wctx, wcancel := context.WithTimeout(context.Background(), 25*time.Second)
		id, err := spotifyconnect.WaitForDevice(wctx, client, ls)
		wcancel()
		if err != nil {
			notice = err.Error() + " (" + logPath + ")"
			if errors.Is(err, context.DeadlineExceeded) {
				notice = i18n.T("librespot started but did not connect; see") + " " + logPath
			}
			break
		}
		plyr.SetDevice(id)
		backend += " · librespot"
	}
	env.mux.Set(idx, plyr)
	send(tui.EngineReadyMsg{Player: plyr, Provider: prov, Backend: backend, Notice: notice})
}
