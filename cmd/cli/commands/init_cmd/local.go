package init_cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/spf13/cobra"
)

func localCommand(dm *dependency.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "local",
		Short: "Use a local configuration to clone the configured repos",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := dm.GetGit()
			if err != nil {
				return err
			}

			cm, err := dm.GetConfigManager()
			if err != nil {
				return err
			}

			initFn := func(cloneOps git.CloneOps) (bool, error) {
				return cloneOrResume(g, cloneOps)
			}

			initArgs := make([]git.CloneOps, 0, len(cm.Repos))
			for _, repo := range cm.Repos {
				initArgs = append(initArgs, git.CloneOps{
					Remote: repo.GitConfig.Remote,
					Path:   repo.GitConfig.Path,
					Refs:   repo.GitConfig.Refs,
				})
			}

			results := runner.Run(initFn, initArgs)

			var failures []error
			for i, res := range results {
				logger := log.NewRepoLogger(os.Stdout, cm.Repos[i].Name)
				if res.Error != nil {
					logger.Error("%s", res.Error)
					failures = append(failures, fmt.Errorf("%s: %w", cm.Repos[i].Name, res.Error))
				} else if res.Res {
					logger.Success("already cloned; matching origin")
				} else {
					logger.Success("successfully cloned")
				}
			}

			return errors.Join(failures...)
		},
	}
}
