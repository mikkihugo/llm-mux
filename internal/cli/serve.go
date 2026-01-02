package cli

import (
	"os"

	"github.com/nghyane/llm-mux/internal/cmd"
	"github.com/nghyane/llm-mux/internal/logging"
	log "github.com/nghyane/llm-mux/internal/logging"
	"github.com/spf13/cobra"
)

var servePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the llm-mux server",
	Long: `Start the llm-mux API gateway server.

This is the main command to run the proxy server. It loads the configuration,
initializes the token stores, and starts the HTTP server.`,
	Run: func(c *cobra.Command, args []string) {
		logging.SetupBaseLogger()

		configPath := cfgFile
		if configPath == "" {
			configPath = "$XDG_CONFIG_HOME/llm-mux/config.yaml"
		}

		result, err := Bootstrap(configPath)
		if err != nil {
			log.Fatalf("Failed to bootstrap: %v", err)
			os.Exit(1)
		}

		cfg := result.Config

		if servePort != 0 && servePort != 8317 {
			cfg.Port = servePort
		}

		if err := logging.ConfigureLogOutput(cfg.LoggingToFile); err != nil {
			log.Fatalf("Failed to configure log output: %v", err)
		}

		cmd.StartService(cfg, result.ConfigFilePath, "")
	},
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8317, "server port")
	rootCmd.AddCommand(serveCmd)
}
