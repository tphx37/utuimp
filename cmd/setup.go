package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
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
	Use:   "setup",
	Short: "Choose the language, Discord status and services (run again to change them)",
	RunE: func(*cobra.Command, []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		return runSetup(cfg, os.Stdin, os.Stdout)
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
	{"soundcloud", "SoundCloud (your own API app)", false},
	{"local", "Local files", true},
}

// runSetup asks a few questions in the terminal and saves the answers.
func runSetup(cfg *config.Config, in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	ask := func(prompt, def string) string {
		fmt.Fprintf(out, "%s ", prompt)
		line, _ := r.ReadString('\n')
		if line = strings.TrimSpace(line); line == "" {
			return def
		}
		return line
	}
	yesNo := func(prompt string, def bool) bool {
		hint := i18n.T("[Y/n]")
		if !def {
			hint = i18n.T("[y/N]")
		}
		a := strings.ToLower(ask(prompt+" "+hint, ""))
		switch {
		case a == "":
			return def
		case strings.HasPrefix(a, "y"), strings.HasPrefix(a, "д"):
			return true
		default:
			return false
		}
	}

	// 1. Language
	cur := cfg.Language
	if cur == "" {
		cur = i18n.Detect()
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "utuimp setup")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "1/3  Language / Язык")
	defIdx := 1
	for i, l := range i18n.Languages {
		fmt.Fprintf(out, "     %d) %s\n", i+1, l[1])
		if l[0] == cur {
			defIdx = i + 1
		}
	}
	choice := ask(fmt.Sprintf("     [%d]:", defIdx), fmt.Sprint(defIdx))
	lang := i18n.Languages[defIdx-1][0]
	for i, l := range i18n.Languages {
		if choice == fmt.Sprint(i+1) || strings.EqualFold(choice, l[0]) {
			lang = l[0]
		}
	}
	cfg.Language = lang
	i18n.Set(lang)

	// 2. Discord
	fmt.Fprintln(out)
	fmt.Fprintln(out, i18n.T("2/3  Discord"))
	on := yesNo("     "+i18n.T("Show what you're playing in your Discord status?"), cfg.DiscordRPCEnabled())
	cfg.DiscordRPC = &on

	// 3. Services
	fmt.Fprintln(out)
	fmt.Fprintln(out, i18n.T("3/3  Services (each becomes a tab, keys 1, 2, …)"))
	var services []string
	for _, s := range setupServices {
		def := s.defOn
		if len(cfg.Services) > 0 {
			def = slices.Contains(cfg.Services, s.key)
		}
		if yesNo("     "+i18n.T(s.name)+"?", def) {
			services = append(services, s.key)
		}
		if s.key == "spotify" && slices.Contains(services, "spotify") {
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
		if s.key == "youtube" && slices.Contains(services, "youtube") {
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
		if s.key == "soundcloud" && slices.Contains(services, "soundcloud") {
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
		if s.key == "local" && slices.Contains(services, "local") {
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
	cfg.SetupDone = true

	if err := cfg.Save(cfgFile); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	path, _ := config.ConfigPath(cfgFile)
	fmt.Fprintln(out)
	fmt.Fprintf(out, i18n.T("Saved to %s. Run utuimp setup again to change it.")+"\n", path)
	return nil
}
