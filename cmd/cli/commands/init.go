package commands

import (
	"os"
	"sort"

	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/spf13/cobra"
)

type initOutputPayload struct {
	RepoName string
}
type initInputPayload struct{}

func initRepo(info runner.RunnerInfo[initOutputPayload, initInputPayload]) {
	var err error
	res := initOutputPayload{
		RepoName: info.RepoData.Name,
	}
	defer func() {
		info.Result <- runner.NewRunnerResult(err, res)
	}()

	err = info.Git.Clone(git.CloneOps{
		Remote: info.RepoData.GitConfig.Remote,
		Path:   info.RepoData.GitConfig.Path,
		Refs:   info.RepoData.GitConfig.Refs,
	})
}

func localInitFactory(ops commandOps) *cobra.Command {
	return &cobra.Command{
		Use:   "local",
		Short: "Use a local configuration to clones the configured repos",
		Run: func(cmd *cobra.Command, args []string) {
			repos := ops.ConfigManager.GetRepos()
			wc := make(chan runner.RunnerResult[initOutputPayload], 0)
			wp := runner.WrapRunnerFunc(initRepo, runner.RunnerFuncWrapperInfo[initOutputPayload]{
				Git:    ops.git,
				Result: wc,
			})
			for _, repo := range repos {
				go wp(repo, initInputPayload{})
			}

			// TODO: change this allocations for given-size arrays
			res := []runner.RunnerResult[initOutputPayload]{}
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
					logger.Success("successfully cloned")
				}
			}
		},
	}
}

func initFactory(ops commandOps) *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the multi-repo architecture",
	}

	// TODO: introduce a remote init command that gets the location
	// of a remote configuration file
	initCmd.AddCommand(localInitFactory(ops))

	return initCmd
}
