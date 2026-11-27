package tag

import (
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/charmbracelet/lipgloss"
)

func TestPreviewTagChecksPlanWithoutCreatingTag(t *testing.T) {
	path := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return string(out)
	}
	run("init", "-b", "main")
	run("-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-m", "Initial")
	run("remote", "add", "origin", "../repo-that-does-not-exist")
	g, err := git.NewGitCLI()
	if err != nil {
		t.Fatal(err)
	}

	input := tagInputPayload{gr: git.Repo{Path: path, Refs: "main"}, pattern: "release-{{.Branch}}", pull: true, push: true}
	plan, err := previewTag(g, input)
	if err != nil || plan.tag != "release-main" || plan.branch != "main" || plan.remote != "origin" {
		t.Fatalf("unexpected preview: %+v, %v", plan, err)
	}
	if tags := run("tag"); tags != "" {
		t.Fatalf("preview created a tag: %s", tags)
	}

	run("tag", plan.tag)
	if _, err := previewTag(g, input); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("preview should detect an existing tag, got %v", err)
	}
}

func TestTagPreviewShowsSharedSettingsOnce(t *testing.T) {
	renderer := lipgloss.NewRenderer(io.Discard)
	repos := []config.LiveRepo{{Name: "api"}, {Name: "web"}, {Name: "worker"}, {Name: "billing"}}
	results := []runner.Result[tagOutputPayload]{
		{Res: tagOutputPayload{tag: "release-2026.11.27", branch: "main", remote: "origin"}},
		{Res: tagOutputPayload{tag: "release-2026.11.27", branch: "main", remote: "origin"}},
		{Res: tagOutputPayload{tag: "release-2026.11.27", branch: "feature/release-review", remote: "origin"}},
		{Error: config.ErrRepoNotFound},
	}

	const width = 38
	got := formatTagPreview(renderer, width, repos, results, true, false, true)
	want := `Tag preview
Tag       release-2026.11.27
Source    current branches
Push      origin
Pre-pull  origin

Eligible · 3
✓ api     main
✓ web     main
✓ worker  feature/release-review

Unavailable · 1
× billing  repo not found`
	if got != want {
		t.Fatalf("unexpected preview:\n%s", got)
	}
}

func TestTagPreviewKeepsUnpaddedBranchOnOneLineAtWidthLimit(t *testing.T) {
	renderer := lipgloss.NewRenderer(io.Discard)
	repos := []config.LiveRepo{{Name: "x"}, {Name: "worker"}, {Name: "界"}}
	longBranch := "feature/" + strings.Repeat("x", 25)
	results := []runner.Result[tagOutputPayload]{
		{Res: tagOutputPayload{tag: "release", branch: longBranch}},
		{Res: tagOutputPayload{tag: "release", branch: "main"}},
		{Res: tagOutputPayload{tag: "release", branch: "stable"}},
	}

	const width = 38
	got := formatTagPreview(renderer, width, repos, results, true, true, false)
	if !strings.Contains(got, "✓ x  "+longBranch+"\n✓ worker  main\n✓ 界      stable") {
		t.Fatalf("expected fitting branch rows and display-width alignment:\n%s", got)
	}
}

func TestTagPreviewKeepsRepositorySpecificTagsAndDestinations(t *testing.T) {
	renderer := lipgloss.NewRenderer(io.Discard)
	repos := []config.LiveRepo{{Name: "api"}, {Name: "web"}, {Name: "docs"}}
	results := []runner.Result[tagOutputPayload]{
		{Res: tagOutputPayload{tag: "api-release", branch: "main", remote: "origin"}},
		{Res: tagOutputPayload{tag: "web-release", branch: "main", remote: "upstream"}},
		{Res: tagOutputPayload{tag: "docs-release", branch: "main"}, Error: errors.New("tag already exists")},
	}

	got := formatTagPreview(renderer, 0, repos, results, false, false, false)
	want := `Tag preview
Tag       per repository
Source    main
Push      per repository
Pre-pull  off

Eligible · 2
✓ api  tag api-release · → origin
✓ web  tag web-release · → upstream

Unavailable · 1
× docs  tag docs-release · tag already exists`
	if got != want {
		t.Fatalf("unexpected preview:\n%s", got)
	}
}

func TestTagPreviewWrapsLongErrorsAtTerminalWidth(t *testing.T) {
	renderer := lipgloss.NewRenderer(io.Discard)
	repos := []config.LiveRepo{{Name: "api"}, {Name: "web"}}
	results := []runner.Result[tagOutputPayload]{
		{Res: tagOutputPayload{tag: "release", branch: "main"}},
		{Error: errors.New("target check failed: long/路径/feature/do-not-truncate-this-ref")},
	}
	const width = 25
	got := formatTagPreview(renderer, width, repos, results, false, true, false)
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > width {
			t.Fatalf("preview exceeds %d columns: %q", width, line)
		}
	}
	if !strings.Contains(got, "Push      off") ||
		!strings.Contains(got, "× web") || !strings.Contains(got, "this-ref") {
		t.Fatalf("preview omitted settings or failure detail:\n%s", got)
	}
}
