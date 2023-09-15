package commands

import (
	"context"
	"io"
	"os"
	"os/signal"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/AzraelSec/glock/pkg/updater"
	"github.com/spf13/cobra"
)

type updateInputPayload struct {
	UpdaterTag string
	RepoPath   string
}
type updateOutputPayload struct {
	UpdaterTag string
	Inferred   bool
	Skipped    bool
}

func updateStart(ctx context.Context, g git.Git, out io.Writer, payload updateInputPayload) (updateOutputPayload, error) {
	res := updateOutputPayload{}
	if !dir.DirExists(payload.RepoPath) {
		return res, config.RepoNotFoundErr
	}

	// TODO: unroll this some way
	if payload.UpdaterTag != "" {
		if payload.UpdaterTag == updater.IGNORE_TAG {
			res.Skipped = true
			return res, nil
		}

		repoUpdater, err := updater.MatchByTag(payload.UpdaterTag)
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
	repoUpdater, err := updater.Infer(d)
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

	res.UpdaterTag = repoUpdater.Tag()
	return res, nil
}

func updaterListStart(ctx context.Context, g git.Git, payload updateInputPayload) (updateOutputPayload, error) {
	res := updateOutputPayload{}
	if !dir.DirExists(payload.RepoPath) {
		return res, config.RepoNotFoundErr
	}

	if payload.UpdaterTag != "" {
		if payload.UpdaterTag == updater.IGNORE_TAG {
			res.Skipped = true
			return res, nil
		}
		if repoUpdater, err := updater.MatchByTag(payload.UpdaterTag); err != nil {
			res.UpdaterTag = repoUpdater.Tag()
			return res, err
		}
	}

	d, err := dir.NewDirectory(payload.RepoPath)
	if err != nil {
		return res, err
	}
	repoUpdater, err := updater.Infer(d)
	if err != nil {
		return res, err
	}
	res.UpdaterTag = repoUpdater.Tag()
	res.Inferred = true
	return res, nil
}

func updateFactory(cm *config.ConfigManager, g git.Git) *cobra.Command {
	var list *bool
	var output *bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates your repositories",
		Run: func(cmd *cobra.Command, args []string) {
			repos := cm.Repos
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
			defer stop()

			updateArgs := make([]updateInputPayload, 0, len(repos))
			updateFn := func(args updateInputPayload) (updateOutputPayload, error) {
				if *list {
					return updaterListStart(ctx, g, args)
				}

				var out io.Writer = nil
				if *output {
					out = os.Stdout
				}
				return updateStart(ctx, g, out, args)
			}

			for _, repo := range repos {
				updateArgs = append(updateArgs, updateInputPayload{
					UpdaterTag: repo.Config.Updater,
					RepoPath:   repo.GitConfig.Path,
				})
			}

			results := runner.Run(updateFn, updateArgs)

			for i, res := range results {
				logger := log.NewRepoLogger(os.Stdout, repos[i].Name)

				if *list {
					if res.Error != nil {
						logger.Error(res.Error.Error())
						continue
					}

					if res.Res.Skipped {
						logger.Info(": skipped")
						continue
					}

					if res.Res.Inferred {
						logger.Info(": inferred => %s", res.Res.UpdaterTag)
					} else {
						logger.Info(": matching => %s", res.Res.UpdaterTag)
					}
					continue
				}

				if res.Error != nil {
					logger.Error("not updated => %s", res.Error.Error())
					continue
				}

				if res.Res.Skipped {
					logger.Info("skipped")
				} else {
					logger.Success("updated 👌")
				}
			}
		},
	}

	list = cmd.Flags().BoolP("list", "l", false, "List the matching updater instead of running them")
	output = cmd.Flags().BoolP("output", "o", false, "Print the updaters output on stdout (mostly debugging purposes)")
	return cmd
}
