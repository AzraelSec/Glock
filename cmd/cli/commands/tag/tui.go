package tag

import (
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/ui"
	"github.com/spf13/cobra"
)

func (t *tagCmd) runTui(cmd *cobra.Command, repos []config.LiveRepo, useCurrent, skipPush, pullBefore, isYeet bool) error {
	tagFn, args := runnerArgs(t.g, repos, t.cm.TagPattern, useCurrent, skipPush, pullBefore)
	tasks := make([]ui.Task, 0, len(repos))
	for i, repo := range repos {
		tasks = append(tasks, ui.Task{Name: repo.Name, Run: func() ui.Result {
			result, err := tagFn(args[i])
			if err != nil {
				if result.created {
					return ui.Result{Outcome: ui.Failure, Label: "Created locally · push failed", Detail: result.tag + " · " + ui.ErrorDetail(err)}
				}
				return ui.Result{Outcome: ui.Failure, Label: "Failed", Detail: ui.ErrorDetail(err)}
			}

			detail := result.tag + " · " + result.branch
			if result.remote != "" {
				return ui.Result{Label: "Pushed", Detail: detail + " → " + result.remote}
			}
			return ui.Result{Label: "Created", Detail: detail}
		}})
	}

	batch := ui.Batch{
		Title: "Repository tags", Activity: "Creating and pushing tags", Tasks: tasks, Inline: true,
	}
	if skipPush {
		batch.Activity = "Creating local tags"
	}
	if isYeet {
		batch.Banner = YEET_ASCII_IMAGE
	}
	return batch.Run(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
}
