package switchcmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/internal/ui"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type impactKind int

const (
	eligible impactKind = iota
	alreadyOnBranch
	branchUnavailable
	unavailable
)

type branchImpact struct {
	repo   config.LiveRepo
	kind   impactKind
	detail string
}

type impactLoaded struct {
	revision int
	repos    []branchImpact
}

type branchReview struct {
	target   string
	revision int
	repos    []branchImpact
	viewport viewport.Model
}

var (
	confirmSwitch = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch"))
	backToPicker  = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	quitSwitch    = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit"))
	reviewScroll  = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll"))
)

func (sg *switchGit) branchImpact(branch string, force bool) []branchImpact {
	results := runner.Run(func(repo config.LiveRepo) (branchImpact, error) {
		return sg.inspectRepo(repo, git.BranchName(branch), force), nil
	}, sg.repos)

	impacts := make([]branchImpact, 0, len(results))
	for _, result := range results {
		impacts = append(impacts, result.Res)
	}
	return impacts
}

func (sg *switchGit) inspectRepo(repo config.LiveRepo, branch git.BranchName, force bool) branchImpact {
	failure := func(err error) branchImpact {
		return branchImpact{repo, unavailable, ui.ErrorDetail(err)}
	}
	if !dir.DirExists(repo.GitConfig.Path) {
		return failure(config.ErrRepoNotFound)
	}
	status, err := sg.Status(repo.GitConfig)
	if err != nil {
		return failure(err)
	}
	if status.Change && !force {
		return failure(errors.New("working tree has local changes"))
	}
	if status.Branch == branch {
		if force && status.Change {
			return branchImpact{repo, eligible, "Already on branch · forced switch can discard local changes"}
		}
		return branchImpact{repo, alreadyOnBranch, "Already on " + string(branch)}
	}

	branches, err := sg.ListBranches(repo.GitConfig)
	if err != nil {
		return failure(err)
	}
	detail := fmt.Sprintf("%s → %s", status.Branch, branch)
	if status.Change {
		detail += " · local changes may be discarded"
	}
	if slices.Contains(branches, branch) {
		return branchImpact{repo, eligible, detail}
	}

	remote, err := sg.HasRemoteBranch(repo.GitConfig, branch)
	if err != nil {
		return failure(err)
	}
	if remote {
		return branchImpact{repo, eligible, detail + " · cached remote branch"}
	}
	return branchImpact{repo, branchUnavailable, "No local or cached remote branch"}
}

func (r *branchReview) canSwitch() bool {
	return slices.ContainsFunc(r.repos, func(repo branchImpact) bool { return repo.kind == eligible })
}

func (p *branchPicker) startReview(branch string) tea.Cmd {
	p.reviewRevision++
	revision := p.reviewRevision
	p.review = &branchReview{target: branch, revision: revision, viewport: viewport.New(0, 0)}
	p.resizeReview()
	return tea.Batch(p.spinner.Tick, func() tea.Msg {
		return impactLoaded{revision, p.inspect(branch)}
	})
}

func (p *branchPicker) resizeReview() {
	r := p.review
	r.viewport.Width = max(1, min(100, p.width-3))
	headerHeight := 1 + strings.Count(ui.WrapWords(ui.SingleLine(r.target), r.viewport.Width), "\n") + 1
	if p.height >= 7 {
		headerHeight++
	}
	r.viewport.Height = max(1, p.height-headerHeight-1)
	if r.repos != nil {
		p.renderReviewRows()
	}
}

func (p *branchPicker) setReview(loaded impactLoaded) {
	r := p.review
	if r == nil || r.revision != loaded.revision {
		return
	}
	r.repos = loaded.repos
	p.renderReviewRows()
}

func (p *branchPicker) renderReviewRows() {
	r := p.review
	var sections []string
	for kind, label := range []string{"Eligible", "Already on branch", "Skipped", "Unavailable"} {
		var lines []string
		for _, impact := range r.repos {
			if impact.kind == impactKind(kind) {
				row := ui.SingleLine(impact.repo.Name) + "  " + ui.SingleLine(impact.detail)
				lines = append(lines, ui.WrapWords(row, r.viewport.Width))
			}
		}
		if len(lines) > 0 {
			heading := ui.Title(p.renderer).Render(fmt.Sprintf("%s · %d", label, len(lines)))
			sections = append(sections, heading+"\n"+strings.Join(lines, "\n"))
		}
	}
	if len(sections) == 0 {
		sections = append(sections, "No repositories to review.")
	}
	r.viewport.SetContent(strings.Join(sections, "\n\n"))
	r.viewport.SetYOffset(r.viewport.YOffset)
}

func (p *branchPicker) reviewUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	r := p.review
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(msg, backToPicker):
			p.review = nil
			return p, nil
		case key.Matches(msg, quitSwitch):
			return p, tea.Quit
		case key.Matches(msg, confirmSwitch) && r.canSwitch():
			p.target = r.target
			return p, tea.Quit
		}
	}
	var cmd tea.Cmd
	r.viewport, cmd = r.viewport.Update(msg)
	return p, cmd
}

func (p *branchPicker) reviewView() string {
	r := p.review
	title := "Review switch"
	if p.force {
		title = "Review forced switch"
	}
	target := ui.WrapWords(ui.SingleLine(r.target), r.viewport.Width)
	header := ui.Title(p.renderer).Render(title) + "\n" + target
	help := ui.NewHelp(p.renderer)
	help.Width = r.viewport.Width
	help.ShortSeparator = "  "
	if r.repos == nil {
		return header + "\n" + p.spinner.View() + " Checking repositories\n" + help.ShortHelpView([]key.Binding{backToPicker, quitSwitch})
	}
	bindings := []key.Binding{backToPicker}
	if r.canSwitch() {
		bindings = append([]key.Binding{confirmSwitch}, bindings...)
	}
	if r.viewport.TotalLineCount() > r.viewport.Height {
		bindings = append(bindings, reviewScroll)
	}
	bindings = append(bindings, quitSwitch)
	footer := help.ShortHelpView(bindings)
	if p.height < 7 {
		return header + "\n" + r.viewport.View() + "\n" + footer
	}
	note := "Local and cached branches"
	if p.force {
		note = "Local changes may be discarded."
	}
	return header + "\n" + ui.Muted(p.renderer).Render(note) + "\n" +
		r.viewport.View() + "\n" + footer
}
