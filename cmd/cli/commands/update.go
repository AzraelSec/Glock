package commands

import (
	"context"
	"os"
	"os/signal"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/updater"
	"github.com/AzraelSec/glock/pkg/utils"
	"github.com/spf13/cobra"
)

type updateInputPayload struct {
	UpdaterTag string
}
type updateOutputPayload struct {
	RepoName   string
	UpdaterTag string
	Inferred   bool
}

func updateStart(info runner.RunnerInfo[updateOutputPayload, updateInputPayload]) {
	var err error
	resultPayload := updateOutputPayload{
		RepoName: info.RepoData.Name,
	}
	defer func() {
		info.Result <- runner.NewRunnerResult(err, resultPayload)
	}()

	if !utils.DirExists(info.RepoData.GitConfig.Path) {
		err = config.RepoNotFoundErr
		return
	}

	// TODO: unroll this some way
	if info.Args.UpdaterTag != "" {
		repoUpdater, err := updater.MatchByTag(info.Args.UpdaterTag)
		if err == nil {
			resultPayload.UpdaterTag = repoUpdater.Tag()
			err = repoUpdater.Update(info.Context, info.Output, info.RepoData.GitConfig.Path)
			return
		}
	}

	d, err := utils.NewDirectory(info.RepoData.GitConfig.Path)
	if err != nil {
		return
	}
	repoUpdater, err := updater.Infer(d)
	if err != nil {
		return
	}
	err = repoUpdater.Update(info.Context, info.Output, info.RepoData.GitConfig.Path)
	if err != nil {
		return
	}
	resultPayload.UpdaterTag = repoUpdater.Tag()
}

func updaterListStart(info runner.RunnerInfo[updateOutputPayload, updateInputPayload]) {
	var err error
	resultPayload := updateOutputPayload{
		RepoName: info.RepoData.Name,
	}
	defer func() {
		info.Result <- runner.NewRunnerResult(err, resultPayload)
	}()

	if !utils.DirExists(info.RepoData.GitConfig.Path) {
		return
	}

	if info.Args.UpdaterTag != "" {
		if repoUpdater, err := updater.MatchByTag(info.Args.UpdaterTag); err != nil {
			resultPayload.UpdaterTag = repoUpdater.Tag()
			return
		}
	}

	d, err := utils.NewDirectory(info.RepoData.GitConfig.Path)
	if err != nil {
		return
	}
	repoUpdater, err := updater.Infer(d)
	if err != nil {
		return
	}
	resultPayload.UpdaterTag = repoUpdater.Tag()
	resultPayload.Inferred = true
}

func updateFactory(ops commandOps) *cobra.Command {
	var list *bool
	var output *bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Updates your repositories",
		Run: func(cmd *cobra.Command, args []string) {
			wc := make(chan runner.RunnerResult[updateOutputPayload])
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
			defer stop()

			handler := updateStart
			if *list == true {
				handler = updaterListStart
			}
			rConfig := runner.RunnerFuncWrapperInfo[updateOutputPayload]{
				Context: ctx,
				Git:     ops.git,
				Result:  wc,
			}
			if *output {
				rConfig.Output = os.Stdout
			}
			wp := runner.WrapRunnerFunc(handler, rConfig)

			repos := ops.ConfigManager.GetRepos()
			for _, repo := range ops.ConfigManager.GetRepos() {
				go wp(repo, updateInputPayload{
					UpdaterTag: repo.Config.Updater,
				})
			}

			for i := 0; i < len(repos); i++ {
				res := <-wc
				logger := log.NewRepoLogger(os.Stdout, res.Result.RepoName)

				if *list {
					if res.Error != nil {
						logger.Error(res.Error.Error())
						continue
					}

					if res.Result.Inferred {
						logger.Info(": inferred => %s", res.Result.UpdaterTag)
					} else {
						logger.Info(": matching => %s", res.Result.UpdaterTag)
					}
					continue
				}

				if res.Error != nil {
					logger.Error("not updated => %s", res.Error.Error())
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
