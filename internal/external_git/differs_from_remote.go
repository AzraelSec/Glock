package external_git

import (
	"github.com/AzraelSec/glock/internal/git"
	gitcb "github.com/AzraelSec/glock/internal/git_command_builder"
)

type differsFromRemote struct {
	*gitcb.CommandBuilder
}

func newDiffersFromRemote(repo git.Repo, br git.BranchName) differsFromRemote {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("diff", "--exit-code", "--quiet").
		Arg(string(br), "@{upstream}")
	return differsFromRemote{cb}
}
