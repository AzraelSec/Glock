package commands

import (
	"fmt"
	"sort"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/AzraelSec/glock/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type switchOutputPayload struct {
	RepoName string
}
type switchInputPayload struct {
	targetRepo string
	force      bool
}

func switchRepo(info runner.RunnerInfo[switchOutputPayload, switchInputPayload]) {
	var err error
	res := switchOutputPayload{
		RepoName: info.RepoData.Name,
	}
	defer func() {
		info.Result <- runner.NewRunnerResult(err, res)
	}()

	if !utils.DirExists(info.RepoData.GitConfig.Path) {
		err = config.RepoNotFoundErr
		return
	}

	err = info.Git.Fetch(info.RepoData.GitConfig)
	if err != nil {
		return
	}

	err = info.Git.Switch(info.RepoData.GitConfig, git.BranchName(info.Args.targetRepo), info.Args.force)
}

func switchFactory(ops commandOps) *cobra.Command {
	var force *bool

	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Changes the branch to the given one - for all the repos that has it",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			repos := ops.ConfigManager.GetRepos()

			if *force == false && !routine.AllClean(repos, ops.git) {
				color.Red("Some of the repositories are not clean - it's not safe to switch")
				return
			}

			wc := make(chan runner.RunnerResult[switchOutputPayload], 0)
			wp := runner.WrapRunnerFunc(switchRepo, runner.RunnerFuncWrapperInfo[switchOutputPayload]{
				Git:    ops.git,
				Result: wc,
			})

			for _, repo := range repos {
				go wp(repo, switchInputPayload{targetRepo: args[0]})
			}

			res := []string{}
			for i := 0; i < len(repos); i++ {
				info := <-wc
				if info.Error != nil {
					continue
				}
				res = append(res, info.Result.RepoName)
			}
			sort.Strings(res)
			if len(res) != 0 {
				color.Green("Switches:")
			}
			for _, rs := range res {
				fmt.Printf("\t -> %s\n", rs)
			}
		},
	}

	force = cmd.Flags().BoolP("force", "f", false, "Force the switch even if some changes exist")

	return cmd
}
