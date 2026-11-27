package source

import (
	"errors"
	"testing"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/git"
)

// mockGit records the Pull invocations and returns a configurable error,
// satisfying the git.Git interface.
type mockGit struct {
	pulls   []git.Repo
	rebase  []bool
	pullErr error
}

func (m *mockGit) Clone(ops git.CloneOps) error {
	return m.pullErr
}

func (m *mockGit) ListRemotes(repo git.Repo) ([]git.Remote, error) {
	return nil, nil
}

func (m *mockGit) Fetch(repo git.Repo) error { return m.pullErr }

func (m *mockGit) Status(repo git.Repo) (git.StatusRes, error) {
	return git.StatusRes{}, nil
}

func (m *mockGit) CurrentBranch(repo git.Repo) (git.BranchName, error) {
	return "", nil
}

func (m *mockGit) HasChanges(repo git.Repo) (bool, error) {
	return false, nil
}

func (m *mockGit) DiffersFromRemote(repo git.Repo) (bool, error) {
	return false, nil
}

func (m *mockGit) Switch(repo git.Repo, branch git.BranchName, force bool) error {
	return nil
}

func (m *mockGit) SwitchToConfiguredRef(repo git.Repo) (bool, error) {
	return false, nil
}

func (m *mockGit) Pull(repo git.Repo, rebase bool) error {
	m.pulls = append(m.pulls, repo)
	m.rebase = append(m.rebase, rebase)
	return m.pullErr
}

func (m *mockGit) PullBranch(repo git.Repo, branch git.BranchName, rebase bool) error {
	return nil
}

func (m *mockGit) ListBranches(repo git.Repo) ([]git.BranchName, error) {
	return nil, nil
}

func (m *mockGit) CreateLightweightTag(repo git.Repo, tag string, branch git.BranchName) error {
	return nil
}

func (m *mockGit) PushTag(repo git.Repo, tag git.Tag, remote git.Remote) error {
	return nil
}

func githubConfig() config.Source {
	return config.Source{
		Type: "github",
		Path: new("src/api"),
		URL:  new("git@github.com:example/api.git"),
		Head: new("main"),
	}
}

func TestGitHubSource_Pull(t *testing.T) {
	g := &mockGit{}
	src, err := NewSource(githubConfig(), "/project", g)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}

	if err := src.Pull(); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(g.pulls) != 1 {
		t.Fatalf("expected 1 pull, got %d", len(g.pulls))
	}
	got := g.pulls[0]
	want := git.Repo{
		Remote: git.RemoteGitUrl("git@github.com:example/api.git"),
		Path:   "/project/src/api",
		Refs:   git.BranchName("main"),
	}
	if got != want {
		t.Errorf("pull repo = %+v, want %+v", got, want)
	}
	if g.rebase[0] {
		t.Error("expected plain pull without rebase")
	}
}

func TestGitHubSource_PullPropagatesError(t *testing.T) {
	g := &mockGit{pullErr: errors.New("unable to locate repo")}
	src, err := NewSource(githubConfig(), "/project", g)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}

	err = src.Pull()
	if err == nil {
		t.Fatal("expected Pull to return the git error")
	}
	if err.Error() != "unable to locate repo" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGitHubSourceRequiresPath(t *testing.T) {
	cfg := githubConfig()
	cfg.Path = nil
	if _, err := NewSource(cfg, "/project", &mockGit{}); err == nil {
		t.Fatal("missing path accepted")
	}
}

func TestS3Source_PullNotImplemented(t *testing.T) {
	cfg := config.Source{
		Type: "s3",
		Path: new("data"),
		URL:  new("https://bucket.s3.amazonaws.com/file"),
	}
	src, err := NewSource(cfg, "/project", &mockGit{})
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}

	if err := src.Pull(); err == nil {
		t.Fatal("expected s3 Pull to report not implemented")
	}
}

func TestNewSource_UnsupportedType(t *testing.T) {
	for _, typ := range []config.SourceType{"http", "unknown"} {
		cfg := config.Source{Type: typ, Path: new("src"), URL: new("https://example.com")}
		if _, err := NewSource(cfg, "/project", &mockGit{}); err == nil {
			t.Errorf("expected error for unsupported type %q", typ)
		}
	}
}

func TestNewSource_MissingRequiredFields(t *testing.T) {
	if _, err := NewSource(config.Source{Type: "github"}, "/project", &mockGit{}); err == nil {
		t.Error("expected error for github without url/head")
	}
	if _, err := NewSource(config.Source{Type: "s3"}, "/project", &mockGit{}); err == nil {
		t.Error("expected error for s3 without url")
	}
}

func TestFilesystemSourceUsesExistingDirectory(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{root, "."} {
		src, err := NewSource(config.Source{Type: config.SourceTypeFilesystem, Path: &path}, root, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = src.Pull(); err != nil {
			t.Fatal(err)
		}
	}
	path := "missing"
	src, err := NewSource(config.Source{Type: config.SourceTypeFilesystem, Path: &path}, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = src.Pull(); err == nil {
		t.Fatal("missing directory accepted")
	}
}
