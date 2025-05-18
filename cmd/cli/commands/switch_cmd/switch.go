package switchcmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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
		return config.ErrRepoNotFound
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

func printResults(repos []config.LiveRepo, results []runner.Result[struct{}]) {
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

		if !errors.Is(res.Error, git.ErrInvalidReference) {
			touched = true
			switchedData = append(switchedData, []string{repos[idx].Name, results[idx].Error.Error()})
		}
	}

	if !touched {
		fmt.Println("Done, but no repos changed its current branch!")
		return
	}

	switchedTable := table.New().
		Border(lipgloss.RoundedBorder()).
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
	cm  *config.ConfigManager
	g   git.Git
	err error
}

func New(dm *dependency.Manager) *switchCmd {
	scmd := &switchCmd{}
	scmd.g, scmd.err = dm.GetGit()
	if scmd.err != nil {
		return scmd
	}
	scmd.cm, scmd.err = dm.GetConfigManager()
	return scmd
}

func (s *switchCmd) Command() *cobra.Command {
	var force *bool

	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Changes the branch to the given one - for all the repos that has it",
		RunE: func(cmd *cobra.Command, args []string) error {
			if s.err != nil {
				return s.err
			}

			if !*force && !routine.AllClean(s.cm.Repos, s.g) {
				return errors.New("some of the repositories are not clean - it's not safe to switch")
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

			return nil
		},
	}

	force = cmd.Flags().BoolP("force", "f", false, "Force the switch even if some changes exist")

	return cmd
}
