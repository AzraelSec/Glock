package init_cmd

import (
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/spf13/cobra"
)

type initCmd struct {
	dm *dependency.Manager
}

func New(dm *dependency.Manager) *initCmd {
	return &initCmd{dm}
}

func (i *initCmd) Command() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the multi-repo architecture",
	}

	initCmd.AddCommand(localCommand(i.dm))
	initCmd.AddCommand(remoteCommand(i.dm))

	return initCmd
}
