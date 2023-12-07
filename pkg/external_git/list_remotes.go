package external_git

import (
	"github.com/AzraelSec/glock/pkg/git"
	gitcb "github.com/AzraelSec/glock/pkg/git_command_builder"
)

type listRemotes struct {
	*gitcb.CommandBuilder
}

func newListRemotes(repo git.Repo) listRemotes {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("remote")
	return listRemotes{cb}
}
