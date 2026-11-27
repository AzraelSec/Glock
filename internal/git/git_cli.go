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

	for remote := range strings.SplitSeq(out, "\n") {
		if remote != "" {
			remotes = append(remotes, Remote(remote))
		}
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

	ret := gc.builder().
		SetRepo(repo).
		Arg("diff", "--exit-code", "--quiet", "HEAD", "@{upstream}", "--").
		RunWithExitCode()
	if ret != 0 && ret != 1 {
		return false, fmt.Errorf("could not compare with upstream (git exit code %d)", ret)
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

func (gc GitCLI) SwitchToConfiguredRef(repo Repo) (bool, error) {
	branch, err := gc.CurrentBranch(repo)
	if err != nil {
		return false, err
	}
	if branch == repo.Refs && branch != "HEAD" {
		return false, nil
	}

	tagRef := "refs/tags/" + string(repo.Refs)
	hasTag, err := gc.hasRef(repo, tagRef)
	if err != nil {
		return false, err
	}
	if hasTag {
		// A local branch takes precedence, matching git switch and git clone --branch.
		hasBranch, err := gc.hasRef(repo, "refs/heads/"+string(repo.Refs))
		if err != nil {
			return false, err
		}
		if !hasBranch {
			err := gc.builder().SetRepo(repo).Arg("switch", "--detach", "--", tagRef).Run()
			return err == nil, err
		}
	}

	return false, gc.Switch(repo, repo.Refs, false)
}

func (gc GitCLI) hasRef(repo Repo, ref string) (bool, error) {
	err := gc.builder().SetRepo(repo).Arg("show-ref", "--verify", "--quiet", "--", ref).Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return err == nil, err
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
		Arg("for-each-ref", "--format=%(refname:lstrip=2)", "refs/heads/").
		RunWithOutput()
	if err != nil {
		return brs, err
	}

	for br := range strings.SplitSeq(out, "\n") {
		if br != "" {
			brs = append(brs, BranchName(br))
		}
	}
	return brs, nil
}

// HasRemoteBranch checks the configured remotes' local tracking refs.
func (gc GitCLI) HasRemoteBranch(repo Repo, branch BranchName) (bool, error) {
	remotes, err := gc.ListRemotes(repo)
	if err != nil || len(remotes) == 0 {
		return false, err
	}
	refs := make(map[string]bool, len(remotes))
	builder := gc.builder().SetRepo(repo).Arg("for-each-ref", "--format=%(refname)")
	for _, remote := range remotes {
		ref := "refs/remotes/" + string(remote) + "/" + string(branch)
		refs[ref] = true
		builder.Arg(ref)
	}
	out, err := builder.RunWithOutput()
	if err != nil {
		return false, err
	}
	for ref := range strings.SplitSeq(out, "\n") {
		if refs[ref] {
			return true, nil
		}
	}
	return false, nil
}

// CheckTag validates the local parts of a tag plan without changing the repository.
func (gc GitCLI) CheckTag(repo Repo, tag string, ref BranchName) error {
	if strings.HasPrefix(tag, "-") {
		return fmt.Errorf("invalid tag name %q", tag)
	}
	tagRef := "refs/tags/" + tag
	if err := gc.builder().SetRepo(repo).Arg("check-ref-format", tagRef).Run(); err != nil {
		return fmt.Errorf("invalid tag name %q", tag)
	}
	exists, err := gc.hasRef(repo, tagRef)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("tag %q already exists", tag)
	}
	err = gc.builder().SetRepo(repo).Arg("rev-parse", "--verify", "--end-of-options", string(ref)+"^{commit}").Run()
	if err != nil {
		return fmt.Errorf("tag target %q is unavailable: %w", ref, err)
	}
	return nil
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
