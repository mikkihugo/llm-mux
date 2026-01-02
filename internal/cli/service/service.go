package service

import "github.com/spf13/cobra"

// ServiceCmd is the parent command for service management
var ServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage background service",
	Long:  `Manage the llm-mux background service (install, start, stop, status).`,
}
