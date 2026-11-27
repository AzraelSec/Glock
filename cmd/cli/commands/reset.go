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
	skipPull  bool
	skipDirty bool
	gitRepo   git.Repo
}

func resetRepo(g git.Git, payload resetInputPayload) (skipped bool, err error) {
	repo := payload.gitRepo
	if !dir.DirExists(repo.Path) {
		return false, config.ErrRepoNotFound
	}
	if payload.skipDirty {
		dirty, err := g.HasChanges(repo)
		if err != nil {
			return false, fmt.Errorf("check working tree: %w", err)
		}
		if dirty {
			return true, nil
		}
	}

	detachedAtTag, err := g.SwitchToConfiguredRef(repo)
	if err != nil {
		return false, fmt.Errorf("switch to %q: %w", repo.Refs, err)
	}
	if detachedAtTag || payload.skipPull {
		return false, nil
	}

	if err := g.Pull(repo, false); err != nil {
		return false, fmt.Errorf("pull branch %q: %w", repo.Refs, err)
	}
	return false, nil
}

func resetTargets(repos []config.LiveRepo, exclude []string) ([]config.LiveRepo, error) {
	if len(exclude) == 0 {
		return repos, nil
	}
	excluded := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		excluded[name] = false
	}
	targets := make([]config.LiveRepo, 0, len(repos))
	for _, repo := range repos {
		if _, skip := excluded[repo.Name]; skip {
			excluded[repo.Name] = true
			continue
		}
		targets = append(targets, repo)
	}
	for _, name := range exclude {
		if !excluded[name] {
			return nil, fmt.Errorf("unknown Git source %q in --exclude", name)
		}
	}
	return targets, nil
}

func resetFactory(dm *dependency.Manager) *cobra.Command {
	var exclude []string
	var skipPull, skipDirty bool
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

			targets, err := resetTargets(cm.Repos, exclude)
			if err != nil {
				return err
			}
			if len(targets) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No repositories selected for reset.")
				return err
			}
			if !skipDirty && !routine.AllClean(targets, g) {
				return errors.New("some of the repositories are not clean - it's not safe to switch (use --skip-dirty to reset only clean repositories)")
			}

			var failures []error
			for _, repo := range targets {
				skipped, err := resetRepo(g, resetInputPayload{
					skipPull:  skipPull,
					skipDirty: skipDirty,
					gitRepo:   repo.GitConfig,
				})
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", repo.Name, err))
					continue
				}
				if skipped {
					fmt.Fprintf(cmd.OutOrStdout(), "[%s] Skipped (working tree has changes)\n", repo.Name)
					continue
				}
				log.NewRepoLogger(cmd.OutOrStdout(), repo.Name).Success("Came back home 🏠")
			}

			return errors.Join(failures...)
		},
	}

	cmd.Flags().BoolVarP(&skipPull, "skip-pull", "s", false, "Skip pulling changes for branches")
	cmd.Flags().BoolVar(&skipDirty, "skip-dirty", false, "Skip repositories with working tree changes")
	cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "Exclude Git sources by config key (repeatable or comma-separated)")

	return cmd
}
