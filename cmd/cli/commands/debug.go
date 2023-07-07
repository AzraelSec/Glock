package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func debugFactory(ops commandOps) *cobra.Command {
	return &cobra.Command{
		Use: "debug",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("== DEBUG ==")
			fmt.Println(ops.ConfigManager)
			fmt.Println(args)
			fmt.Println("== DEBUG ==")
		},
	}
}
