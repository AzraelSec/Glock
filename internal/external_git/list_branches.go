package external_git

import (
	"github.com/AzraelSec/glock/internal/git"
	gitcb "github.com/AzraelSec/glock/internal/git_command_builder"
)

type listBranches struct {
	*gitcb.CommandBuilder
}

func newListBranches(repo git.Repo) listBranches {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("branch", "--format=%(refname:short)")
	return listBranches{cb}
}
