package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tphx37/utuimp/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("utuimp %s\n", version.Version)
		fmt.Println("universal terminal user interface media player")
		fmt.Println("based on vibez by simonepelosi · https://github.com/simonepelosi/vibez")
		fmt.Println("https://github.com/tphx37/utuimp")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
