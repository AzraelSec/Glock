package external_git

import (
	"github.com/AzraelSec/glock/pkg/git"
	gitcb "github.com/AzraelSec/glock/pkg/git_command_builder"
)

type pull struct {
	*gitcb.CommandBuilder
}

func newPull(repo git.Repo, rebase bool) pull {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("pull").
		ArgIf(rebase, "--rebase")
	return pull{cb}
}

type pullBranch struct {
	*gitcb.CommandBuilder
}

func newPullBranch(repo git.Repo, br git.BranchName, remote git.Remote, rebase bool) pullBranch {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("pull", string(remote), string(br)).
		ArgIf(rebase, "--rebase")
	return pullBranch{cb}
}
