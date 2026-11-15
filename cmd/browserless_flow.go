package cmd

import (
	"fmt"
	"os"

	"github.com/tphx37/utuimp/internal/i18n"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/auth"
	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/lastfm"
	"github.com/tphx37/utuimp/internal/player/browserless"
	"github.com/tphx37/utuimp/internal/provider/apple"
	"github.com/tphx37/utuimp/internal/tui"
	"github.com/tphx37/utuimp/internal/updater"
	"github.com/tphx37/utuimp/internal/version"
)

// startAppleEngine resolves the developer token, signs in to Apple Music if
// needed and brings up the browserless (Widevine + GStreamer) player.
func startAppleEngine(cfg *config.Config, send func(tea.Msg), env *engineEnv, idx int, audioBitrateKbps int) {
	status := func(s string) { send(tui.InitStatusMsg(s)) }
	saveCfg := func(what string) {
		if err := cfg.Save(""); err != nil && debug {
			fmt.Fprintf(os.Stderr, "debug: saving %s: %v\n", what, err)
		}
	}
	onUserToken := func(token string) {
		cfg.AppleUserToken = token
		saveCfg("user token")
	}
	onStorefront := func(sf string) {
		if sf != "" && sf != cfg.StoreFront {
			cfg.StoreFront = sf
			saveCfg("storefront")
		}
	}

	if exe := updater.CheckAndUpdate(version.Version, noUpdate, status); exe != "" {
		env.restart(exe)
		send(tui.RestartMsg{})
		return
	}

	status(i18n.T("Preparing Apple Music…"))
	tokenErr := auth.ResolveDeveloperToken(cfg)
	lastfm.ApplyEmbedded(cfg)
	for _, msg := range auth.BrokerLogs() {
		send(tui.DebugLogMsg(msg))
	}
	if cfg.AppleDeveloperToken == "" {
		err := fmt.Errorf("apple developer token not set (set apple_developer_token in the config)")
		if tokenErr != nil {
			err = tokenErr
		}
		send(tui.InitErrMsg{Err: err})
		return
	}

	devTokenRejected := false
	if cfg.AppleUserToken != "" {
		status(i18n.T("Checking Apple Music session..."))
		switch st, sf := auth.CheckTokens(cfg.AppleDeveloperToken, cfg.AppleUserToken); st {
		case auth.UserTokenRejected:
			status(i18n.T("Session expired - re-authenticating..."))
			cfg.AppleUserToken = ""
			saveCfg("config")
		case auth.DeveloperTokenRejected:
			devTokenRejected = true
			res := updater.UpdateNow(version.Version, noUpdate, status)
			if res.Outcome == updater.OutcomeInstalled {
				env.restart(res.Exe)
				send(tui.RestartMsg{})
				return
			}
			msg := auth.DeveloperTokenStatus
			if advice := res.Advice(); advice != "" {
				msg = auth.DeveloperTokenPrefix + " - " + advice
			}
			status(msg)
		case auth.TokensValid:
			onStorefront(sf)
		}
	}

	if cfg.AppleUserToken == "" {
		send(tui.AuthPendingMsg{})
		status(i18n.T("Authorizing with Apple Music (see your browser)..."))
		if err := auth.LoginWithStatus(cfg, status); err != nil {
			send(tui.InitErrMsg{Err: fmt.Errorf("authentication: %w", err)})
			return
		}
		onUserToken(cfg.AppleUserToken)
		if _, sf := auth.CheckTokens(cfg.AppleDeveloperToken, cfg.AppleUserToken); sf != "" {
			onStorefront(sf)
		}
	}

	if !devTokenRejected {
		status(i18n.T("Starting audio engine..."))
	}
	prov := apple.New(cfg)
	blPlayer, err := browserless.New(cfg, prov, func(msg string) {
		if !devTokenRejected {
			status(msg)
		}
	})
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("audio engine: %w", err)})
		return
	}
	env.addCleanup(func() { _ = blPlayer.Close() })
	_ = blPlayer.SetAudioBitrate(audioBitrateKbps)
	blPlayer.OnSessionExpired = func() {
		send(tui.SessionExpiredMsg{})
		go func() {
			if err := auth.LoginWithStatus(cfg, status); err != nil {
				send(tui.InitErrMsg{Err: fmt.Errorf("re-auth: %w", err)})
				return
			}
			onUserToken(cfg.AppleUserToken)
			if _, sf := auth.CheckTokens(cfg.AppleDeveloperToken, cfg.AppleUserToken); sf != "" {
				onStorefront(sf)
			}
			blPlayer.ResetSessionExpired()
			send(tui.SessionRestoredMsg{})
		}()
	}

	env.mux.Set(idx, blPlayer)
	setupBrowserlessPlatform(blPlayer, func(msg string) { send(tui.DebugLogMsg(msg)) })
	send(tui.EngineReadyMsg{
		Player:   blPlayer,
		Provider: prov,
		Backend:  browserlessBackendLabel(audioBitrateKbps),
	})
}
