package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/player/spotifyconnect"
	"github.com/tphx37/utuimp/internal/provider/soundcloud"
	"github.com/tphx37/utuimp/internal/provider/spotify"
)

var setupCmd = &cobra.Command{
	Use:   "setup [section…]",
	Short: "Choose the language, Discord status, services, visualizer and animation (run again to change them)",
	Long: "Without arguments the first run asks everything; later runs offer a menu of sections.\n" +
		"Sections: language, discord, services, visualizer, interface.",
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		return runSetup(cfg, os.Stdin, os.Stdout, args...)
	},
}

func init() { rootCmd.AddCommand(setupCmd) }

// isInteractive reports whether stdin and stdout are a terminal.
func isInteractive() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

// setupServices are the services offered in setup, in tab order (local last).
var setupServices = []struct {
	key, name string
	defOn     bool
}{
	{"yandex", "Yandex Music", true},
	{"apple", "Apple Music", true},
	{"spotify", "Spotify (Premium)", false},
	{"youtube", "YouTube Music", false},
	{"soundcloud", "SoundCloud", false},
	{"local", "Local files", true},
}

// setupIO asks questions in the terminal.
type setupIO struct {
	r   *bufio.Reader
	out io.Writer
	eof bool
}

func (s *setupIO) say(a ...any) { fmt.Fprintln(s.out, a...) }

// ask prints prompt and reads a line; an empty one (or the end of the
// input) is def.
func (s *setupIO) ask(prompt, def string) string {
	fmt.Fprintf(s.out, "%s ", prompt)
	line, err := s.r.ReadString('\n')
	if err != nil {
		s.eof = true
	}
	if line = strings.TrimSpace(line); line == "" {
		return def
	}
	return line
}

func (s *setupIO) yesNo(prompt string, def bool) bool {
	hint := i18n.T("[Y/n]")
	if !def {
		hint = i18n.T("[y/N]")
	}
	a := strings.ToLower(s.ask(prompt+" "+hint, ""))
	switch {
	case a == "":
		return def
	case strings.HasPrefix(a, "y"), strings.HasPrefix(a, "д"):
		return true
	default:
		return false
	}
}

// choose offers numbered options (value, label) and returns the value
// picked by number or by name; Enter keeps cur.
func (s *setupIO) choose(opts [][2]string, cur string) string { return s.chooseOr(opts, cur, nil) }

// chooseOr is choose that also takes an answer free accepts (and turns
// into a value) when it is none of the options.
func (s *setupIO) chooseOr(opts [][2]string, cur string, free func(string) (string, bool)) string {
	def := 1
	for i, o := range opts {
		if o[0] == cur {
			def = i + 1
		}
	}
	for i, o := range opts {
		mark := " "
		if i+1 == def {
			mark = "•"
		}
		fmt.Fprintf(s.out, "   %s %d) %s\n", mark, i+1, o[1])
	}
	a := s.ask(fmt.Sprintf("     [%d]:", def), "")
	for i, o := range opts {
		if a == strconv.Itoa(i+1) || a != "" && strings.EqualFold(a, o[0]) { // an empty answer keeps the marked one, even next to an option whose value is ""
			return o[0]
		}
	}
	if free != nil {
		if v, ok := free(a); ok {
			return v
		}
	}
	return opts[def-1][0]
}

// setupSection is one part of the setup, asked on its own from the menu.
type setupSection struct {
	key   string
	title string // English; translated when shown
	// now says what is set, for the menu.
	now func(*config.Config) string
	run func(*setupIO, *config.Config)
}

var setupSections = []setupSection{
	{"language", "Language / Язык", setupLanguageNow, setupLanguage},
	{"discord", "Discord", setupDiscordNow, setupDiscord},
	{"services", "Services (each becomes a tab, keys 1, 2, …)", setupServicesNow, setupServicesRun},
	{"visualizer", "Visualizer (strength and background)", setupVizNow, setupViz},
	{"interface", "Interface (animation and start screen)", setupUINow, setupUI},
	{"library", "Library (deleting and editing)", setupLibraryNow, setupLibrary},
}

// runSetup asks the setup questions in the terminal and saves the answers.
// The first run goes through every section; later ones show a menu, so one
// thing can be changed without the rest. Sections named in only run just
// those.
func runSetup(cfg *config.Config, in io.Reader, out io.Writer, only ...string) error {
	s := &setupIO{r: bufio.NewReader(in), out: out}
	if cfg.Language != "" {
		i18n.Set(cfg.Language)
	}
	var todo []int
	for _, name := range only {
		i := slices.IndexFunc(setupSections, func(sec setupSection) bool {
			return strings.EqualFold(sec.key, name) || strings.EqualFold(i18n.T(sec.key), name)
		})
		if i < 0 {
			keys := make([]string, len(setupSections))
			for k, sec := range setupSections {
				keys[k] = sec.key
			}
			return fmt.Errorf("unknown setup section %q (one of: %s)", name, strings.Join(keys, ", "))
		}
		todo = append(todo, i)
	}
	s.say()
	s.say("utuimp setup")
	switch {
	case len(todo) > 0:
		for _, i := range todo {
			runSection(s, cfg, i, "")
		}
	case !cfg.SetupDone:
		for i := range setupSections {
			runSection(s, cfg, i, fmt.Sprintf("%d/%d  ", i+1, len(setupSections)))
		}
	default:
		setupMenu(s, cfg)
	}
	cfg.SetupDone = true
	if err := cfg.Save(cfgFile); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	path, _ := config.ConfigPath(cfgFile)
	s.say()
	fmt.Fprintf(out, i18n.T("Saved to %s. Run utuimp setup again to change it.")+"\n", path)
	return nil
}

func runSection(s *setupIO, cfg *config.Config, i int, prefix string) {
	sec := setupSections[i]
	s.say()
	s.say(prefix + i18n.T(sec.title))
	sec.run(s, cfg)
}

// setupMenu lists the sections with what each is set to, and runs the
// ones picked until Enter.
func setupMenu(s *setupIO, cfg *config.Config) {
	for !s.eof {
		s.say()
		s.say(i18n.T("What to change?"))
		for i, sec := range setupSections {
			fmt.Fprintf(s.out, "     %d) %-*s %s\n", i+1, 22, i18n.T(setupShort(sec)), setupDim(sec.now(cfg)))
		}
		fmt.Fprintf(s.out, "     a) %s\n", i18n.T("everything, one after another"))
		a := strings.ToLower(s.ask("     "+i18n.T("Number (Enter = save and quit):"), ""))
		switch {
		case a == "" || a == "q" || a == "й":
			return
		case a == "a" || a == "ф" || a == "all" || a == "все" || a == "всё":
			for i := range setupSections {
				runSection(s, cfg, i, fmt.Sprintf("%d/%d  ", i+1, len(setupSections)))
			}
			return
		}
		// "1 4" or "14" runs several.
		ran := false
		for _, r := range strings.ReplaceAll(strings.ReplaceAll(a, ",", ""), " ", "") {
			if i := int(r - '1'); i >= 0 && i < len(setupSections) {
				runSection(s, cfg, i, "")
				ran = true
			}
		}
		if !ran {
			s.say("     " + i18n.T("No such section."))
		}
	}
}

// setupShort is a section's name for the menu.
func setupShort(sec setupSection) string {
	switch sec.key {
	case "language":
		return "Language / Язык"
	case "services":
		return "Services"
	case "visualizer":
		return "Visualizer"
	case "interface":
		return "Interface"
	}
	return sec.title
}

func setupDim(s string) string {
	if !isInteractive() {
		return s
	}
	return "\x1b[2m" + s + "\x1b[0m"
}

// ── Language ────────────────────────────────────────────────────────────────

func setupLanguageNow(cfg *config.Config) string {
	cur := cfg.Language
	if cur == "" {
		cur = i18n.Detect()
	}
	for _, l := range i18n.Languages {
		if l[0] == cur {
			return l[1]
		}
	}
	return cur
}

func setupLanguage(s *setupIO, cfg *config.Config) {
	cur := cfg.Language
	if cur == "" {
		cur = i18n.Detect()
	}
	opts := make([][2]string, len(i18n.Languages))
	for i, l := range i18n.Languages {
		opts[i] = [2]string{l[0], l[1]}
	}
	cfg.Language = s.choose(opts, cur)
	i18n.Set(cfg.Language)
}

// ── Discord ─────────────────────────────────────────────────────────────────

func setupDiscordNow(cfg *config.Config) string {
	if cfg.DiscordRPCEnabled() {
		return i18n.T("status on")
	}
	return i18n.T("status off")
}

func setupDiscord(s *setupIO, cfg *config.Config) {
	on := s.yesNo("     "+i18n.T("Show what you're playing in your Discord status?"), cfg.DiscordRPCEnabled())
	cfg.DiscordRPC = &on
}

// ── Visualizer ──────────────────────────────────────────────────────────────

// vizStrengths are the strengths offered, in percent.
var vizStrengths = [][2]string{
	{"25", "calm: 25% (least motion)"},
	{"50", "soft: 50%"},
	{"75", "gentle: 75%"},
	{"100", "full: 100%"},
	{"150", "wild: 150%"},
}

var vizBackgrounds = [][2]string{
	{"", "none"},
	{"stars", "stars: tiny squares standing still (a steady backdrop for the eye)"},
	{"sparks", "sparks: tiny squares drifting up, as behind the ribbons"},
}

func setupVizNow(cfg *config.Config) string {
	bg := i18n.T("no background")
	for _, b := range vizBackgrounds[1:] {
		if b[0] == cfg.VisualizerBackground {
			bg = i18n.T(b[0])
		}
	}
	return fmt.Sprintf("%d%% · %s", int(cfg.VizStrength()*100+0.5), bg)
}

func setupViz(s *setupIO, cfg *config.Config) {
	s.say("     " + i18n.T("Strength: how hard the picture moves, zooms and shakes. Less of it helps if the visualizer makes you dizzy."))
	opts := make([][2]string, len(vizStrengths))
	for i, o := range vizStrengths {
		opts[i] = [2]string{o[0], i18n.T(o[1])}
	}
	cur := strconv.Itoa(int(cfg.VizStrength()*100 + 0.5))
	if !slices.ContainsFunc(opts, func(o [2]string) bool { return o[0] == cur }) {
		opts = append(opts, [2]string{cur, cur + "%"})
	}
	fmt.Fprintln(s.out, "     "+i18n.T("(or type a number of percent, 0–200)"))
	v := s.chooseOr(opts, cur, func(a string) (string, bool) {
		n, err := strconv.Atoi(strings.TrimSuffix(a, "%"))
		return strconv.Itoa(n), err == nil && n > len(opts)
	})
	if n, err := strconv.Atoi(strings.TrimSuffix(v, "%")); err == nil {
		n = min(200, max(0, n))
		if n == 100 {
			cfg.VisualizerStrength = nil
		} else {
			cfg.VisualizerStrength = &n
		}
	}
	s.say()
	s.say("     " + i18n.T("Background behind every scene:"))
	bgs := make([][2]string, len(vizBackgrounds))
	for i, o := range vizBackgrounds {
		bgs[i] = [2]string{o[0], i18n.T(o[1])}
	}
	cfg.VisualizerBackground = s.choose(bgs, cfg.VisualizerBackground)
}

// ── Interface ───────────────────────────────────────────────────────────────

var uiMotions = [][2]string{
	{config.MotionOff, "off: still"},
	{config.MotionCalm, "calm: panels and new songs fade in"},
	{config.MotionLively, "lively: the same (nothing moves any more)"},
	{config.MotionWild, "wild: the same"},
}

func setupUINow(cfg *config.Config) string {
	intro := i18n.T("start screen on")
	if !cfg.IntroEnabled() {
		intro = i18n.T("start screen off")
	}
	return i18n.T("animation") + ": " + i18n.T(cfg.Motion()) + " · " + intro
}

func setupUI(s *setupIO, cfg *config.Config) {
	s.say("     " + i18n.T("How lively the interface is: it reacts to moving through lists, to song changes and to the music."))
	opts := make([][2]string, len(uiMotions))
	for i, o := range uiMotions {
		opts[i] = [2]string{o[0], i18n.T(o[1])}
	}
	m := s.choose(opts, cfg.Motion())
	cfg.UIMotion = m
	if m == config.MotionLively {
		cfg.UIMotion = ""
	}
	on := s.yesNo("     "+i18n.T("Start screen (the logo over dancing ribbons)?"), cfg.IntroEnabled())
	if on {
		cfg.Intro = nil
	} else {
		cfg.Intro = &on
	}
}

// ── Services ────────────────────────────────────────────────────────────────

func setupServicesNow(cfg *config.Config) string {
	if len(cfg.Services) == 0 {
		return i18n.T("all")
	}
	return strings.Join(cfg.Services, ", ")
}

func setupServicesRun(s *setupIO, cfg *config.Config) {
	ask, yesNo, out := s.ask, s.yesNo, s.out
	var services []string
	for _, sv := range setupServices {
		def := sv.defOn
		if len(cfg.Services) > 0 {
			def = slices.Contains(cfg.Services, sv.key)
		}
		if yesNo("     "+i18n.T(sv.name)+"?", def) {
			services = append(services, sv.key)
		}
		if sv.key == "spotify" && slices.Contains(services, "spotify") {
			if _, err := spotifyconnect.FindLibrespot(cfg.LibrespotPath); err != nil {
				fmt.Fprintln(out, "     "+i18n.T("Spotify plays through librespot, which isn't installed yet:"))
				fmt.Fprintln(out, "       "+librespotInstallHint())
			}
			fmt.Fprintln(out, "     "+i18n.T("On first use, approve access on the Spotify page that opens."))
			fmt.Fprintln(out, "     "+i18n.T("Spotify sometimes blocks the shared sign-in. To avoid that, use your own free app (optional):"))
			fmt.Fprintln(out, "       1. "+i18n.T("Open https://developer.spotify.com/dashboard and create an app."))
			fmt.Fprintln(out, "       2. "+i18n.T("Redirect URI:")+" "+spotify.RedirectURI+"   "+i18n.T("API: Web API."))
			fmt.Fprintln(out, "       3. "+i18n.T("Paste its Client ID here."))
			def := cfg.SpotifyClientID
			prompt := "     Client ID (" + i18n.T("Enter = shared sign-in") + "):"
			if def != "" {
				prompt = "     Client ID [" + def + "] (" + i18n.T("\"-\" = shared sign-in") + "):"
			}
			id := strings.TrimSpace(ask(prompt, def))
			if id == "-" {
				id = ""
			}
			if id != cfg.SpotifyClientID {
				// Another app: the old sign-in doesn't apply.
				cfg.SpotifyToken, cfg.SpotifyRefreshToken, cfg.SpotifyTokenExpiry = "", "", 0
				cfg.SpotifyClientID = id
			}
		}
		if sv.key == "youtube" && slices.Contains(services, "youtube") {
			if _, err := findTool(cfg.YtDlpPath, "yt-dlp"); err != nil {
				fmt.Fprintln(out, "     "+i18n.T("YouTube Music plays through yt-dlp and ffmpeg:"))
				fmt.Fprintln(out, "       "+ytInstallHint())
			} else if _, err := findTool(cfg.FFmpegPath, "ffmpeg"); err != nil {
				fmt.Fprintln(out, "     "+i18n.T("YouTube Music plays through yt-dlp and ffmpeg:"))
				fmt.Fprintln(out, "       "+ytInstallHint())
			}
			fmt.Fprintln(out, "     "+i18n.T("If YouTube asks to confirm you're not a bot, yt-dlp needs your browser's YouTube cookies."))
			def := cfg.YouTubeCookies
			prompt := "     " + i18n.T("Browser to take cookies from (firefox, chrome…) or a cookies.txt path") + " (" + i18n.T("Enter = none") + "):"
			if def != "" {
				prompt = "     " + i18n.T("Browser to take cookies from (firefox, chrome…) or a cookies.txt path") + " [" + def + "] (" + i18n.T("\"-\" = none") + "):"
			}
			v := strings.TrimSpace(ask(prompt, def))
			if v == "-" {
				v = ""
			}
			cfg.YouTubeCookies = v
		}
		if sv.key == "soundcloud" && slices.Contains(services, "soundcloud") {
			fmt.Fprintln(out, "     "+i18n.T("SoundCloud plays through yt-dlp and ffmpeg, no account needed (favorites stay in utuimp)."))
			if _, err := findTool(cfg.YtDlpPath, "yt-dlp"); err != nil {
				fmt.Fprintln(out, "       "+ytInstallHint())
			}
			hasApp := cfg.SoundCloudClientID != "" && cfg.SoundCloudClientSecret != ""
			if !yesNo("     "+i18n.T("Use your own SoundCloud API app instead (your likes and playlists; needs Artist Pro)?"), hasApp) {
				if hasApp {
					cfg.SoundCloudClientID, cfg.SoundCloudClientSecret = "", ""
					cfg.SoundCloudToken, cfg.SoundCloudRefreshToken, cfg.SoundCloudTokenExpiry = "", "", 0
				}
				continue
			}
			fmt.Fprintln(out, "     "+i18n.T("SoundCloud works through its official API with your own app (registering one needs SoundCloud Artist Pro):"))
			fmt.Fprintln(out, "       1. "+i18n.T("Open https://soundcloud.com/you/apps and create an app."))
			fmt.Fprintln(out, "       2. "+i18n.T("Redirect URI:")+" "+soundcloud.RedirectURI)
			fmt.Fprintln(out, "       3. "+i18n.T("Paste its Client ID and Client Secret here."))
			id := strings.TrimSpace(ask("     Client ID ["+cfg.SoundCloudClientID+"]:", cfg.SoundCloudClientID))
			secretHint := ""
			if cfg.SoundCloudClientSecret != "" {
				secretHint = " [" + i18n.T("saved") + "]"
			}
			secret := strings.TrimSpace(ask("     Client Secret"+secretHint+":", cfg.SoundCloudClientSecret))
			if id != cfg.SoundCloudClientID || secret != cfg.SoundCloudClientSecret {
				// Another app: the old sign-in doesn't apply.
				cfg.SoundCloudToken, cfg.SoundCloudRefreshToken, cfg.SoundCloudTokenExpiry = "", "", 0
				cfg.SoundCloudClientID, cfg.SoundCloudClientSecret = id, secret
			}
		}
		if sv.key == "local" && slices.Contains(services, "local") {
			dir := ask("     "+i18n.T("Music folder")+" ["+localMusicDir(cfg)+"]:", "")
			if dir != "" {
				cfg.MusicDir = dir
			}
		}
	}
	if len(services) == 0 {
		fmt.Fprintln(out, "     "+i18n.T("No service chosen; keeping local files."))
		services = []string{"local"}
	}
	cfg.Services = services
}

// ── Library ─────────────────────────────────────────────────────────────────

func setupLibraryNow(cfg *config.Config) string {
	on := func(b bool) string {
		if b {
			return i18n.T("on")
		}
		return i18n.T("off")
	}
	return i18n.T("deleting") + ": " + on(cfg.LibraryDelete) + " · " + i18n.T("editing") + ": " + on(cfg.LibraryEdit)
}

func setupLibrary(s *setupIO, cfg *config.Config) {
	s.say("     " + i18n.T("Adding songs, albums and artists to the library and to playlists always works."))
	cfg.LibraryDelete = s.yesNo("     "+i18n.T("Allow deleting from the library (playlists, songs from playlists, likes of songs, albums and artists; d or f twice)?"), cfg.LibraryDelete)
	cfg.LibraryEdit = s.yesNo("     "+i18n.T("Allow editing the library (renaming playlists; r)?"), cfg.LibraryEdit)
}
