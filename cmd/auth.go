package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tphx37/utuimp/internal/auth"
	"github.com/tphx37/utuimp/internal/config"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your music provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		return auth.Login(cfg)
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and clear saved tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		return auth.Logout(cfg)
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if cfg.AppleUserToken == "" {
			fmt.Println("Not authenticated.")
			return nil
		}
		sf := cfg.StoreFront
		if sf == "" {
			sf = "auto-detected"
		}
		fmt.Printf("Authenticated with Apple Music (storefront: %s)\n", sf)
		switch {
		case auth.BuiltIn():
			fmt.Println("Developer token: built into this build")
		case cfg.AppleTokenSource == config.AppleTokenVibez:
			fmt.Println("Developer token: borrowed from vibez (github.com/simonepelosi/vibez)")
		case cfg.AppleTokenSource == config.AppleTokenOwn:
			fmt.Println("Developer token: your own (apple_developer_token)")
		default:
			fmt.Println("Developer token: not chosen yet, run `utuimp setup services`")
		}
		return nil
	},
}

var authYandexCmd = &cobra.Command{
	Use:   "yandex",
	Short: "Sign in to Yandex Music",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		return yandexSignIn(cfg, func(s string) { fmt.Println("  " + s) })
	},
}

var authYandexLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Forget the Yandex Music sign-in",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		cfg.YandexToken, cfg.YandexRefreshToken, cfg.YandexTokenExpiry = "", "", 0
		if err := cfg.Save(cfgFile); err != nil {
			return err
		}
		fmt.Println("Signed out of Yandex Music.")
		return nil
	},
}

func init() {
	authYandexCmd.AddCommand(authYandexLogoutCmd)
	authCmd.AddCommand(authYandexCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}
