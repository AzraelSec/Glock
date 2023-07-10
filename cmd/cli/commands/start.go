package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/AzraelSec/glock/pkg/shell"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type startInputPayload struct {
	gitPath  string
	startCmd string
	stopCmd  string
}

type startOutputPayload struct {
	Pid     int
	RetCode int
}

func startRepo(ctx context.Context, g git.Git, payload startInputPayload) (startOutputPayload, error) {
	res := startOutputPayload{
		Pid:     -1,
		RetCode: -1,
	}

	if !dir.DirExists(payload.gitPath) {
		return res, config.RepoNotFoundErr
	}

	startProcess := shell.NewSyncShell(shell.ShellOps{
		ExecPath: payload.gitPath,
		Cmd:      payload.startCmd,
		Ctx:      ctx,
	})
	if rc, err := startProcess.Start(os.Stdout); shell.IgnoreInterrupt(err) != nil {
		return res, fmt.Errorf("impossible to start => %v", err)
	} else {
		res.RetCode = rc
		res.Pid = startProcess.Pid
	}

	if payload.stopCmd == "" {
		return res, nil
	}

	stopProcess := shell.NewSyncShell(shell.ShellOps{
		ExecPath: payload.gitPath,
		Cmd:      payload.stopCmd,
	})
	if _, err := stopProcess.Start(os.Stdout); err != nil {
		return res, fmt.Errorf("impossible to end => %v", err)
	}
	return res, nil
}

func startFactory(cm *config.ConfigManager, g git.Git) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the development stack 🚀",
		Run: func(cmd *cobra.Command, args []string) {
			repos := cm.GetRepos()
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
			defer stop()

			filteredRepos := []config.LiveRepo{}
			for _, repo := range repos {
				if repo.Config.StartCmd == "" {
					continue
				}
				filteredRepos = append(filteredRepos, repo)
			}

			startArgs := make([]startInputPayload, 0, len(filteredRepos))
			startFn := func(args startInputPayload) (startOutputPayload, error) {
				return startRepo(ctx, g, args)
			}

			for _, repo := range filteredRepos {
				startArgs = append(startArgs, startInputPayload{
					startCmd: repo.Config.StartCmd,
					stopCmd:  repo.Config.StopCmd,
					gitPath:  repo.GitConfig.Path,
				})
			}

			runner.Run(startFn, startArgs)

			color.Green("Execution completed")
		},
	}
}
