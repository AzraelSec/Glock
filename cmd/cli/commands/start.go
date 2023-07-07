package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/shell"
	"github.com/AzraelSec/glock/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type startInputPayload struct {
	startCmd string
	stopCmd  string
}

type startOutputPayload struct {
	Pid      int
	RepoName string
	RetCode  int
}

func startRepo(info runner.RunnerInfo[startOutputPayload, startInputPayload]) {
	var err error
	resultPayload := startOutputPayload{
		Pid:      -1,
		RepoName: info.RepoData.Name,
		RetCode:  -1,
	}
	defer func() {
		info.Result <- runner.NewRunnerResult(err, resultPayload)
	}()

	if !utils.DirExists(info.RepoData.GitConfig.Path) {
		err = config.RepoNotFoundErr
		return
	}

	startProcess := shell.NewSyncShell(shell.ShellOps{
		ExecPath: info.RepoData.GitConfig.Path,
		Cmd:      info.Args.startCmd,
		Ctx:      info.Context,
	})
	if rc, err := startProcess.Start(os.Stdout); shell.IgnoreInterrupt(err) != nil {
		err = fmt.Errorf("impossible to start => %v", err)
		return
	} else {
		resultPayload.RetCode = rc
		resultPayload.Pid = startProcess.Pid
	}

	if info.Args.stopCmd == "" {
		return
	}

	stopProcess := shell.NewSyncShell(shell.ShellOps{
		ExecPath: info.RepoData.GitConfig.Path,
		Cmd:      info.Args.stopCmd,
	})
	if _, err = stopProcess.Start(os.Stdout); err != nil {
		err = fmt.Errorf("impossible to end => %v", err)
		return
	}
}

func startFactory(ops commandOps) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the development stack 🚀",
		Run: func(cmd *cobra.Command, args []string) {
			tw := 0
			waitingChannel := make(chan runner.RunnerResult[startOutputPayload])
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
			defer stop()

			wp := runner.WrapRunnerFunc(startRepo, runner.RunnerFuncWrapperInfo[startOutputPayload]{
				Context: ctx,
				Output:  os.Stdout,
				Git:     ops.git,
				Result:  waitingChannel,
			})

			for _, repo := range ops.ConfigManager.GetRepos() {
				// TODO: this should be checked inside the goroutine function
				if repo.Config.StartCmd == "" {
					continue
				}

				tw = tw + 1
				go wp(repo, startInputPayload{
					startCmd: repo.Config.StartCmd,
					stopCmd:  repo.Config.StopCmd,
				})
			}

			res := []runner.RunnerResult[startOutputPayload]{}
			for i := 0; i < tw; i++ {
				res = append(res, <-waitingChannel)
			}

			color.Green("Execution completed")
		},
	}
}
