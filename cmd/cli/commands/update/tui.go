package update

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/AzraelSec/glock/internal/ui"
	"github.com/spf13/cobra"
)

func (u *update) runTui(cmd *cobra.Command) error {
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	updateFn, args := runnerArgs(ctx, u.cm.Repos, false)
	tasks := make([]ui.Task, 0, len(u.cm.Repos))
	for i, repo := range u.cm.Repos {
		tasks = append(tasks, ui.Task{Name: repo.Name, Run: func() ui.Result {
			result, err := updateFn(args[i])
			switch {
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return ui.Result{Outcome: ui.Canceled, Label: "Canceled"}
			case err != nil:
				return ui.Result{Outcome: ui.Failure, Label: "Failed", Detail: err.Error()}
			case result.Ignored:
				return ui.Result{Outcome: ui.Skipped, Label: "Skipped", Detail: "Updater disabled in config"}
			}

			source := "configured"
			if result.Inferred {
				source = "inferred"
			}
			return ui.Result{Label: "Updated", Detail: result.UpdaterTag + " (" + source + ")"}
		}})
	}

	return (ui.Batch{
		Title: "Dependency updates", Activity: "Updating repositories",
		Empty: "No repositories to update.", Tasks: tasks, Cancel: cancel, ExpandFailures: true,
	}).Run(ctx, cmd.InOrStdin(), cmd.OutOrStdout())
}
