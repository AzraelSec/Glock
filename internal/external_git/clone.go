package external_git

import (
	"github.com/AzraelSec/glock/internal/git"
	gitcb "github.com/AzraelSec/glock/internal/git_command_builder"
)

type clone struct {
	*gitcb.CommandBuilder
}

func newClone(ops git.CloneOps) clone {
	cb := gitcb.NewCommandBuilder().
		Arg("clone").
		Arg(string(ops.Remote)).
		Arg(ops.Path).
		Arg("--branch", string(ops.Refs))

	return clone{cb}
}
