package status

import (
	"strings"

	"github.com/AzraelSec/glock/internal/ui"
	"github.com/spf13/cobra"
)

const (
	needsAttention      = "Needs attention"
	upstreamUnavailable = "Upstream unavailable"
	cleanAndTracked     = "Clean and tracked"
)

func (s *status) runTui(cmd *cobra.Command) error {
	tasks := make([]ui.Task, 0, len(s.cm.Repos))
	for _, repo := range s.cm.Repos {
		tasks = append(tasks, ui.Task{Name: repo.Name, Run: func() ui.Result {
			result, err := s.statusRepo(repo.GitConfig)
			if err != nil {
				return ui.Result{Outcome: ui.Failure, Label: "Unavailable", Detail: ui.ErrorDetail(err), Group: needsAttention}
			}

			branch := result.branch
			if branch == "HEAD" {
				branch = "Detached HEAD"
			}
			details := []string{branch}
			label := "Clean"
			outcome := ui.Success
			group := cleanAndTracked
			if result.dirty {
				label = "Dirty"
				outcome = ui.Attention
				group = needsAttention
			}
			if result.remoteDiff {
				details = append(details, "differs from upstream")
				outcome = ui.Attention
				group = needsAttention
			}
			if result.upstreamUnavailable {
				if group == cleanAndTracked {
					group = upstreamUnavailable
					outcome = ui.Attention
				} else {
					details = append(details, "upstream unavailable")
				}
			}
			return ui.Result{Outcome: outcome, Label: label, Detail: strings.Join(details, " · "), Group: group}
		}})
	}
	return (ui.Batch{
		Title: "Repository status", Activity: "Checking repositories",
		Empty: "No repositories to check.", Tasks: tasks, ReadOnly: true,
		Groups: []string{needsAttention, upstreamUnavailable, cleanAndTracked},
	}).Run(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
}
