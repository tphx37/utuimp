package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"

	"github.com/tphx37/utuimp/internal/i18n"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/tphx37/utuimp/internal/assets"
	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/gfx"
	demoPlayer "github.com/tphx37/utuimp/internal/player/demo"
	demoProvider "github.com/tphx37/utuimp/internal/provider/demo"
	"github.com/tphx37/utuimp/internal/tui"
	"github.com/tphx37/utuimp/internal/tui/art"
	"github.com/tphx37/utuimp/internal/tui/styles"
	"github.com/tphx37/utuimp/internal/updater"
	"github.com/tphx37/utuimp/internal/version"
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
	stop := startCPUProfile()
	err := rootCmd.Execute()
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// startCPUProfile records where the CPU time goes when UTUIMP_CPUPROFILE
// names a file, for "go tool pprof"; the returned func writes it out.
func startCPUProfile() (stop func()) {
	path := os.Getenv("UTUIMP_CPUPROFILE")
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path) //nolint:gosec // a path the user gave
	if err != nil {
		return func() {}
	}
	if pprof.StartCPUProfile(f) != nil {
		_ = f.Close()
		return func() {}
	}
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
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
	rootCmd.PersistentFlags().StringSliceVar(&relogin, "relogin", nil, "sign out and sign in again (switch accounts): --relogin=yandex, --relogin=apple, --relogin=spotify, --relogin=soundcloud, or --relogin for all")
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
			cfg.SoundCloudToken, cfg.SoundCloudRefreshToken, cfg.SoundCloudTokenExpiry = "", "", 0
			names = append(names, "Yandex Music", "Apple Music", "Spotify", "SoundCloud")
		case "soundcloud", "sc":
			cfg.SoundCloudToken, cfg.SoundCloudRefreshToken, cfg.SoundCloudTokenExpiry = "", "", 0
			names = append(names, "SoundCloud")
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
			return nil, fmt.Errorf("--relogin: unknown service %q (use yandex, apple, spotify, soundcloud or all)", svc)
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
	if !noUpdate {
		res := updater.CheckWithPrompt(version.Version, func(s string) {
			if debug {
				fmt.Fprintln(os.Stderr, s)
			}
		}, func(tag string) bool {
			if !term.IsTerminal(os.Stdin.Fd()) {
				return false
			}
			fmt.Fprintf(os.Stderr, i18n.T("Update %s is available. Install it now? [y/N]")+" ", tag)
			answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			answer = strings.ToLower(strings.TrimSpace(answer))
			return answer == "y" || answer == "yes" || answer == "д" || answer == "да"
		})
		if res.Outcome == updater.OutcomeInstalled {
			return restartProcess(res.Exe)
		}
		if res.Outcome == updater.OutcomeManual {
			fmt.Fprintln(os.Stderr, res.Advice())
		}
	}
	opts := tui.Options{MemProfiling: memProfiling, OpenSettings: openSettings}
	firstRun := false
	// First run: the settings open by themselves, with a greeting.
	if !cfg.SetupDone && !demo {
		cfg.SetupDone = true
		if len(cfg.Services) == 0 {
			// Apple Music waits for the user to say whose token it may use.
			cfg.Services = []string{"yandex", "local"}
		}
		if err := cfg.Save(cfgFile); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		opts.OpenSettings, opts.Welcome = "services", true
		firstRun = true
	}

	audioBitrateKbps, err := cfg.AudioBitrateKbps()
	if err != nil {
		return err
	}

	// Apply theme before creating any TUI model so all panels pick up the palette.
	theme, themeErr := styles.LoadTheme(cfg.Theme, filepath.Dir(cfgPath))
	if themeErr != nil && debug {
		fmt.Fprintf(os.Stderr, "debug: theme: %v (falling back to default)\n", themeErr)
	}
	if styles.DetectConsole() {
		// The Linux console (no desktop): its own palette, glyphs and
		// colour handling, see styles.ConsoleText.
		pal, restore := styles.EnterConsole(theme)
		defer restore()
		art.SetConsolePalette(pal)
		theme = styles.ConsoleTheme()
	}
	styles.Apply(theme)
	styles.QueryPalette()
	gfx.CleanStale()
	defer gfx.CleanOwn()

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
		prog := tea.NewProgram(tui.New(cfg, dp, p, opts), programOptions()...)
		restoreStderr := captureStderr()
		_, err = prog.Run()
		restoreStderr()
		return err
	}

	opts.IconPath = assets.InstallIcon()
	assets.InstallDesktopEntry()

	initial := ""
	switch {
	case firstRun:
		// Local files ask for no sign-in: nothing gets in the way of the
		// settings a new user is looking at.
		initial = "local"
	case useApple:
		initial = "apple"
	case local:
		initial = "local"
	}
	return runServices(cfg, opts, audioBitrateKbps, initial)
}

// programOptions are the Bubble Tea options every utuimp program uses. On
// the Linux console the output is held to its 16 colours, so the palette
// indices console mode draws with reach it as they are.
// programOptions is how the interface runs. It also sets the pace the
// screen is drawn at to fit the display's refresh (see tui.InitScreenPace),
// as the program starts.
func programOptions() []tea.ProgramOption {
	tui.InitScreenPace()
	// Bubble Tea writes what was drawn (and the pictures with it) to the
	// terminal on a clock of its own, sixty times a second by default.
	// Against the screen's pace that clock beats: now and then two frames
	// went out together and the next one late, and what moved jerked.
	// At its fastest a frame goes out within a few milliseconds of being
	// drawn; a flush with nothing new writes nothing.
	// The terminal, shared: pictures are written to it the moment their
	// frame is drawn, not on Bubble Tea's clock (see tui.SharedOutput).
	out := tui.NewSharedOutput(os.Stdout)
	tui.SetDirectOutput(out)
	opts := []tea.ProgramOption{tea.WithFPS(120), tea.WithOutput(out)}
	if styles.Console {
		opts = append(opts, tea.WithColorProfile(colorprofile.ANSI))
	}
	return opts
}
