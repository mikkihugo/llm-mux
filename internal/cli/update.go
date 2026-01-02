package cli

import (
	"os"

	log "github.com/nghyane/llm-mux/internal/logging"
	"github.com/spf13/cobra"
)

var checkOnly bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for updates and install if available",
	Long: `Check for updates and install the latest version.

By default, this will download and install the latest release.
Use --check to only check for updates without installing.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := DoUpdate(checkOnly); err != nil {
			log.Fatalf("Update failed: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	updateCmd.Flags().BoolVar(&checkOnly, "check", false, "check for updates without installing")
	rootCmd.AddCommand(updateCmd)
}
