package commands

import (
	"errors"
	"os"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/spf13/cobra"
)

type resetInputPayload struct {
	skipPull bool
	gitRepo  git.Repo
}

func resetRepo(g git.Git, payload resetInputPayload) error {
	if !dir.DirExists(payload.gitRepo.Path) {
		return config.ErrRepoNotFound
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

func resetFactory(dm *dependency.DependencyManager) *cobra.Command {
	var skipPull *bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset the branch to its original base branch and pull changes from remote",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := dm.GetGit()
			if err != nil {
				return err
			}
			cm, err := dm.GetConfigManager()
			if err != nil {
				return err
			}

			if !routine.AllClean(cm.Repos, g) {
				return errors.New("some of the repositories are not clean - it's not safe to switch")
			}

			resetArgs := make([]git.Repo, 0, len(cm.Repos))
			resetFn := func(r git.Repo) (struct{}, error) {
				return struct{}{}, resetRepo(g, resetInputPayload{
					skipPull: *skipPull,
					gitRepo:  r,
				})
			}

			for _, repo := range cm.Repos {
				resetArgs = append(resetArgs, repo.GitConfig)
			}

			results := runner.Run(resetFn, resetArgs)

			for i, res := range results {
				logger := log.NewRepoLogger(os.Stdout, cm.Repos[i].Name)
				if res.Error != nil {
					logger.Error(res.Error.Error())
				} else {
					logger.Success("Came back home 🏠")
				}
			}

			return nil
		},
	}

	skipPull = cmd.Flags().BoolP("skip-pull", "s", false, "skip pull and just switch to the base branch")

	return cmd
}
