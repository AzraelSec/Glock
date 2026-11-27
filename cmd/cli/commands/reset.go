package commands

import (
	"errors"
	"fmt"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/spf13/cobra"
)

type resetInputPayload struct {
	skipPull bool
	gitRepo  git.Repo
}

func resetRepo(g git.Git, payload resetInputPayload) error {
	repo := payload.gitRepo
	if !dir.DirExists(repo.Path) {
		return config.ErrRepoNotFound
	}

	detachedAtTag, err := g.SwitchToConfiguredRef(repo)
	if err != nil {
		return fmt.Errorf("switch to %q: %w", repo.Refs, err)
	}
	if detachedAtTag || payload.skipPull {
		return nil
	}

	if err := g.Pull(repo, false); err != nil {
		return fmt.Errorf("pull branch %q: %w", repo.Refs, err)
	}
	return nil
}

func resetFactory(dm *dependency.Manager) *cobra.Command {
	var skipPull *bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Switch to each configured branch or tag and pull branch changes",
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

			var failures []error
			for _, repo := range cm.Repos {
				err := resetRepo(g, resetInputPayload{
					skipPull: *skipPull,
					gitRepo:  repo.GitConfig,
				})
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", repo.Name, err))
					continue
				}
				log.NewRepoLogger(cmd.OutOrStdout(), repo.Name).Success("Came back home 🏠")
			}

			return errors.Join(failures...)
		},
	}

	skipPull = cmd.Flags().BoolP("skip-pull", "s", false, "skip pulling changes for branches")

	return cmd
}
