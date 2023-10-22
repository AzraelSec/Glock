package switchcmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type switchGit struct {
	repos []config.LiveRepo
	git.Git
}

type switchInputPayload struct {
	gitRepo      git.Repo
	targetBranch string
	force        bool
}

func (sg *switchGit) switchRepo(payload switchInputPayload) error {
	if !dir.DirExists(payload.gitRepo.Path) {
		return config.RepoNotFoundErr
	}

	if err := sg.Fetch(payload.gitRepo); err != nil {
		return err
	}

	return sg.Switch(
		payload.gitRepo,
		git.BranchName(payload.targetBranch),
		payload.force,
	)
}

func (sg *switchGit) performSwitch(branch string, force bool) []runner.Result[struct{}] {
	switchArgs := make([]switchInputPayload, 0, len(sg.repos))
	switchFn := func(args switchInputPayload) (struct{}, error) {
		return struct{}{}, sg.switchRepo(args)
	}

	for _, repo := range sg.repos {
		switchArgs = append(switchArgs, switchInputPayload{
			targetBranch: branch,
			force:        force,
			gitRepo:      repo.GitConfig,
		})
	}

	return runner.Run(switchFn, switchArgs)
}

func printRichResults(repos []config.LiveRepo, results []runner.Result[struct{}]) {
	re := lipgloss.NewRenderer(os.Stdout)
	baseStyle := re.NewStyle().
		Padding(0, 1)
	headerStyle := baseStyle.Copy().
		Foreground(lipgloss.Color("252")).
		Bold(true)
	headers := []string{"REPO", "STATUS"}

	touched := false
	switchedData := make([][]string, 0)
	for idx, res := range results {
		if res.Error == nil {
			touched = true
			switchedData = append(switchedData, []string{repos[idx].Name, "Done!"})
			continue
		}

		if !errors.Is(res.Error, git.InvalidReferenceErr) {
			touched = true
			switchedData = append(switchedData, []string{repos[idx].Name, results[idx].Error.Error()})
		}
	}

	if !touched {
		fmt.Println("Done, but no repos changed its current branch!")
		return
	}

	switchedTable := table.New().
		Border(lipgloss.NormalBorder()).
		Headers(headers...).
		BorderStyle(re.NewStyle().Foreground(lipgloss.Color("238"))).
		Rows(switchedData...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return headerStyle
			}
			return baseStyle
		})

	fmt.Println(switchedTable.Render())
}

type switchCmd struct {
	cm *config.ConfigManager
	g  git.Git
}

func New(cm *config.ConfigManager, g git.Git) *switchCmd {
	return &switchCmd{cm, g}
}

func (s *switchCmd) Command() *cobra.Command {
	var force *bool

	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Changes the branch to the given one - for all the repos that has it",
		Run: func(cmd *cobra.Command, args []string) {
			if !*force && !routine.AllClean(s.cm.Repos, s.g) {
				color.Red("Some of the repositories are not clean - it's not safe to switch")
				return
			}

			sg := &switchGit{
				repos: s.cm.Repos,
				Git:   s.g,
			}

			if len(args) > 0 {
				newCli(sg).run(args[0], *force)
			} else {
				newTui(sg).run(*force)
			}
		},
	}

	force = cmd.Flags().BoolP("force", "f", false, "Force the switch even if some changes exist")

	return cmd
}
