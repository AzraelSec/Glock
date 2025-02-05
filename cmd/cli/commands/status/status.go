package status

import (
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/spf13/cobra"
)

type statusOutputPayload struct {
	branch     string
	dirty      bool
	remoteDiff bool
}

type status struct {
	cm  *config.ConfigManager
	g   git.Git
	err error
}

func (s *status) statusRepo(gr git.Repo) (statusOutputPayload, error) {
	res := statusOutputPayload{}

	if !dir.DirExists(gr.Path) {
		return res, config.ErrRepoNotFound
	}

	status, err := s.g.Status(gr)
	if err != nil {
		return res, err
	}
	diff, _ := s.g.DiffersFromRemote(gr)

	res.branch = string(status.Branch)
	res.dirty = status.Change
	res.remoteDiff = diff

	return res, nil
}

func (s *status) collect() []runner.Result[statusOutputPayload] {
	statusArgs := make([]git.Repo, 0, len(s.cm.Repos))
	statusFn := func(gr git.Repo) (statusOutputPayload, error) {
		return s.statusRepo(gr)
	}

	for _, repo := range s.cm.Repos {
		statusArgs = append(statusArgs, repo.GitConfig)
	}

	return runner.Run(statusFn, statusArgs)
}

func NewStatus(dm *dependency.DependencyManager) *status {
	s := &status{}
	s.g, s.err = dm.GetGit()
	if s.err != nil {
		return s
	}
	s.cm, s.err = dm.GetConfigManager()
	return s
}

func (s *status) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Retrieves the current branch on each repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			if s.err != nil {
				return s.err
			}

			// todo: add CLI version
			s.runTui()
			return nil
		},
	}
}
