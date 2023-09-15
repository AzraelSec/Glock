package commands

import (
	"fmt"
	"strings"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type statusOutputPayload struct {
	Branch     string
	Dirty      bool
	RemoteDiff bool
	RepoName   string
}

func statusRepo(g git.Git, gr git.Repo) (statusOutputPayload, error) {
	res := statusOutputPayload{}

	if !dir.DirExists(gr.Path) {
		return res, config.RepoNotFoundErr
	}

	status, err := g.Status(gr)
	if err != nil {
		return res, err
	}
	diff, _ := g.DiffersFromRemote(gr)

	res.Branch = string(status.Branch)
	res.Dirty = status.Change
	res.RemoteDiff = diff

	return res, nil
}

func prettyPrint(repoName string, maxLength int, result runner.Result[statusOutputPayload]) string {
	redBold := color.New(color.Bold).Add(color.FgRed).SprintfFunc()
	blueBold := color.New(color.Bold).Add(color.FgBlue).SprintfFunc()

	padding := strings.Repeat(" ", maxLength-len(repoName))
	name := repoName
	branch := string(result.Res.Branch)

	if result.Error != nil {
		return fmt.Sprintf("[%s] %s=> ERROR: %s\n", redBold(repoName), padding, redBold(result.Error.Error()))
	}

	changeLabel := ""
	if result.Res.Dirty {
		changeLabel = "🛠 "
		name = blueBold(name)
		branch = blueBold(branch)
	}

	diffLabel := ""
	if result.Res.RemoteDiff {
		diffLabel = "🧨"
	}

	return fmt.Sprintf("[%s] %s=> %s %s %s\n", name, padding, branch, changeLabel, diffLabel)
}

func statusFactory(cm *config.ConfigManager, g git.Git) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Retrieves the current branch on each repo",
		Run: func(cmd *cobra.Command, args []string) {
			repos := cm.Repos
			lrn := 1

			statusArgs := make([]git.Repo, 0, len(repos))
			statusFn := func(gr git.Repo) (statusOutputPayload, error) {
				return statusRepo(g, gr)
			}

			for _, repo := range repos {
				if lg := len(repo.Name); lg > lrn {
					lrn = lg
				}
				statusArgs = append(statusArgs, repo.GitConfig)
			}

			results := runner.Run(statusFn, statusArgs)

			for i, res := range results {
				fmt.Print(prettyPrint(repos[i].Name, lrn, res))
			}
		},
	}
}
