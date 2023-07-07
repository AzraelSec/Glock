package external_git

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/AzraelSec/glock/pkg/git"
	"github.com/AzraelSec/glock/pkg/git_command_builder"
	"github.com/AzraelSec/glock/pkg/utils"
)

type ExternalGit struct{}

var mergeConflictErr = errors.New("merge conflicts!")

// ensure to implement the Git interface
var _ git.Git = ExternalGit{}

func checkGitPresence() {
	_, err := exec.LookPath("git")
	if err != nil {
		panic("install git client before running glock")
	}
}

func NewExternalGit() ExternalGit {
	checkGitPresence()
	return ExternalGit{}
}

func (ExternalGit) Clone(ops git.CloneOps) error {
	if utils.DirExists(ops.Path) {
		return fmt.Errorf("repo already cloned at %s", ops.Path)
	}

	return gitcb.NewCommandBuilder().
		Arg("clone").
		Arg(string(ops.Remote)).
		Arg(ops.Path).
		Arg("--branch", string(ops.Refs)).
		Run()
}

func (ExternalGit) Fetch(repo git.Repo) error {
	return gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("fetch").
		Run()
}

func (ExternalGit) CurrentBranch(repo git.Repo) (git.BranchName, error) {
	if !utils.DirExists(repo.Path) {
		return "", errors.New("unable to locate repo")
	}

	br, err := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("rev-parse", "--abbrev-ref", "HEAD").
		RunWithOutput()
	return git.BranchName(br), err
}

func (g ExternalGit) Status(repo git.Repo) (git.StatusRes, error) {
	if !utils.DirExists(repo.Path) {
		return git.StatusRes{}, errors.New("unable to locate repo")
	}

	br, err := g.CurrentBranch(repo)
	if err != nil {
		return git.StatusRes{}, err
	}

	hasChanges, err := g.HasChanges(repo)
	if err != nil {
		return git.StatusRes{}, err
	}

	return git.StatusRes{
		Change: hasChanges,
		Branch: git.BranchName(br),
	}, nil
}

func (ExternalGit) HasChanges(repo git.Repo) (bool, error) {
	if !utils.DirExists(repo.Path) {
		return false, errors.New("unable to locate repo")
	}

	changes, err := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("status", "--porcelain").
		RunWithOutput()
	if err != nil {
		return false, err
	}

	return changes != "", nil
}

func (g ExternalGit) DiffersFromRemote(repo git.Repo) (bool, error) {
	if !utils.DirExists(repo.Path) {
		return false, errors.New("unable to locate repo")
	}

	br, err := g.CurrentBranch(repo)
	if err != nil {
		return false, err
	}

	ret := gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("diff", "--exit-code", "--quiet").
		Arg(string(br), fmt.Sprintf("origin/%s", string(br))).
		RunWithExitCode()
	if ret == -1 {
		return false, errors.New("returned with -1")
	}

	return ret == 1, nil
}

func (eg ExternalGit) Switch(repo git.Repo, branch git.BranchName, force bool) error {
	if !utils.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}

	return gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("switch", string(branch)).
		ArgIf(utils.Truthy(force), "-f").
		Run()
}

func (eg ExternalGit) Pull(repo git.Repo, rebase bool) error {
	if !utils.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}

	return gitcb.NewCommandBuilder().
		SetRepo(repo).
		Arg("pull").
		ArgIf(utils.Truthy(rebase), "--rebase").
		Run()
}
