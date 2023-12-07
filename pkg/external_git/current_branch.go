package external_git

import (
	"github.com/AzraelSec/glock/pkg/git"
	gitcb "github.com/AzraelSec/glock/pkg/git_command_builder"
)

type currentBranch struct {
	*gitcb.CommandBuilder
}

func newCurrentBranch(repo git.Repo) currentBranch {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("rev-parse", "--abbrev-ref", "HEAD")
	return currentBranch{cb}
}
