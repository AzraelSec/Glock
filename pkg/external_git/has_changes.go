package external_git

import (
	"github.com/AzraelSec/glock/pkg/git"
	gitcb "github.com/AzraelSec/glock/pkg/git_command_builder"
)

type hasChanges struct {
	*gitcb.CommandBuilder
}

func newHasChanges(repo git.Repo) hasChanges {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("status", "--porcelain")
	return hasChanges{cb}
}
