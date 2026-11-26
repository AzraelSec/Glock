package config_cmd

import (
	"fmt"

	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/spf13/cobra"
)

func NewCommand(dm *dependency.Manager) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Inspect project configuration"}
	cmd.AddCommand(&cobra.Command{Use: "validate", Short: "Validate glock.yml", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		project, err := dm.LoadProject()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s: valid\n", project.Path)
		return nil
	}})
	return cmd
}
