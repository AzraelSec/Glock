package commands

import (
	"errors"
	"fmt"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type switchInputPayload struct {
	gitRepo    git.Repo
	targetRepo string
	force      bool
}

func switchRepo(g git.Git, payload switchInputPayload) error {
	if !dir.DirExists(payload.gitRepo.Path) {
		return config.RepoNotFoundErr
	}

	if err := g.Fetch(payload.gitRepo); err != nil {
		return err
	}

	return g.Switch(
		payload.gitRepo,
		git.BranchName(payload.targetRepo),
		payload.force,
	)
}

func switchFactory(cm *config.ConfigManager, g git.Git) *cobra.Command {
	var force *bool

	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Changes the branch to the given one - for all the repos that has it",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			repos := cm.Repos

			if !*force && !routine.AllClean(repos, g) {
				color.Red("Some of the repositories are not clean - it's not safe to switch")
				return
			}

			switchArgs := make([]switchInputPayload, 0, len(repos))
			switchFn := func(args switchInputPayload) (struct{}, error) {
				return struct{}{}, switchRepo(g, args)
			}

			for _, repo := range repos {
				switchArgs = append(switchArgs, switchInputPayload{
					targetRepo: args[0],
					force:      *force,
					gitRepo:    repo.GitConfig,
				})
			}

			results := runner.Run(switchFn, switchArgs)
			if len(results) != 0 {
				color.Green("Switches:")
			}

			errs := []struct {
				idx int
				err error
			}{}
			for i, rs := range results {
				if rs.Error != nil {
					if errors.Is(rs.Error, git.InvalidReferenceErr) {
						continue
					}

					errs = append(errs, struct {
						idx int
						err error
					}{
						idx: i,
						err: rs.Error,
					})
					continue
				}

				fmt.Printf("\t -> %s\n", repos[i].Name)
			}

			if len(errs) != 0 {
				color.Red("Errors:")
			}
			for _, err := range errs {
				fmt.Printf("\t -> %s {%s}\n", repos[err.idx].Name, err.err.Error())
			}
		},
	}

	force = cmd.Flags().BoolP("force", "f", false, "Force the switch even if some changes exist")

	return cmd
}
