package login

import (
	"github.com/spf13/cobra"
)

// LoginCmd is the parent command for all provider login subcommands.
var LoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to LLM providers",
	Long:  `Login to various LLM providers using OAuth or other authentication methods.`,
}
