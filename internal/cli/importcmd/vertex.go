package importcmd

import (
	"github.com/nghyane/llm-mux/internal/cmd"
	"github.com/nghyane/llm-mux/internal/config"
	"github.com/spf13/cobra"
)

var vertexCmd = &cobra.Command{
	Use:   "vertex <key-file>",
	Short: "Import Vertex AI service account JSON",
	Args:  cobra.ExactArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		cfgPath, _ := c.Flags().GetString("config")

		cfg, err := config.LoadConfig(cfgPath)
		if err != nil {
			return err
		}

		cmd.DoVertexImport(cfg, args[0])
		return nil
	},
}

func init() {
	ImportCmd.AddCommand(vertexCmd)
}
