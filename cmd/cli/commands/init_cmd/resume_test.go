package init_cmd

import (
	"bytes"
	"fmt"
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AzraelSec/glock/internal/git"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	argv := append([]string{"-C", dir, "-c", "core.hooksPath=/dev/null", "-c", "user.name=Test", "-c", "user.email=test@example.com"}, args...)
	out, err := exec.Command("git", argv...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
func setupCheckout(t *testing.T, root string, commit bool, origin string) {
	t.Helper()
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "-q")
	if origin != "" {
		runGit(t, root, "remote", "add", "origin", origin)
	}
	if commit {
		runGit(t, root, "commit", "--allow-empty", "-qm", "initial")
	}
}
func TestCloneOrResumeExistingDestinations(t *testing.T) {
	const remote = "git@github.com:example/repo.git"
	g, err := git.NewGitCLI()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		commit bool
		origin string
		want   string
	}{
		{"matching", true, remote, ""}, {"wrong-origin", true, "git@github.com:other/repo.git", "matching origin"},
		{"missing-origin", true, "", "matching origin"}, {"partial", false, remote, "no checked-out commit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "repo")
			setupCheckout(t, root, tc.commit, tc.origin)
			marker := filepath.Join(root, "untracked")
			if err := os.WriteFile(marker, []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			resumed, err := cloneOrResume(g, git.CloneOps{Path: root, Remote: remote, Refs: "main"})
			if tc.want == "" {
				if err != nil || !resumed {
					t.Fatalf("%v %v", resumed, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q: %v", tc.want, err)
			}
			b, err := os.ReadFile(marker)
			if err != nil || string(b) != "untouched" {
				t.Fatal("changed existing checkout")
			}
		})
	}
}
func TestCloneOrResumeRejectsNestedCheckoutAndDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	setupCheckout(t, root, true, "git@github.com:example/repo.git")
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	g, err := git.NewGitCLI()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{nested, filepath.Join(root, "dangling")} {
		if strings.HasSuffix(path, "dangling") {
			if err := os.Symlink("missing", path); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := cloneOrResume(g, git.CloneOps{Path: path, Remote: "git@github.com:example/repo.git"}); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}

func TestRemoteFailureResumesWithLocalInit(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	root := t.TempDir()
	remotes := filepath.Join(root, "remotes")
	if err := os.Mkdir(remotes, 0700); err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(root, "seed")
	setupCheckout(t, seed, true, "")
	branchBytes, err := exec.Command("git", "-C", seed, "branch", "--show-current").Output()
	if err != nil {
		t.Fatal(err)
	}
	branch := strings.TrimSpace(string(branchBytes))
	makeRemote := func(name string) {
		t.Helper()
		cmd := exec.Command("git", "clone", "--bare", seed, filepath.Join(remotes, name+".git"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("make remote: %v: %s", err, out)
		}
	}
	makeRemote("a")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url.file://"+filepath.ToSlash(remotes)+"/.insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "git@github.com:example/")
	raw := fmt.Sprintf("version: 2\nsources:\n  a: {type: github, path: a, url: 'git@github.com:example/a.git', head: %s}\n  b: {type: github, path: b, url: 'git@github.com:example/b.git', head: %s}\n", branch, branch)
	http.DefaultTransport = configTransport{200, raw}
	target := filepath.Join(root, "project")
	cmd := remoteCommand(dependency.NewManager())
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"https://example.invalid/glock.yml", target})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "init local") {
		t.Fatalf("expected recovery guidance: %v", err)
	}
	marker := filepath.Join(target, "a", "my-work")
	if err := os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	makeRemote("b")
	t.Setenv(config.ConfigPathEnv, filepath.Join(target, "glock.yml"))
	local := localCommand(dependency.NewManager())
	local.SilenceErrors = true
	local.SilenceUsage = true
	local.SetOut(io.Discard)
	local.SetErr(io.Discard)
	if err := local.Execute(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "preserve" {
		t.Fatal("existing checkout was modified")
	}
	if _, err := os.Stat(filepath.Join(target, "b", ".git")); err != nil {
		t.Fatal("missing resumed clone", err)
	}
}

func TestInitLocalReportsUnpopulatedCheckoutAndResumesAfterRepair(t *testing.T) {
	const origin = "git@github.com:example/repo.git"
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	setupCheckout(t, seed, false, "")
	if err := os.WriteFile(filepath.Join(seed, "tracked"), []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, seed, "add", "tracked")
	runGit(t, seed, "commit", "-qm", "initial")
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(project, "repo")
	clone := exec.Command("git", "clone", "--no-checkout", seed, repo)
	if out, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v %s", err, out)
	}
	runGit(t, repo, "remote", "set-url", "origin", origin)
	configPath := filepath.Join(project, "glock.yml")
	raw := "version: 2\nsources: {repo: {type: github, path: repo, url: '" + origin + "', head: main}}\n"
	if err := os.WriteFile(configPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.ConfigPathEnv, configPath)
	marker := filepath.Join(repo, "user-work")
	if err := os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	runInit := func() error {
		cmd := localCommand(dependency.NewManager())
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		return cmd.Execute()
	}
	if err := runInit(); err == nil || !strings.Contains(err.Error(), "partial checkout") {
		t.Fatalf("partial checkout not reported: %v", err)
	}
	for _, path := range []string{filepath.Join(repo, ".git", "index"), filepath.Join(repo, "tracked")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("init modified %s: %v", path, err)
		}
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "preserve" {
		t.Fatalf("user work changed: %s %v", got, err)
	}
	// Simulate the user repairing the checkout and then making local changes.
	runGit(t, repo, "checkout", "-f", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "tracked")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("unstaged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	indexBefore, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	statusBefore, err := exec.Command("git", "-C", repo, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("repaired checkout not resumed: %v", err)
	}
	indexAfter, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	statusAfter, err := exec.Command("git", "-C", repo, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(indexBefore, indexAfter) || !bytes.Equal(statusBefore, statusAfter) {
		t.Fatal("init altered staged or unstaged work")
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "preserve" {
		t.Fatal("init modified untracked work")
	}
}

func TestCloneOrResumeRejectsCorruptIndexWithoutChangingIt(t *testing.T) {
	const origin = "git@github.com:example/repo.git"
	repo := filepath.Join(t.TempDir(), "repo")
	setupCheckout(t, repo, true, origin)
	index := filepath.Join(repo, ".git", "index")
	data := []byte("not a git index")
	if err := os.WriteFile(index, data, 0600); err != nil {
		t.Fatal(err)
	}
	g, err := git.NewGitCLI()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cloneOrResume(g, git.CloneOps{Path: repo, Remote: origin}); err == nil || !strings.Contains(err.Error(), "unreadable checkout index") {
		t.Fatalf("corrupt index: %v", err)
	}
	got, err := os.ReadFile(index)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("corrupt index was modified")
	}
}

func TestCloneOrResumeLinkedWorktreeAndSymlink(t *testing.T) {
	const origin = "git@github.com:example/repo.git"
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	setupCheckout(t, repo, true, origin)
	worktree := filepath.Join(root, "worktree")
	runGit(t, repo, "worktree", "add", "--detach", worktree, "HEAD")
	link := filepath.Join(root, "linked")
	if err := os.Symlink(worktree, link); err != nil {
		t.Fatal(err)
	}
	g, err := git.NewGitCLI()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{worktree, link} {
		resumed, err := cloneOrResume(g, git.CloneOps{Path: path, Remote: origin})
		if err != nil || !resumed {
			t.Fatalf("%s: resumed=%v err=%v", path, resumed, err)
		}
	}
}

func TestCloneOrResumeCheckoutAndSymlinkEndingInWhitespace(t *testing.T) {
	const origin = "git@github.com:example/repo.git"
	root := t.TempDir()
	repo := filepath.Join(root, "repo \t")
	setupCheckout(t, repo, true, origin)
	link := filepath.Join(root, "link \t")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(repo, "user-work")
	if err := os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := git.NewGitCLI()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{repo, link} {
		resumed, err := cloneOrResume(g, git.CloneOps{Path: path, Remote: origin})
		if err != nil || !resumed {
			t.Fatalf("%q: resumed=%v err=%v", path, resumed, err)
		}
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "preserve" {
		t.Fatal("existing work changed")
	}
}
