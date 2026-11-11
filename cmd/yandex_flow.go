package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/tphx37/utuimp/internal/i18n"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/openurl"
	"github.com/tphx37/utuimp/internal/player/ystream"
	"github.com/tphx37/utuimp/internal/provider/yandex"
	"github.com/tphx37/utuimp/internal/signin"
	"github.com/tphx37/utuimp/internal/tui"
)

// yandexSignIn runs the device-code sign-in; status shows the instructions
// (in the terminal or on the loading screen).
func yandexSignIn(cfg *config.Config, status func(string)) error {
	ctx := context.Background()
	dc, err := yandex.RequestDeviceCode(ctx)
	if err != nil {
		return fmt.Errorf("yandex sign-in: %w", err)
	}
	status(fmt.Sprintf(i18n.T("Yandex sign-in: open %s and enter the code %s"), dc.VerificationURL, dc.UserCode))
	// The browser gets utuimp's own page with the code and a button to
	// Yandex's; it says when the sign-in is over. Without it (no free
	// port), Yandex's page is opened as before.
	page, finish, perr := signin.ServeWaiting(signin.Page{
		Service: "Yandex Music", Colors: signin.Yandex,
		Text:   "Open Yandex's page and enter this code there to confirm the sign-in.",
		Code:   dc.UserCode,
		Button: "Open Yandex", ButtonURL: dc.VerificationURL, NewTab: true,
	})
	if perr != nil {
		page, finish = dc.VerificationURL, func(error) {}
	}
	_ = openurl.Open(page)
	tok, err := yandex.WaitForToken(ctx, dc)
	finish(err)
	if err != nil {
		return err
	}
	saveYandexToken(cfg, tok)
	status(i18n.T("✓ Signed in to Yandex Music"))
	return nil
}

func saveYandexToken(cfg *config.Config, tok *yandex.Token) {
	cfg.YandexToken = tok.AccessToken
	if tok.RefreshToken != "" {
		cfg.YandexRefreshToken = tok.RefreshToken
	}
	cfg.YandexTokenExpiry = 0
	if !tok.Expiry.IsZero() {
		cfg.YandexTokenExpiry = tok.Expiry.Unix()
	}
	if err := cfg.Save(cfgFile); err != nil {
		fmt.Fprintf(os.Stderr, "warning: saving the Yandex token: %v\n", err)
	}
}

// yandexAccount returns an authenticated client and the user id, refreshing
// the token when it is close to expiry and signing in again when needed.
func yandexAccount(cfg *config.Config, status func(string)) (*yandex.Client, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if cfg.YandexToken == "" {
		if err := yandexSignIn(cfg, status); err != nil {
			return nil, "", err
		}
	}
	if exp := cfg.YandexTokenExpiry; exp > 0 && cfg.YandexRefreshToken != "" &&
		time.Until(time.Unix(exp, 0)) < 7*24*time.Hour {
		if tok, err := yandex.Refresh(ctx, cfg.YandexRefreshToken); err == nil {
			saveYandexToken(cfg, tok)
		}
	}
	c := yandex.NewClient(cfg.YandexToken)
	uid, err := c.AccountUID(ctx)
	if errors.Is(err, yandex.ErrUnauthorized) {
		if cfg.YandexRefreshToken != "" {
			if tok, rerr := yandex.Refresh(ctx, cfg.YandexRefreshToken); rerr == nil {
				saveYandexToken(cfg, tok)
				c = yandex.NewClient(cfg.YandexToken)
				uid, err = c.AccountUID(ctx)
			}
		}
		if errors.Is(err, yandex.ErrUnauthorized) {
			status(i18n.T("The Yandex Music session has expired"))
			if err := yandexSignIn(cfg, status); err != nil {
				return nil, "", err
			}
			c = yandex.NewClient(cfg.YandexToken)
			uid, err = c.AccountUID(ctx)
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf("yandex music account: %w", err)
	}
	return c, uid, nil
}

// startYandexEngine signs in and brings up the Yandex Music player.
func startYandexEngine(cfg *config.Config, send func(tea.Msg), env *engineEnv, idx int) {
	status := func(s string) { send(tui.InitStatusMsg(s)) }
	status(i18n.T("Connecting to Yandex Music…"))
	client, uid, err := yandexAccount(cfg, status)
	if err != nil {
		send(tui.InitErrMsg{Err: err})
		return
	}
	prov := yandex.New(client, uid)
	plyr, err := ystream.New(prov, func(msg string) { send(tui.DebugLogMsg(msg)) })
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("audio engine: %w", err)})
		return
	}
	env.addCleanup(func() { _ = plyr.Close() })
	env.mux.Set(idx, plyr)
	send(tui.EngineReadyMsg{Player: plyr, Provider: prov, Backend: "Yandex Music · GStreamer"})
}
