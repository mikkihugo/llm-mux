package login

import (
	"github.com/nghyane/llm-mux/internal/cmd"
	"github.com/nghyane/llm-mux/internal/config"
	"github.com/spf13/cobra"
)

var antigravityCmd = &cobra.Command{
	Use:     "antigravity",
	Aliases: []string{"ag"},
	Short:   "Login to Antigravity (Google Gemini)",
	Long: `Login to Antigravity provider using OAuth authentication.

This command initiates the OAuth flow for Google Gemini through the Antigravity
provider. A browser window will open for you to authenticate with your Google
account. Use --no-browser to get a manual authentication URL instead.`,
	RunE: func(c *cobra.Command, args []string) error {
		cfgPath, _ := c.Flags().GetString("config")
		noBrowser, _ := c.Flags().GetBool("no-browser")

		cfg, err := config.LoadConfig(cfgPath)
		if err != nil {
			return err
		}

		options := &cmd.LoginOptions{
			NoBrowser: noBrowser,
		}

		cmd.DoAntigravityLogin(cfg, options)
		return nil
	},
}

func init() {
	LoginCmd.AddCommand(antigravityCmd)
}
