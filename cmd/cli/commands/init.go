package commands

import (
	"os"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/spf13/cobra"
)

func localInitFactory(cm *config.ConfigManager, g git.Git) *cobra.Command {
	return &cobra.Command{
		Use:   "local",
		Short: "Use a local configuration to clones the configured repos",
		Run: func(cmd *cobra.Command, args []string) {
			repos := cm.GetRepos()

			initFn := func(cloneOps git.CloneOps) (struct{}, error) {
				return struct{}{}, g.Clone(cloneOps)
			}

			initArgs := make([]git.CloneOps, 0, len(repos))
			for _, repo := range repos {
				initArgs = append(initArgs, git.CloneOps{
					Remote: repo.GitConfig.Remote,
					Path:   repo.GitConfig.Path,
					Refs:   repo.GitConfig.Refs,
				})
			}

			results := runner.Run(initFn, initArgs)

			for i, res := range results {
				logger := log.NewRepoLogger(os.Stdout, repos[i].Name)
				if res.Error != nil {
					logger.Error(res.Error.Error())
				} else {
					logger.Success("successfully cloned")
				}
			}
		},
	}
}

func initFactory(cm *config.ConfigManager, g git.Git) *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the multi-repo architecture",
	}

	// TODO: introduce a remote init command that gets the location
	// of a remote configuration file
	initCmd.AddCommand(localInitFactory(cm, g))

	return initCmd
}
