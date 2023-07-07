package routine

import (
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/AzraelSec/glock/pkg/utils"
)

type cleanCheckOutputPayload struct {
	Changed  bool
	RepoName string
}

func cleanCheckRepo(info runner.RunnerInfo[cleanCheckOutputPayload, interface{}]) {
	var err error
	res := cleanCheckOutputPayload{
		RepoName: info.RepoData.Name,
	}
	defer func() {
		info.Result <- runner.NewRunnerResult(err, res)
	}()
	if !utils.DirExists(info.RepoData.GitConfig.Path) {
		err = config.RepoNotFoundErr
		return
	}

	res.Changed, err = info.Git.HasChanges(info.RepoData.GitConfig)
}

func AllClean(repos []config.LiveRepo, g git.Git) bool {
	wc := make(chan runner.RunnerResult[cleanCheckOutputPayload], 0)
	wp := runner.WrapRunnerFunc(cleanCheckRepo, runner.RunnerFuncWrapperInfo[cleanCheckOutputPayload]{
		Git:    g,
		Result: wc,
	})

	for _, repo := range repos {
		go wp(repo, nil)
	}

	exit := true
	for i := 0; i < len(repos); i++ {
		res := <-wc
		if res.Error != nil {
			continue
		}
		if res.Result.Changed {
			exit = false
		}
	}
	return exit
}
