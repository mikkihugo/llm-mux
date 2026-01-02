package cli

import (
	"os"

	log "github.com/nghyane/llm-mux/internal/logging"
	"github.com/spf13/cobra"
)

var forceInit bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize config and generate management key",
	Long: `Initialize llm-mux configuration and generate a management key.

On first run, this creates the config file and auth directory.
If config already exists, it shows the current management key.

Use --force to regenerate the management key.`,
	Run: func(cmd *cobra.Command, args []string) {
		configPath := cfgFile
		if configPath == "" {
			configPath = "$XDG_CONFIG_HOME/llm-mux/config.yaml"
		}
		if err := DoInitConfig(configPath, forceInit); err != nil {
			log.Fatalf("Init failed: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	initCmd.Flags().BoolVar(&forceInit, "force", false, "force regenerate management key")
	rootCmd.AddCommand(initCmd)
}
