package update

import (
	"context"
	"io"
	"os"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/spf13/cobra"
)

type updateInputPayload struct {
	UpdaterTag string
	RepoPath   string
}
type updateOutputPayload struct {
	UpdaterTag string
	Inferred   bool
	Ignored    bool
}

func updateStart(ctx context.Context, out io.Writer, payload updateInputPayload) (updateOutputPayload, error) {
	res := updateOutputPayload{}
	if !dir.DirExists(payload.RepoPath) {
		return res, config.ErrRepoNotFound
	}

	// TODO: unroll this some way
	if payload.UpdaterTag != "" {
		if isIgnoreUpdaterTag(payload.UpdaterTag) {
			res.Ignored = true
			return res, nil
		}

		repoUpdater, err := matchUpdaterByTag(payload.UpdaterTag)
		if err == nil {
			res.UpdaterTag = repoUpdater.Tag()
			err := repoUpdater.Update(ctx, out, payload.RepoPath)
			return res, err
		}
	}

	d, err := dir.NewDirectory(payload.RepoPath)
	if err != nil {
		return res, err
	}
	repoUpdater, err := inferUpdater(d)
	if err != nil {
		return res, err
	}

	if err := repoUpdater.Update(
		ctx,
		out,
		payload.RepoPath,
	); err != nil {
		return res, err
	}

	res.Inferred = true
	res.UpdaterTag = repoUpdater.Tag()
	return res, nil
}

type updateRunnerFunc func(args updateInputPayload) (updateOutputPayload, error)

func runnerArgs(ctx context.Context, repos []config.LiveRepo, output bool) (updateRunnerFunc, []updateInputPayload) {
	updateArgs := make([]updateInputPayload, 0, len(repos))
	updateFn := func(args updateInputPayload) (updateOutputPayload, error) {
		var out io.Writer
		if output {
			out = os.Stdout
		}
		return updateStart(ctx, out, args)
	}

	for _, repo := range repos {
		updateArgs = append(updateArgs, updateInputPayload{
			UpdaterTag: repo.Config.Updater,
			RepoPath:   repo.GitConfig.Path,
		})
	}

	return updateFn, updateArgs
}

type update struct {
	cm  *config.ConfigManager
	err error
}

func NewUpdate(dm *dependency.Manager) *update {
	u := &update{}
	u.cm, u.err = dm.GetConfigManager()
	return u
}

func (u *update) Command() *cobra.Command {
	var output, disableTui *bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update your repositories (download dependencies)",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// note: disable output when tui is enabled
			v, err := cmd.Flags().GetBool("output")
			if err != nil {
				return err
			}

			if v {
				return cmd.MarkFlagRequired("disableTui")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if u.err != nil {
				return u.err
			}

			if *disableTui {
				newCli(u.cm.Repos, *output).run()
			} else {
				return newTui(u.cm.Repos).run()
			}

			return nil
		},
	}

	output = cmd.Flags().BoolP("output", "o", false, "Print the updaters output on stdout (mostly debugging purposes)")
	disableTui = cmd.Flags().Bool("disableTui", false, "Disable TUI")

	return cmd
}
