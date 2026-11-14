package config_cmd

import (
	"fmt"
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/spf13/cobra"
)

func NewCommand(dm *dependency.Manager) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Inspect and migrate configuration"}
	cmd.AddCommand(&cobra.Command{Use: "validate", Short: "Validate glock.yml against the v2 schema", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		project, err := dm.LoadProject()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s: valid\n", project.Path)
		return nil
	}})
	var output, v1Root string
	migrate := &cobra.Command{Use: "migrate", Short: "Convert v1 configuration to v2 without overwriting files", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		input, err := dm.GetConfigFilePath()
		if err != nil {
			return err
		}
		if err = config.WriteMigration(input, output, v1Root); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s; review lifecycle commands before starting\n", output)
		return nil
	}}
	migrate.Flags().StringVar(&v1Root, "v1-root", "", "absolute effective project root for a relative v1 root_path")
	migrate.Flags().StringVar(&output, "output", "", "destination glock.yml in the old effective project root")
	_ = migrate.MarkFlagRequired("output")
	cmd.AddCommand(migrate)
	return cmd
}
