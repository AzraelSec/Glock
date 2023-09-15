package commands

import (
	"os"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type resetInputPayload struct {
	skipPull bool
	gitRepo  git.Repo
}

func resetRepo(g git.Git, payload resetInputPayload) error {
	if !dir.DirExists(payload.gitRepo.Path) {
		return config.RepoNotFoundErr
	}

	branch, err := g.CurrentBranch(payload.gitRepo)
	if err != nil {
		return err
	}

	if branch != git.BranchName(payload.gitRepo.Refs) {
		if err := g.Switch(payload.gitRepo, payload.gitRepo.Refs, false); err != nil {
			return err
		}
	}

	if payload.skipPull {
		return nil
	}

	return g.Pull(payload.gitRepo, false)
}

func resetFactory(cm *config.ConfigManager, g git.Git) *cobra.Command {
	var skipPull *bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset the branch to its original base branch and pull changes from remote",
		Run: func(cmd *cobra.Command, args []string) {
			repos := cm.Repos

			if !routine.AllClean(repos, g) {
				color.Red("Some of the repositories are not clean - it's not safe to switch")
				return
			}

			resetArgs := make([]git.Repo, 0, len(repos))
			resetFn := func(r git.Repo) (struct{}, error) {
				return struct{}{}, resetRepo(g, resetInputPayload{
					skipPull: *skipPull,
					gitRepo:  r,
				})
			}

			for _, repo := range repos {
				resetArgs = append(resetArgs, repo.GitConfig)
			}

			results := runner.Run(resetFn, resetArgs)

			for i, res := range results {
				logger := log.NewRepoLogger(os.Stdout, repos[i].Name)
				if res.Error != nil {
					logger.Error(res.Error.Error())
				} else {
					logger.Success("Came back home 🏠")
				}
			}
		},
	}

	skipPull = cmd.Flags().BoolP("skip-pull", "s", false, "skip pull and just switch to the base branch")

	return cmd
}
