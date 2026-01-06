// Package cli provides the Cobra-based command-line interface for llm-mux.
package cli

import (
	"fmt"
	"os"

	"github.com/nghyane/llm-mux/internal/buildinfo"
	"github.com/nghyane/llm-mux/internal/cli/importcmd"
	"github.com/nghyane/llm-mux/internal/cli/login"
	"github.com/nghyane/llm-mux/internal/cli/service"
	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	noBrowser bool
	debug     bool
)

var rootCmd = &cobra.Command{
	Use:   "llm-mux",
	Short: "AI Gateway for subscription-based LLMs",
	Long:  `llm-mux turns your Claude Pro, GitHub Copilot, and Gemini subscriptions into standard LLM APIs.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: run serve command
		serveCmd.Run(serveCmd, args)
	},
}

func Execute() {
	rootCmd.Version = buildinfo.Version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().BoolVar(&noBrowser, "no-browser", false, "don't open browser for OAuth")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug output")

	rootCmd.Version = buildinfo.Version

	rootCmd.AddCommand(login.LoginCmd)
	rootCmd.AddCommand(service.ServiceCmd)
	rootCmd.AddCommand(importcmd.ImportCmd)
}

func GetConfigPath() string { return cfgFile }

func GetNoBrowser() bool { return noBrowser }

func GetDebug() bool { return debug }
