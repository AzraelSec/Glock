package external_git

import (
	"github.com/AzraelSec/glock/internal/git"
	gitcb "github.com/AzraelSec/glock/internal/git_command_builder"
)

type switchCb struct {
	*gitcb.CommandBuilder
}

func newSwitch(repo git.Repo, br git.BranchName, force bool) switchCb {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("switch", string(br)).
		ArgIf(force, "-f")
	return switchCb{cb}
}
