package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// openSettings is the settings section utuimp opens on at start-up ("" for
// none); `utuimp setup` sets it.
var openSettings string //nolint:gochecknoglobals // set by the setup command

// setupSections maps what `utuimp setup <section>` accepts to the settings
// section it opens (the names of the old question-by-question setup still
// work).
var setupSections = map[string]string{ //nolint:gochecknoglobals // a constant table
	"services": "services", "interface": "interface", "language": "interface",
	"visualizer": "visualizer", "discord": "integrations", "lastfm": "integrations",
	"integrations": "integrations", "sound": "sound", "graphics": "graphics", "lyrics": "lyrics", "library": "library",
	"playback": "playback",
}

var setupCmd = &cobra.Command{
	Use:   "setup [section]",
	Short: "Open utuimp on its settings (the same as pressing , inside it)",
	Long: `Opens utuimp with the settings panel shown. Everything is set there:
services, language, theme, animation, playback, visualizer, Discord,
Last.fm, library.

A section name opens the panel on it: services, interface, playback,
visualizer, graphics, integrations, sound, lyrics, library.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		openSettings = "services"
		if len(args) == 1 {
			sec, ok := setupSections[strings.ToLower(args[0])]
			if !ok {
				return fmt.Errorf("unknown settings section %q (one of: services, interface, playback, visualizer, graphics, integrations, sound, lyrics, library)", args[0])
			}
			openSettings = sec
		}
		return runTUI(cmd, nil)
	},
}

func init() { rootCmd.AddCommand(setupCmd) }
