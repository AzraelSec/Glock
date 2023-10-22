package multiplexer

import (
	"context"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/runner"
)

type (
	runHandler func(startInputPayload) (startOutputPayload, error)
	runArgs    []startInputPayload
)

type multiplexer struct {
	serviceStartHandler runHandler
	serviceStartArgs    runArgs
	serviceStopHandler  runHandler
	serviceStopArgs     runArgs

	repoStartHander runHandler
	repoStartArgs   runArgs
	repoStopHander  runHandler
	repoStopArgs    runArgs
}

func newMultiplexer(ctxStart context.Context, out io.Writer, cm *config.ConfigManager, repos []config.LiveRepo) *multiplexer {
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

	startServiceArgs := make([]startInputPayload, 0, len(executableService))
	startServiceFn := func(payload startInputPayload) (startOutputPayload, error) {
		return processRun(ctxStart, cm.EnvFilenames, payload)
	}
	for _, srv := range executableService {
		startServiceArgs = append(startServiceArgs, startInputPayload{
			cmd:  srv.Cmd,
			out:  log.NewTagStreamWriter(srv.Tag, out),
			path: path.Dir(cm.ConfigPath),
		})
	}
	startArgs := make([]startInputPayload, 0, len(executableRepo))
	startFn := func(args startInputPayload) (startOutputPayload, error) {
		return processRun(ctxStart, cm.EnvFilenames, args)
	}
	for _, repo := range executableRepo {
		for _, startCmd := range repo.Config.OnStart {
			startArgs = append(startArgs, startInputPayload{
				cmd:  startCmd,
				path: repo.GitConfig.Path,
				out:  log.NewTagStreamWriter(repo.Name, out),
			})
		}
	}

	// TODO: better define a mechanism for a timeout
	stopServiceArgs := make([]startInputPayload, 0, len(disposableService))
	stopServiceFn := func(payload startInputPayload) (startOutputPayload, error) {
		return processRun(context.TODO(), cm.EnvFilenames, payload)
	}
	for _, srv := range disposableService {
		stopServiceArgs = append(stopServiceArgs, startInputPayload{
			cmd:  srv.Dispose,
			out:  log.NewTagStreamWriter(srv.Tag, os.Stdout),
			path: path.Dir(cm.ConfigPath),
		})
	}
	stopArgs := make([]startInputPayload, 0, len(disposableRepo))
	stopFn := func(args startInputPayload) (startOutputPayload, error) {
		return processRun(context.TODO(), cm.EnvFilenames, args)
	}
	for _, repo := range executableRepo {
		stopArgs = append(stopArgs, startInputPayload{
			cmd:  repo.Config.OnStop,
			path: repo.GitConfig.Path,
			out:  log.NewTagStreamWriter(repo.Name, os.Stdout),
		})
	}

	return &multiplexer{
		serviceStartHandler: startServiceFn,
		serviceStartArgs:    startServiceArgs,
		serviceStopHandler:  stopServiceFn,
		serviceStopArgs:     stopServiceArgs,

		repoStartHander: startFn,
		repoStartArgs:   startArgs,
		repoStopHander:  stopFn,
		repoStopArgs:    stopArgs,
	}
}

func (m *multiplexer) start() {
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		runner.Run(m.serviceStartHandler, m.serviceStartArgs)
		wg.Done()
	}()
	// note: is there a way to get rid of this?
	time.Sleep(5 * time.Second)
	go func() {
		runner.Run(m.repoStartHander, m.repoStartArgs)
		wg.Done()
	}()
	wg.Wait()

	wg.Add(2)
	go func() {
		runner.Run(m.repoStopHander, m.repoStopArgs)
		wg.Done()
	}()
	go func() {
		runner.Run(m.serviceStopHandler, m.serviceStopArgs)
		wg.Done()
	}()
	wg.Wait()
}
