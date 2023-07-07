package commands

import (
	"fmt"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/spf13/cobra"
)

func debugFactory(cm *config.ConfigManager) *cobra.Command {
	return &cobra.Command{
		Use: "debug",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("== DEBUG ==")
			fmt.Println(cm.Config)
			fmt.Println(args)
			fmt.Println("== DEBUG ==")
		},
	}
}
