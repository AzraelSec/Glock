package external_git

import (
	"github.com/AzraelSec/glock/pkg/git"
	gitcb "github.com/AzraelSec/glock/pkg/git_command_builder"
)

type fetch struct {
	*gitcb.CommandBuilder
}

func newFetch(repo git.Repo) fetch {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("fetch")
	return fetch{cb}
}
