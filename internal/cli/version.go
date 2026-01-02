package cli

import (
	"fmt"

	"github.com/nghyane/llm-mux/internal/buildinfo"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("llm-mux %s\n", buildinfo.Version)
		fmt.Printf("Commit: %s\n", buildinfo.Commit)
		fmt.Printf("Built: %s\n", buildinfo.BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
