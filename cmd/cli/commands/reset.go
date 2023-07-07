package commands

import (
	"os"
	"sort"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type resetOutputPayload struct {
	RepoName string
}
type resetInputPayload struct {
	SkipPull bool
}

func resetRepo(info runner.RunnerInfo[resetOutputPayload, resetInputPayload]) {
	var err error
	res := resetOutputPayload{
		RepoName: info.RepoData.Name,
	}
	defer func() {
		info.Result <- runner.NewRunnerResult(err, res)
	}()

	if !dir.DirExists(info.RepoData.GitConfig.Path) {
		err = config.RepoNotFoundErr
		return
	}

	branch, err := info.Git.CurrentBranch(info.RepoData.GitConfig)
	if err != nil {
		return
	}
	if branch == git.BranchName(info.RepoData.Config.Remote) {
		return
	}

	err = info.Git.Switch(info.RepoData.GitConfig, info.RepoData.GitConfig.Refs, false)
	if err != nil {
		return
	}

	if info.Args.SkipPull {
		return
	}
	err = info.Git.Pull(info.RepoData.GitConfig, false)
	if err != nil {
		return
	}
}

func resetFactory(cm *config.ConfigManager, g git.Git) *cobra.Command {
	var skipPull *bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset the branch to its original base branch and pull changes from remote",
		Run: func(cmd *cobra.Command, args []string) {
			repos := cm.GetRepos()

			if !routine.AllClean(repos, g) {
				color.Red("Some of the repositories are not clean - it's not safe to switch")
				return
			}

			wc := make(chan runner.RunnerResult[resetOutputPayload])
			wp := runner.WrapRunnerFunc(resetRepo, runner.RunnerFuncWrapperInfo[resetOutputPayload]{
				Git:    g,
				Result: wc,
			})

			for _, repo := range repos {
				go wp(repo, resetInputPayload{
					SkipPull: *skipPull,
				})
			}

			res := []runner.RunnerResult[resetOutputPayload]{}
			for i := 0; i < len(repos); i++ {
				res = append(res, <-wc)
			}
			sort.Slice(res, func(i, j int) bool {
				return res[i].Result.RepoName < res[j].Result.RepoName
			})

			for _, rs := range res {
				logger := log.NewRepoLogger(os.Stdout, rs.Result.RepoName)
				if rs.Error != nil {
					logger.Error(rs.Error.Error())
				} else {
					logger.Success("🏠")
				}
			}
		},
	}

	skipPull = cmd.Flags().BoolP("skip-pull", "s", false, "skip pull and just switch to the base branch")

	return cmd
}
