package commands

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
)

func runResetGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	options := []string{
		"-C", dir,
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "user.name=Test",
		"-c", "user.email=test@example.com",
		"-c", "commit.gpgSign=false",
		"-c", "tag.gpgSign=false",
	}
	out, err := exec.Command("git", append(options, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func resetCheckout(t *testing.T, root, name string) string {
	t.Helper()
	repo := filepath.Join(root, name)
	runResetGit(t, root, "init", "-q", "-b", "main", repo)
	runResetGit(t, repo, "commit", "--allow-empty", "-qm", "initial")
	return repo
}

func setResetConfig(t *testing.T, root, sources string) {
	t.Helper()
	path := filepath.Join(root, "glock.yml")
	if err := os.WriteFile(path, []byte("sources:\n"+sources), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.ConfigPathEnv, path)
}

func runReset(t *testing.T, args []string) {
	t.Helper()
	cmd := resetFactory(dependency.NewManager())
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestResetChecksOutTagWithoutPull(t *testing.T) {
	root := t.TempDir()
	repo := resetCheckout(t, root, "repo")
	taggedCommit := runResetGit(t, repo, "rev-parse", "HEAD")
	runResetGit(t, repo, "tag", "-am", "release", "release/v1.0.0")
	runResetGit(t, repo, "commit", "--allow-empty", "-qm", "after release")
	setResetConfig(t, root, `  pinned: {type: github, url: 'https://example.com/org/repo.git', head: release/v1.0.0, path: repo}
`)

	// There is no upstream. Both a branch checkout and a repeat reset must succeed.
	for _, args := range [][]string{{}, {"--skip-pull"}, {}} {
		runReset(t, args)
		if branch := runResetGit(t, repo, "branch", "--show-current"); branch != "" {
			t.Fatalf("expected detached HEAD, on branch %q", branch)
		}
		if head := runResetGit(t, repo, "rev-parse", "HEAD"); head != taggedCommit {
			t.Fatalf("expected tagged commit %s, got %s", taggedCommit, head)
		}
	}
}

func TestResetBranchPullAndSkipPull(t *testing.T) {
	root := t.TempDir()
	origin := resetCheckout(t, root, "origin")
	initial := runResetGit(t, origin, "rev-parse", "HEAD")
	runResetGit(t, origin, "tag", "main")
	repo := filepath.Join(root, "repo")
	runResetGit(t, root, "clone", "-q", "--branch", "main", origin, repo)
	runResetGit(t, origin, "commit", "--allow-empty", "-qm", "new upstream commit")
	upstream := runResetGit(t, origin, "rev-parse", "HEAD")
	setResetConfig(t, root, `  branch: {type: github, url: 'https://example.com/org/repo.git', head: main, path: repo}
`)

	for _, tc := range []struct {
		name string
		args []string
		head string
	}{
		{"skip pull", []string{"--skip-pull"}, initial},
		{"pull", []string{}, upstream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runResetGit(t, repo, "switch", "--detach", initial)
			runReset(t, tc.args)
			if branch := runResetGit(t, repo, "branch", "--show-current"); branch != "main" {
				t.Fatalf("expected main branch to take precedence over the tag, got %q", branch)
			}
			if head := runResetGit(t, repo, "rev-parse", "HEAD"); head != tc.head {
				t.Fatalf("expected commit %s, got %s", tc.head, head)
			}
		})
	}
}

func TestResetSerializesSourcesSharingCheckout(t *testing.T) {
	root := t.TempDir()
	repo := resetCheckout(t, root, "repo")
	runResetGit(t, repo, "tag", "earlier")
	runResetGit(t, repo, "commit", "--allow-empty", "-qm", "next release")
	runResetGit(t, repo, "tag", "later")
	later := runResetGit(t, repo, "rev-parse", "HEAD")
	runResetGit(t, repo, "commit", "--allow-empty", "-qm", "after both releases")
	if err := os.Symlink(repo, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	setResetConfig(t, root, `  second: {type: github, url: 'https://example.com/org/repo.git', head: later, path: alias}
  first: {type: github, url: 'https://example.com/org/repo.git', head: earlier, path: repo}
`)
	runReset(t, []string{})
	if head := runResetGit(t, repo, "rev-parse", "HEAD"); head != later {
		t.Fatalf("expected final commit %s from source second, got %s", later, head)
	}
}

func TestResetExcludesSourcesBeforeCheckingWorkingTrees(t *testing.T) {
	root := t.TempDir()
	dirty := resetCheckout(t, root, "local-dirty")
	omitted := resetCheckout(t, root, "local-omitted")
	clean := resetCheckout(t, root, "local-clean")
	for _, repo := range []string{dirty, omitted, clean} {
		runResetGit(t, repo, "switch", "-qc", "feature")
	}
	if err := os.WriteFile(filepath.Join(dirty, "work.txt"), []byte("in progress"), 0600); err != nil {
		t.Fatal(err)
	}
	setResetConfig(t, root, `  api: {type: github, url: 'https://example.com/org/api.git', head: main, path: local-dirty}
  docs: {type: github, url: 'https://example.com/org/docs.git', head: main, path: local-omitted}
  web: {type: github, url: 'https://example.com/org/web.git', head: main, path: local-clean}
`)

	for _, tc := range []struct {
		args    []string
		wantErr string
	}{
		{[]string{"--skip-pull"}, "not clean"},
		{[]string{"--skip-pull", "--exclude", "api,typo"}, `unknown Git source "typo" in --exclude`},
	} {
		cmd := resetFactory(dependency.NewManager())
		cmd.SetArgs(tc.args)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("reset %v: expected %q, got %v", tc.args, tc.wantErr, err)
		}
	}

	runReset(t, []string{"--exclude", "api,docs,web"})
	if branch := runResetGit(t, clean, "branch", "--show-current"); branch != "feature" {
		t.Fatalf("validation or excluding every source switched web to %q", branch)
	}

	runReset(t, []string{"--skip-pull", "--exclude", "api", "--exclude", "docs"})
	for _, repo := range []string{dirty, omitted} {
		if branch := runResetGit(t, repo, "branch", "--show-current"); branch != "feature" {
			t.Errorf("excluded checkout %s switched to %q", repo, branch)
		}
	}
	if branch := runResetGit(t, clean, "branch", "--show-current"); branch != "main" {
		t.Errorf("clean checkout stayed on %q", branch)
	}
}

func TestResetSkipDirtySkipsSwitchAndPull(t *testing.T) {
	root := t.TempDir()
	origin := resetCheckout(t, root, "origin")
	initial := runResetGit(t, origin, "rev-parse", "HEAD")
	for _, name := range []string{"api", "worker", "web"} {
		runResetGit(t, root, "clone", "-q", origin, name)
	}
	runResetGit(t, origin, "commit", "--allow-empty", "-qm", "upstream change")
	upstream := runResetGit(t, origin, "rev-parse", "HEAD")
	runResetGit(t, filepath.Join(root, "worker"), "switch", "-qc", "feature")
	for _, name := range []string{"api", "worker"} {
		dir := filepath.Join(root, name)
		if err := os.WriteFile(filepath.Join(dir, "work.txt"), []byte("in progress"), 0600); err != nil {
			t.Fatal(err)
		}
		runResetGit(t, dir, "add", "work.txt")
	}
	setResetConfig(t, root, `  api: {type: github, url: 'https://example.com/org/api.git', head: main, path: api}
  docs: {type: github, url: 'https://example.com/org/docs.git', head: main, path: missing-docs}
  worker: {type: github, url: 'https://example.com/org/worker.git', head: main, path: worker}
  web: {type: github, url: 'https://example.com/org/web.git', head: main, path: web}
`)

	var out bytes.Buffer
	cmd := resetFactory(dependency.NewManager())
	cmd.SetArgs([]string{"--exclude", "docs", "--skip-dirty"})
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if head := runResetGit(t, filepath.Join(root, "api"), "rev-parse", "HEAD"); head != initial {
		t.Errorf("dirty checkout on main pulled to %s", head)
	}
	if branch := runResetGit(t, filepath.Join(root, "worker"), "branch", "--show-current"); branch != "feature" {
		t.Errorf("dirty checkout switched to %q", branch)
	}
	if head := runResetGit(t, filepath.Join(root, "web"), "rev-parse", "HEAD"); head != upstream {
		t.Errorf("clean checkout did not pull; HEAD is %s", head)
	}
	for _, name := range []string{"api", "worker"} {
		if !strings.Contains(out.String(), "["+name+"] Skipped") {
			t.Errorf("missing skip reason for %s in %q", name, out.String())
		}
		if state := runResetGit(t, filepath.Join(root, name), "status", "--porcelain"); state != "A  work.txt" {
			t.Errorf("staged change for %s was altered: %q", name, state)
		}
	}
}

func TestResetCLIReturnsNonzero(t *testing.T) {
	const child = "GLOCK_TEST_RESET_SUBPROCESS"
	if os.Getenv(child) == "1" {
		os.Args = []string{os.Args[0], "reset"}
		ExecuteRoot()
		return
	}

	root := t.TempDir()
	resetCheckout(t, root, "missing-ref")
	resetCheckout(t, root, "no-upstream")
	good := resetCheckout(t, root, "good")
	runResetGit(t, good, "tag", "release")
	setResetConfig(t, root, `  missing-ref: {type: github, url: 'https://example.com/org/repo.git', head: unknown, path: missing-ref}
  no-upstream: {type: github, url: 'https://example.com/org/repo.git', head: main, path: no-upstream}
  working-tag: {type: github, url: 'https://example.com/org/repo.git', head: release, path: good}
`)

	cmd := exec.Command(os.Args[0], "-test.run=^TestResetCLIReturnsNonzero$")
	cmd.Env = append(cmd.Environ(), child+"=1")
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit status 1, got %v\n%s", err, out)
	}
	for _, want := range []string{`missing-ref: switch to "unknown"`, `no-upstream: pull branch "main"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
	if branch := runResetGit(t, good, "branch", "--show-current"); branch != "" {
		t.Fatalf("repo with a valid tag was not reset, on branch %q", branch)
	}
}
