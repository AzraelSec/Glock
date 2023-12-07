package external_git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
)

type ExternalGit struct{}

// ensure to implement the Git interface
var _ git.Git = ExternalGit{}

func NewExternalGit() (ExternalGit, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return ExternalGit{}, errors.New("install git client before running glock")
	}
	// fixme: change the methods to use a pointer to ExternalGit
	return ExternalGit{}, nil
}

func (ExternalGit) Clone(ops git.CloneOps) error {
	if dir.DirExists(ops.Path) {
		return fmt.Errorf("repo already cloned at %s", ops.Path)
	}
	return newClone(ops).Run()
}

func (ExternalGit) ListRemotes(repo git.Repo) ([]git.Remote, error) {
	remotes := make([]git.Remote, 0)
	out, err := newListRemotes(repo).RunWithOutput()
	if err != nil {
		return remotes, err
	}

	for _, remote := range strings.Split(out, "\n") {
		remotes = append(remotes, git.Remote(remote))
	}
	return remotes, nil
}

func (ExternalGit) Fetch(repo git.Repo) error {
	return newFetch(repo).Run()
}

func (ExternalGit) CurrentBranch(repo git.Repo) (git.BranchName, error) {
	if !dir.DirExists(repo.Path) {
		return "", errors.New("unable to locate repo")
	}

	br, err := newCurrentBranch(repo).RunWithOutput()
	return git.BranchName(br), err
}

func (g ExternalGit) Status(repo git.Repo) (git.StatusRes, error) {
	if !dir.DirExists(repo.Path) {
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
	if !dir.DirExists(repo.Path) {
		return false, errors.New("unable to locate repo")
	}

	changes, err := newHasChanges(repo).RunWithOutput()
	if err != nil {
		return false, err
	}

	return changes != "", nil
}

func (g ExternalGit) DiffersFromRemote(repo git.Repo) (bool, error) {
	if !dir.DirExists(repo.Path) {
		return false, errors.New("unable to locate repo")
	}

	br, err := g.CurrentBranch(repo)
	if err != nil {
		return false, err
	}

	ret := newDiffersFromRemote(repo, br).RunWithExitCode()
	if ret == -1 {
		return false, errors.New("returned with -1")
	}

	return ret == 1, nil
}

func (eg ExternalGit) Switch(repo git.Repo, br git.BranchName, force bool) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}

	err := newSwitch(repo, br, force).Run()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if ok && strings.Contains(string(ee.Stderr), "invalid reference") {
			err = git.InvalidReferenceErr
		}
	}

	return err
}

func (eg ExternalGit) Pull(repo git.Repo, rebase bool) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}
	return newPull(repo, rebase).Run()
}

func (eg ExternalGit) PullBranch(repo git.Repo, branch git.BranchName, rebase bool) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}

	return newPullBranch(repo, branch, "origin", rebase).Run()
}

func (eg ExternalGit) ListBranches(repo git.Repo) ([]git.BranchName, error) {
	brs := make([]git.BranchName, 0)
	if !dir.DirExists(repo.Path) {
		return brs, errors.New("unable to locate repo")
	}

	out, err := newListBranches(repo).RunWithOutput()
	if err != nil {
		return brs, err
	}

	for _, br := range strings.Split(out, "\n") {
		brs = append(brs, git.BranchName(br))
	}
	return brs, nil
}

func (eg ExternalGit) CreateLightweightTag(repo git.Repo, tag string, branch git.BranchName) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}
	return newCreateLightWeightTag(repo, tag, branch).Run()
}

func (eg ExternalGit) PushTag(repo git.Repo, tag git.Tag, remote git.Remote) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}
	return newPushTag(repo, tag, remote).Run()
}
