package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type statusOutputPayload struct {
	Branch     string
	Dirty      bool
	Padding    int
	RemoteDiff bool
	RepoName   string
}
type statusInputPayload struct {
	maxLength int
}

func statusRepo(info runner.RunnerInfo[statusOutputPayload, statusInputPayload]) {
	var err error
	res := statusOutputPayload{
		RepoName: info.RepoData.Name,
	}
	defer func() {
		info.Result <- runner.NewRunnerResult(err, res)
	}()

	if !utils.DirExists(info.RepoData.GitConfig.Path) {
		err = config.RepoNotFoundErr
		return
	}

	status, ierr := info.Git.Status(info.RepoData.GitConfig)
	if ierr != nil {
		err = ierr
		return
	}
	diff, _ := info.Git.DiffersFromRemote(info.RepoData.GitConfig)

	res.Padding = info.Args.maxLength - len(info.RepoData.Name)
	res.Branch = string(status.Branch)
	res.Dirty = status.Change
	res.RemoteDiff = diff
}

func prettyPrint(args runner.RunnerResult[statusOutputPayload]) string {
	redBold := color.New(color.Bold).Add(color.FgRed).SprintfFunc()
	blueBold := color.New(color.Bold).Add(color.FgBlue).SprintfFunc()

	name := args.Result.RepoName
	padding := strings.Repeat(" ", args.Result.Padding)
	branch := string(args.Result.Branch)

	if args.Error != nil {
		return fmt.Sprintf("[%s] %s=> ERROR: %s\n", redBold(name), padding, redBold(args.Error.Error()))
	}

	changeLabel := ""
	if args.Result.Dirty {
		changeLabel = "🛠 "
		name = blueBold(name)
		branch = blueBold(branch)
	}

	diffLabel := ""
	if args.Result.RemoteDiff {
		diffLabel = "🧨"
	}

	return fmt.Sprintf("[%s] %s=> %s %s %s\n", name, padding, branch, changeLabel, diffLabel)
}

func statusFactory(ops commandOps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Retrieves the current branch on each repo",
		Run: func(cmd *cobra.Command, args []string) {
			repos := ops.ConfigManager.GetRepos()
			wc := make(chan runner.RunnerResult[statusOutputPayload])
			wp := runner.WrapRunnerFunc(statusRepo, runner.RunnerFuncWrapperInfo[statusOutputPayload]{
				Git:    ops.git,
				Result: wc,
			})

			lrn := 1
			for _, repo := range repos {
				if lg := len(repo.Name); lg > lrn {
					lrn = lg
				}
			}

			for _, repo := range repos {
				go wp(repo, statusInputPayload{maxLength: lrn})
			}

			res := []struct {
				repoName string
				output   string
			}{}
			for i := 0; i < len(repos); i++ {
				info := <-wc
				res = append(res, struct {
					repoName string
					output   string
				}{
					repoName: info.Result.RepoName,
					output:   prettyPrint(info),
				})
			}
			sort.Slice(res, func(i, j int) bool {
				return res[i].repoName < res[j].repoName
			})
			for _, rs := range res {
				fmt.Print(rs.output)
			}
		},
	}
}
