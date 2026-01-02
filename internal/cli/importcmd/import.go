package importcmd

import "github.com/spf13/cobra"

// ImportCmd is the parent command for import operations
var ImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import credentials from external files",
	Long:  `Import credentials from external files (e.g. Vertex AI service accounts).`,
}
