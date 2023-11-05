package commands

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/AzraelSec/glock/pkg/shell"
	"github.com/AzraelSec/godotenv"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type startServicePayload struct {
	tag string
	cmd string
}

func serviceRun(ctx context.Context, g git.Git, configPath string, envFilenames []string, payload startServicePayload) (startOutputPayload, error) {
	res := startOutputPayload{
		Pid:     -1,
		RetCode: -1,
	}

	startProcess := shell.NewSyncShell(shell.ShellOps{
		ExecPath: configPath,
		Cmd:      payload.cmd,
		Ctx:      ctx,
	})

	tsw := log.NewTagStreamWriter(payload.tag, os.Stdout)
	rc, err := startProcess.Start(tsw)
	if err != nil {
		return res, err
	}

	res.RetCode = rc
	res.Pid = startProcess.Pid

	return res, nil
}

type startInputPayload struct {
	gitPath string
	cmd     string
	name    string
}

type startOutputPayload struct {
	Pid     int
	RetCode int
}

func repoRun(ctx context.Context, g git.Git, envFilenames []string, payload startInputPayload) (startOutputPayload, error) {
	res := startOutputPayload{
		Pid:     -1,
		RetCode: -1,
	}

	if !dir.DirExists(payload.gitPath) {
		return res, config.RepoNotFoundErr
	}

	denv, _ := godotenv.ReadFrom(payload.gitPath, false, envFilenames...)
	startProcess := shell.NewSyncShell(shell.ShellOps{
		ExecPath: payload.gitPath,
		Cmd:      payload.cmd,
		Ctx:      ctx,
		Env:      denv,
	})

	tsw := log.NewTagStreamWriter(payload.name, os.Stdout)
	rc, err := startProcess.Start(tsw)
	if err != nil {
		return res, err
	}

	res.RetCode = rc
	res.Pid = startProcess.Pid

	return res, nil
}

func startFactory(dm *dependency.DependencyManager) *cobra.Command {
	var filteredRepos *[]string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the development stack 🚀",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := dm.GetGit()
			if err != nil {
				return err
			}

			cm, err := dm.GetConfigManager()
			if err != nil {
				return err
			}

			repos := []config.LiveRepo{}
			if filteredRepos == nil || len(*filteredRepos) == 0 {
				repos = cm.Repos
			} else {
				for _, repo := range cm.Repos {
					if slices.Contains(*filteredRepos, repo.Name) {
						repos = append(repos, repo)
					}
				}
			}

			if len(repos) == 0 {
				return errors.New("You cannot start your stack with no repos selected")
			}

			executableRepo := []config.LiveRepo{}
			disposableRepo := []config.LiveRepo{}
			for _, repo := range repos {
				if len(repo.Config.OnStart) != 0 {
					executableRepo = append(executableRepo, repo)
				}
				if repo.Config.OnStop != "" {
					disposableRepo = append(disposableRepo, repo)
				}
			}

			executableService := []config.Services{}
			disposableService := []config.Services{}

			for _, srv := range cm.Services {
				if srv.Cmd != "" {
					executableService = append(executableService, srv)
				}
				if srv.Dispose != "" {
					disposableService = append(disposableService, srv)
				}
			}

			onStartCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
			defer stop()
			startServiceArgs := make([]startServicePayload, 0, len(executableService))
			startServiceFn := func(payload startServicePayload) (startOutputPayload, error) {
				return serviceRun(onStartCtx, g, path.Dir(cm.ConfigPath), cm.EnvFilenames, payload)
			}
			for _, srv := range executableService {
				startServiceArgs = append(startServiceArgs, startServicePayload{tag: srv.Tag, cmd: srv.Cmd})
			}

			startArgs := make([]startInputPayload, 0, len(executableRepo))
			startFn := func(args startInputPayload) (startOutputPayload, error) {
				return repoRun(onStartCtx, g, cm.EnvFilenames, args)
			}
			for _, repo := range executableRepo {
				for _, startCmd := range repo.Config.OnStart {
					startArgs = append(startArgs, startInputPayload{
						cmd:     startCmd,
						gitPath: repo.GitConfig.Path,
						name:    repo.Name,
					})
				}
			}

			stopServiceArgs := make([]startServicePayload, 0, len(disposableService))
			stopServiceFn := func(payload startServicePayload) (startOutputPayload, error) {
				return serviceRun(nil, g, path.Dir(cm.ConfigPath), cm.EnvFilenames, payload)
			}
			for _, srv := range disposableService {
				stopServiceArgs = append(stopServiceArgs, startServicePayload{tag: srv.Tag, cmd: srv.Dispose})
			}
			onStopCtx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
			defer cancel()

			stopArgs := make([]startInputPayload, 0, len(disposableRepo))
			stopFn := func(args startInputPayload) (startOutputPayload, error) {
				return repoRun(onStopCtx, g, cm.EnvFilenames, args)
			}
			for _, repo := range executableRepo {
				stopArgs = append(stopArgs, startInputPayload{
					cmd:     repo.Config.OnStop,
					gitPath: repo.GitConfig.Path,
				})
			}

			wg := sync.WaitGroup{}
			wg.Add(2)
			go func() {
				runner.Run(startServiceFn, startServiceArgs)
				wg.Done()
			}()
			// note: is there a way to get rid of this?
			time.Sleep(5 * time.Second)
			go func() {
				runner.Run(startFn, startArgs)
				wg.Done()
			}()
			wg.Wait()

			wg.Add(2)
			go func() {
				runner.Run(stopFn, stopArgs)
				wg.Done()
			}()
			go func() {
				runner.Run(stopServiceFn, stopServiceArgs)
				wg.Done()
			}()
			wg.Wait()

			color.Green("Execution completed")
			return nil
		},
	}

	filteredRepos = cmd.Flags().StringArrayP("repos", "r", nil, "list of repository you want to start")

	return cmd
}
