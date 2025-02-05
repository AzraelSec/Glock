package routine

import (
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
)

func AllClean(repos []config.LiveRepo, g git.Git) bool {
	hasChanges := func(repo config.LiveRepo) (bool, error) {
		if !dir.DirExists(repo.GitConfig.Path) {
			return false, config.ErrRepoNotFound
		}
		return g.HasChanges(repo.GitConfig)
	}

	results := runner.Run(hasChanges, repos)

	exit := true
	for i := 0; exit && i < len(repos); i++ {
		res := results[i]
		// TODO: enforce this error handling
		if res.Error != nil {
			continue
		}
		exit = exit && !res.Res // <- I'm not sure if it should be && or ||, and if there should be a ! before res.Res
	}
	return exit
}
