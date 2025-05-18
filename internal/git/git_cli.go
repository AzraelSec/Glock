package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/AzraelSec/glock/internal/dir"
)

type GitCLI struct {
	builder func() *CommandBuilder
}

// ensure to implement the Git interface
var _ Git = GitCLI{}

func NewGitCLI() (GitCLI, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return GitCLI{}, errors.New("install git client before running glock")
	}
	return GitCLI{NewCommandBuilder}, nil
}

func (gc GitCLI) Clone(ops CloneOps) error {
	if dir.DirExists(ops.Path) {
		return fmt.Errorf("repo already cloned at %s", ops.Path)
	}
	return gc.builder().
		Arg("clone").
		Arg(string(ops.Remote)).
		Arg(ops.Path).
		Arg("--branch", string(ops.Refs)).
		Run()
}

func (gc GitCLI) ListRemotes(repo Repo) ([]Remote, error) {
	remotes := make([]Remote, 0)
	out, err := gc.builder().
		SetRepo(repo).
		Arg("remote").
		RunWithOutput()
	if err != nil {
		return remotes, err
	}

	for _, remote := range strings.Split(out, "\n") {
		remotes = append(remotes, Remote(remote))
	}
	return remotes, nil
}

func (gc GitCLI) Fetch(repo Repo) error {
	return gc.builder().
		SetRepo(repo).
		Arg("fetch").
		Run()
}

func (gc GitCLI) CurrentBranch(repo Repo) (BranchName, error) {
	if !dir.DirExists(repo.Path) {
		return "", errors.New("unable to locate repo")
	}

	br, err := gc.builder().
		SetRepo(repo).
		Arg("rev-parse", "--abbrev-ref", "HEAD").
		RunWithOutput()
	return BranchName(br), err
}

func (gc GitCLI) Status(repo Repo) (StatusRes, error) {
	if !dir.DirExists(repo.Path) {
		return StatusRes{}, errors.New("unable to locate repo")
	}

	br, err := gc.CurrentBranch(repo)
	if err != nil {
		return StatusRes{}, err
	}

	hasChanges, err := gc.HasChanges(repo)
	if err != nil {
		return StatusRes{}, err
	}

	return StatusRes{
		Change: hasChanges,
		Branch: BranchName(br),
	}, nil
}

func (gc GitCLI) HasChanges(repo Repo) (bool, error) {
	if !dir.DirExists(repo.Path) {
		return false, errors.New("unable to locate repo")
	}

	changes, err := gc.builder().
		SetRepo(repo).
		Arg("status", "--porcelain").
		RunWithOutput()
	if err != nil {
		return false, err
	}

	return changes != "", nil
}

func (gc GitCLI) DiffersFromRemote(repo Repo) (bool, error) {
	if !dir.DirExists(repo.Path) {
		return false, errors.New("unable to locate repo")
	}

	br, err := gc.CurrentBranch(repo)
	if err != nil {
		return false, err
	}

	ret := gc.builder().
		SetRepo(repo).
		Arg("diff", "--exit-code", "--quiet").
		Arg(string(br), "@{upstream}").
		RunWithExitCode()
	if ret == -1 {
		return false, errors.New("returned with -1")
	}

	return ret == 1, nil
}

func (gc GitCLI) Switch(repo Repo, br BranchName, force bool) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}

	err := gc.builder().
		SetRepo(repo).
		Arg("switch", string(br)).
		ArgIf(force, "-f").
		Run()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if ok && strings.Contains(string(ee.Stderr), "invalid reference") {
			err = ErrInvalidReference
		}
	}

	return err
}

func (gc GitCLI) Pull(repo Repo, rebase bool) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}
	return gc.builder().
		SetRepo(repo).
		Arg("pull").
		ArgIf(rebase, "--rebase").
		Run()
}

func (gc GitCLI) PullBranch(repo Repo, branch BranchName, rebase bool) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}

	return gc.builder().
		SetRepo(repo).
		Arg("pull", "origin", string(branch)).
		ArgIf(rebase, "--rebase").
		Run()
}

func (gc GitCLI) ListBranches(repo Repo) ([]BranchName, error) {
	brs := make([]BranchName, 0)
	if !dir.DirExists(repo.Path) {
		return brs, errors.New("unable to locate repo")
	}

	out, err := gc.builder().
		SetRepo(repo).
		Arg("branch", "--format=%(refname:short)").
		RunWithOutput()
	if err != nil {
		return brs, err
	}

	for _, br := range strings.Split(out, "\n") {
		brs = append(brs, BranchName(br))
	}
	return brs, nil
}

func (gc GitCLI) CreateLightweightTag(repo Repo, tag string, branch BranchName) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}
	return gc.builder().
		SetRepo(repo).
		Arg("tag", tag, string(branch)).
		Run()
}

func (gc GitCLI) PushTag(repo Repo, tag Tag, remote Remote) error {
	if !dir.DirExists(repo.Path) {
		return errors.New("unable to locate repo")
	}
	return gc.builder().
		SetRepo(repo).
		Arg("push", string(remote), string(tag)).
		Run()
}
