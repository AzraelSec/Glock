package init_cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AzraelSec/glock/internal/git"
)

// cloneOrResume never mutates an existing destination. A matching worktree
// root with a checked-out commit and the configured origin is already cloned.
func cloneOrResume(g git.Git, ops git.CloneOps) (bool, error) {
	info, err := os.Stat(ops.Path)
	if errors.Is(err, os.ErrNotExist) {
		// A dangling symlink is an existing destination, not a free path.
		if _, linkErr := os.Lstat(ops.Path); linkErr == nil {
			return false, fmt.Errorf("destination %s is a dangling symlink", ops.Path)
		} else if !errors.Is(linkErr, os.ErrNotExist) {
			return false, linkErr
		}
		return false, g.Clone(ops)
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("destination %s exists and is not a directory", ops.Path)
	}
	gitOutput := func(args ...string) (string, error) {
		full := append([]string{"-C", ops.Path}, args...)
		cmd := exec.Command("git", full...)
		cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
		output, err := cmd.Output()
		return strings.TrimSuffix(string(output), "\n"), err
	}
	top, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return false, fmt.Errorf("destination %s exists but is not a usable Git checkout", ops.Path)
	}
	expected, err := filepath.EvalSymlinks(ops.Path)
	if err != nil {
		return false, err
	}
	actual, err := filepath.EvalSymlinks(top)
	if err != nil {
		return false, err
	}
	expected, err = filepath.Abs(expected)
	if err != nil {
		return false, err
	}
	actual, err = filepath.Abs(actual)
	if err != nil {
		return false, err
	}
	if expected != actual {
		return false, fmt.Errorf("destination %s is inside a different Git checkout (%s)", ops.Path, top)
	}
	origin, err := gitOutput("config", "--local", "--get", "remote.origin.url")
	if err != nil || origin != string(ops.Remote) {
		return false, fmt.Errorf("destination %s has no matching origin for %s", ops.Path, ops.Remote)
	}
	if _, err = gitOutput("rev-parse", "--verify", "HEAD^{commit}"); err != nil {
		return false, fmt.Errorf("destination %s has no checked-out commit; inspect the partial checkout", ops.Path)
	}
	// Git treats a missing index as empty, even when HEAD exists (for example,
	// after clone --no-checkout). Check the worktree-specific index explicitly;
	// then ask Git to read it without refreshing or writing it. Do not compare
	// files to HEAD: staged and unstaged user changes must remain untouched.
	indexPath, err := gitOutput("rev-parse", "--git-path", "index")
	if err != nil || indexPath == "" {
		return false, fmt.Errorf("destination %s: cannot locate checkout index; inspect the partial checkout", ops.Path)
	}
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(ops.Path, indexPath)
	}
	info, err = os.Stat(indexPath)
	if err != nil || !info.Mode().IsRegular() {
		return false, fmt.Errorf("destination %s has no usable checkout index; inspect the partial checkout", ops.Path)
	}
	check := exec.Command("git", "-C", ops.Path, "-c", "core.fsmonitor=false", "ls-files", "--stage", "-z")
	check.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	check.Stdout = io.Discard
	if err := check.Run(); err != nil {
		return false, fmt.Errorf("destination %s has an unreadable checkout index; inspect the partial checkout", ops.Path)
	}
	return true, nil
}
