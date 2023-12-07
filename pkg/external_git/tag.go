package external_git

import (
	"github.com/AzraelSec/glock/pkg/git"
	gitcb "github.com/AzraelSec/glock/pkg/git_command_builder"
)

type createLightWeightTag struct {
	*gitcb.CommandBuilder
}

func newCreateLightWeightTag(repo git.Repo, tag string, branch git.BranchName) createLightWeightTag {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("tag", tag, string(branch))
	return createLightWeightTag{cb}
}

type pushTag struct {
	*gitcb.CommandBuilder
}

func newPushTag(repo git.Repo, tag git.Tag, remote git.Remote) pushTag {
	cb := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("push", string(remote), string(tag))
	return pushTag{cb}
}
