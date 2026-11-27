package switchcmd

import (
	"errors"
	"io"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/AzraelSec/glock/internal/ui"
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

func (sg *switchGit) runSwitch(cmd *cobra.Command, in io.Reader, branch string, force bool, plan []branchImpact) error {
	if plan == nil {
		plan = make([]branchImpact, 0, len(sg.repos))
		for _, repo := range sg.repos {
			plan = append(plan, branchImpact{repo: repo, kind: eligible})
		}
	}
	tasks := make([]ui.Task, 0, len(plan))
	for _, impact := range plan {
		repo := impact.repo
		tasks = append(tasks, ui.Task{Name: repo.Name, Run: func() ui.Result {
			switch impact.kind {
			case alreadyOnBranch:
				return ui.Result{Outcome: ui.Skipped, Label: "Already on branch"}
			case branchUnavailable:
				return ui.Result{Outcome: ui.Skipped, Label: "Skipped", Detail: impact.detail}
			case unavailable:
				return ui.Result{Outcome: ui.Failure, Label: "Unavailable", Detail: impact.detail}
			}
			err := sg.switchRepo(switchInputPayload{gitRepo: repo.GitConfig, targetBranch: branch, force: force})
			switch {
			case errors.Is(err, git.ErrInvalidReference):
				return ui.Result{Outcome: ui.Skipped, Label: "Skipped", Detail: "Branch unavailable"}
			case err != nil:
				return ui.Result{Outcome: ui.Failure, Label: "Failed", Detail: ui.ErrorDetail(err)}
			default:
				return ui.Result{Label: "Switched"}
			}
		}})
	}

	return (ui.Batch{
		Title: "Switch to " + branch, Activity: "Switching repositories",
		Empty: "No repositories to switch.", Tasks: tasks,
	}).Run(cmd.Context(), in, cmd.OutOrStdout())
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
		Use:   "switch [branch]",
		Short: "Switch repositories to a branch",
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
				return sg.runSwitch(cmd, nil, args[0], *force, nil)
			}
			return sg.runPicker(cmd, *force)
		},
	}

	force = cmd.Flags().BoolP("force", "f", false, "Force the switch even if some changes exist")

	return cmd
}
