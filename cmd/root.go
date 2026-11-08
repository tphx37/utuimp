package cmd

import (
	"fmt"
	"github.com/tphx37/utuimp/internal/i18n"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/tphx37/utuimp/internal/assets"
	"github.com/tphx37/utuimp/internal/config"
	demoPlayer "github.com/tphx37/utuimp/internal/player/demo"
	demoProvider "github.com/tphx37/utuimp/internal/provider/demo"
	"github.com/tphx37/utuimp/internal/tui"
	"github.com/tphx37/utuimp/internal/tui/styles"
)

var cfgFile string
var debug bool
var memProfiling bool
var demo bool
var noUpdate bool
var local bool
var musicDir string
var noDiscord bool
var newWindow bool
var useApple bool
var relogin []string

var rootCmd = &cobra.Command{
	Use:   "utuimp",
	Short: "utuimp — universal terminal user interface media player",
	Long:  "utuimp — universal terminal user interface media player.\n\nRun without arguments to open the TUI.",
	RunE:  runTUI,
	// A runtime error (e.g. a crash report) isn't a usage mistake: print just
	// the error, once (Execute prints it).
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/utuimp/config.json; Windows: %APPDATA%\\utuimp\\config.json)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&memProfiling, "mem-profiling", false, "show live RSS in the header")
	rootCmd.PersistentFlags().BoolVar(&demo, "demo", false, "run with built-in fake data — no Apple account or internet required")
	rootCmd.PersistentFlags().BoolVar(&noUpdate, "no-update", false, "skip automatic update check on startup")
	rootCmd.PersistentFlags().BoolVar(&local, "local", false, "start on the Local files tab (music_dir, default ~/Music)")
	rootCmd.PersistentFlags().StringVar(&musicDir, "music-dir", "", "path to your music directory (saved to config)")
	rootCmd.PersistentFlags().BoolVar(&noDiscord, "no-discord", false, "disable Discord Rich Presence")
	rootCmd.Flags().BoolVar(&newWindow, "window", false, "open utuimp in its own terminal window (with the utuimp icon)")
	rootCmd.PersistentFlags().BoolVar(&useApple, "apple", false, "start on the Apple Music tab")
	rootCmd.PersistentFlags().StringSliceVar(&relogin, "relogin", nil, "sign out and sign in again (switch accounts): --relogin=yandex, --relogin=apple, --relogin=spotify, or --relogin for all")
	rootCmd.PersistentFlags().Lookup("relogin").NoOptDefVal = "all"
}

// setupDesktopCmd installs the app-menu launcher and icon (run by make install).
var setupDesktopCmd = &cobra.Command{
	Use:    "setup-desktop",
	Short:  "Install the utuimp launcher and icon for the desktop",
	Hidden: true,
	Run: func(*cobra.Command, []string) {
		assets.InstallIcon()
		assets.InstallDesktopEntry()
	},
}

func init() { rootCmd.AddCommand(setupDesktopCmd) }

// signOut clears the sign-in of the named services ("yandex", "apple",
// "all") and returns their display names.
func signOut(cfg *config.Config, services []string) ([]string, error) {
	var names []string
	for _, svc := range services {
		switch strings.ToLower(strings.TrimSpace(svc)) {
		case "all", "":
			cfg.AppleUserToken = ""
			cfg.YandexToken, cfg.YandexRefreshToken, cfg.YandexTokenExpiry = "", "", 0
			cfg.SpotifyToken, cfg.SpotifyRefreshToken, cfg.SpotifyTokenExpiry = "", "", 0
			names = append(names, "Yandex Music", "Apple Music", "Spotify")
		case "spotify", "sp":
			cfg.SpotifyToken, cfg.SpotifyRefreshToken, cfg.SpotifyTokenExpiry = "", "", 0
			names = append(names, "Spotify")
		case "yandex", "ym":
			cfg.YandexToken, cfg.YandexRefreshToken, cfg.YandexTokenExpiry = "", "", 0
			names = append(names, "Yandex Music")
		case "apple", "am":
			cfg.AppleUserToken = ""
			names = append(names, "Apple Music")
		default:
			return nil, fmt.Errorf("--relogin: unknown service %q (use yandex, apple, spotify or all)", svc)
		}
	}
	return names, nil
}

func runTUI(_ *cobra.Command, _ []string) error {
	if newWindow {
		return openInOwnWindow(os.Args[1:])
	}
	cfgPath, err := config.ConfigPath(cfgFile)
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.Language != "" {
		i18n.Set(cfg.Language)
	} else {
		i18n.Set(i18n.Detect())
	}
	// First run: a few quick questions (utuimp setup changes them later).
	if !cfg.SetupDone && !demo && isInteractive() {
		if err := runSetup(cfg, os.Stdin, os.Stdout); err != nil {
			return err
		}
	}

	audioBitrateKbps, err := cfg.AudioBitrateKbps()
	if err != nil {
		return err
	}

	opts := tui.Options{MemProfiling: memProfiling}

	// Apply theme before creating any TUI model so all panels pick up the palette.
	theme, themeErr := styles.LoadTheme(cfg.Theme, filepath.Dir(cfgPath))
	if themeErr != nil && debug {
		fmt.Fprintf(os.Stderr, "debug: theme: %v (falling back to default)\n", themeErr)
	}
	styles.Apply(theme)

	if musicDir != "" {
		cfg.MusicDir = musicDir
		if saveErr := cfg.Save(cfgFile); saveErr != nil && debug {
			fmt.Fprintf(os.Stderr, "debug: saving music dir: %v\n", saveErr)
		}
	}

	if len(relogin) > 0 {
		names, err := signOut(cfg, relogin)
		if err != nil {
			return err
		}
		if err := cfg.Save(cfgFile); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Signed out of %s; you'll be asked to sign in again.\n", strings.Join(names, " and "))
	}

	if demo {
		iconPath := assets.InstallIcon()
		assets.InstallDesktopEntry()
		p := demoPlayer.New()
		dp := demoProvider.Provider{}
		opts.IconPath = iconPath
		opts.Backend = "Demo mode · built-in fake tracks, no credentials required"
		prog := tea.NewProgram(tui.New(cfg, dp, p, opts))
		restoreStderr := captureStderr()
		_, err = prog.Run()
		restoreStderr()
		return err
	}

	opts.IconPath = assets.InstallIcon()
	assets.InstallDesktopEntry()

	initial := ""
	switch {
	case useApple:
		initial = "apple"
	case local:
		initial = "local"
	}
	return runServices(cfg, opts, audioBitrateKbps, initial)
}
