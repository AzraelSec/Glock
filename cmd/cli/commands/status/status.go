package status

import (
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/spf13/cobra"
)

type statusOutputPayload struct {
	branch              string
	dirty               bool
	remoteDiff          bool
	upstreamUnavailable bool
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
	diff, diffErr := s.g.DiffersFromRemote(gr)

	res.branch = string(status.Branch)
	res.dirty = status.Change
	res.remoteDiff = diff
	res.upstreamUnavailable = diffErr != nil

	return res, nil
}

func NewStatus(dm *dependency.Manager) *status {
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
		Short: "Show the current branch and Git status for each repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			if s.err != nil {
				return s.err
			}

			return s.runTui(cmd)
		},
	}
}
