package switchcmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/routine"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/internal/ui"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func (sg *switchGit) runPicker(cmd *cobra.Command, force bool) error {
	in, out := cmd.InOrStdin(), cmd.OutOrStdout()
	width, height := ui.TerminalSize(out)
	if !ui.IsInteractive(in, out) {
		return errors.New("provide a branch with glock switch <branch> when running without an interactive terminal")
	}

	picker := newBranchPicker(sg.collectBranches, lipgloss.NewRenderer(out), force)
	picker.inspect = func(branch string) []branchImpact { return sg.branchImpact(branch, force) }
	picker.resize(width, height)
	if _, err := tea.NewProgram(picker, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	if picker.target == "" {
		return nil
	}
	if !force && !routine.AllClean(sg.repos, sg.Git) {
		return errors.New("repositories changed during review; commit or stash local changes before switching")
	}
	return sg.runSwitch(cmd, in, picker.target, force, picker.review.repos)
}

type branchesLoaded struct {
	names    []string
	failures int
	firstErr error
}

func (sg *switchGit) collectBranches() branchesLoaded {
	results := runner.Run(func(repo config.LiveRepo) ([]git.BranchName, error) {
		if !dir.DirExists(repo.GitConfig.Path) {
			return nil, config.ErrRepoNotFound
		}
		return sg.ListBranches(repo.GitConfig)
	}, sg.repos)

	var loaded branchesLoaded
	names := make(map[string]struct{})
	for i, result := range results {
		if result.Error != nil {
			loaded.failures++
			if loaded.firstErr == nil {
				loaded.firstErr = fmt.Errorf("%s: %w", sg.repos[i].Name, result.Error)
			}
			continue
		}
		for _, branch := range result.Res {
			if branch != "" {
				names[string(branch)] = struct{}{}
			}
		}
	}
	for name := range names {
		loaded.names = append(loaded.names, name)
	}
	slices.Sort(loaded.names)
	return loaded
}

type branchItem string

func (b branchItem) Title() string       { return ui.SingleLine(string(b)) }
func (b branchItem) Description() string { return "" }
func (b branchItem) FilterValue() string { return b.Title() }

var chooseBranch = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review"))
var dismissPicker = key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q/esc", "quit"))

type filterMatches struct {
	revision int
	matches  list.FilterMatchesMsg
}

// Keep asynchronous Bubbles filter results tied to the query that produced them.
func trackFilter(cmd tea.Cmd, revision int) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case list.FilterMatchesMsg:
			return filterMatches{revision, msg}
		case tea.BatchMsg:
			for i, sub := range msg {
				msg[i] = trackFilter(sub, revision)
			}
			return msg
		default:
			return msg
		}
	}
}

type branchPicker struct {
	load     func() branchesLoaded
	renderer *lipgloss.Renderer
	spinner  spinner.Model
	list     list.Model
	force    bool

	width, height  int
	loaded         bool
	failures       int
	firstErr       error
	target         string
	filterRevision int
	filterPending  bool
	inspect        func(string) []branchImpact
	reviewRevision int
	review         *branchReview
}

func newBranchPicker(load func() branchesLoaded, renderer *lipgloss.Renderer, force bool) *branchPicker {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = renderer.NewStyle().PaddingLeft(2)
	delegate.Styles.DimmedTitle = ui.Muted(renderer).PaddingLeft(2)
	delegate.Styles.SelectedTitle = ui.Title(renderer).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		PaddingLeft(1)
	delegate.ShortHelpFunc = func() []key.Binding { return []key.Binding{chooseBranch} }
	delegate.FullHelpFunc = func() [][]key.Binding { return [][]key.Binding{{chooseBranch}} }

	l := list.New(nil, delegate, 8, 1)
	l.Title = "Switch branch"
	l.Styles.Title = ui.Title(renderer)
	l.Styles.TitleBar = renderer.NewStyle()
	l.Styles.StatusBar = ui.Muted(renderer)
	l.Styles.StatusEmpty = ui.Muted(renderer)
	l.Styles.NoItems = ui.Muted(renderer).PaddingLeft(2)
	l.Styles.PaginationStyle = ui.Muted(renderer).PaddingLeft(2)
	l.Styles.ArabicPagination = ui.Muted(renderer)
	l.Styles.HelpStyle = renderer.NewStyle()
	l.Help = ui.NewHelp(renderer)
	l.Help.ShortSeparator = "  "
	l.FilterInput.Prompt = "Filter: "
	l.FilterInput.PromptStyle = ui.Title(renderer)
	l.SetStatusBarItemName("branch", "branches")
	l.Paginator.Type = paginator.Arabic
	l.KeyMap.CursorUp.SetHelp("↑", "up")
	l.KeyMap.CursorDown.SetHelp("↓", "down")
	l.KeyMap.Quit.SetHelp("q/esc", "quit")
	l.KeyMap.AcceptWhileFiltering.SetKeys("enter")

	return &branchPicker{load: load, renderer: renderer, spinner: ui.NewSpinner(renderer), list: l, force: force}
}

func (p *branchPicker) Init() tea.Cmd {
	return tea.Batch(p.spinner.Tick, func() tea.Msg { return p.load() })
}

func (p *branchPicker) resize(width, height int) {
	p.width, p.height = width, height
	if p.review != nil {
		p.resizeReview()
	}
	p.list.SetShowStatusBar(height >= 7)
	p.list.SetShowHelp(height >= 7)
	p.list.SetShowPagination(height >= 5)
	if (p.force || p.failures > 0) && height >= 3 {
		height--
	}
	p.list.SetSize(max(4, min(100, width-3)), max(1, height))
	p.updatePageKeys()
}

func (p *branchPicker) updatePageKeys() {
	canPage := p.list.FilterState() != list.Filtering && p.list.Paginator.TotalPages > 1
	p.list.KeyMap.PrevPage.SetEnabled(canPage)
	p.list.KeyMap.NextPage.SetEnabled(canPage)
}

func (p *branchPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.resize(msg.Width, msg.Height)
		return p, nil
	case impactLoaded:
		p.setReview(msg)
		return p, nil
	case branchesLoaded:
		p.loaded = true
		p.failures, p.firstErr = msg.failures, msg.firstErr
		items := make([]list.Item, 0, len(msg.names))
		for _, name := range msg.names {
			items = append(items, branchItem(name))
		}
		cmd := p.list.SetItems(items)
		p.resize(p.width, p.height)
		return p, cmd
	case spinner.TickMsg:
		if p.loaded && (p.review == nil || p.review.repos != nil) {
			return p, nil
		}
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		return p, cmd
	case filterMatches:
		if msg.revision != p.filterRevision || p.list.FilterState() != list.Filtering {
			return p, nil
		}
		p.filterPending = false
		p.list, _ = p.list.Update(msg.matches)
		p.resize(p.width, p.height)
		return p, nil
	case tea.KeyMsg:
		if p.review != nil {
			return p.reviewUpdate(msg)
		}
		if !p.loaded || len(p.list.Items()) == 0 {
			if key.Matches(msg, dismissPicker) {
				return p, tea.Quit
			}
			return p, nil
		}
		if key.Matches(msg, chooseBranch) {
			if p.list.FilterState() == list.Filtering {
				if p.filterPending || len(p.list.VisibleItems()) == 0 {
					return p, nil
				}
				return p.updateList(msg)
			}
			if item, ok := p.list.SelectedItem().(branchItem); ok {
				return p, p.startReview(string(item))
			}
			return p, nil
		}
	}

	if p.review != nil {
		return p.reviewUpdate(msg)
	}
	if !p.loaded {
		return p, nil
	}
	return p.updateList(msg)
}

func (p *branchPicker) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	previousQuery := p.list.FilterValue()
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	if p.list.FilterValue() != previousQuery {
		p.filterRevision++
		p.filterPending = p.list.FilterState() == list.Filtering
		if p.filterPending {
			cmd = trackFilter(cmd, p.filterRevision)
		}
	}
	return p, cmd
}

func (p *branchPicker) View() string {
	if p.target != "" {
		return ""
	}

	var view string
	switch {
	case p.review != nil:
		view = p.reviewView()
	case !p.loaded:
		view = ui.Title(p.renderer).Render("Switch branch") + "\n" +
			p.spinner.View() + " Loading local branches"
	case len(p.list.Items()) == 0:
		message := "No local branches found."
		if p.firstErr != nil {
			message += " " + ui.ErrorDetail(p.firstErr)
		}
		view = ui.Title(p.renderer).Render("Switch branch") + "\n" + message +
			"\n\n" + ui.Muted(p.renderer).Render("q / esc to close")
	default:
		var notes []string
		if p.force {
			notes = append(notes, "Force enabled. Local changes may be discarded.")
		}
		if p.failures > 0 {
			unavailable := fmt.Sprintf("%d repositories unavailable", p.failures)
			if p.failures == 1 {
				unavailable = "1 repository unavailable"
			}
			notes = append(notes, unavailable)
		}
		view = p.list.View()
		if len(notes) > 0 && p.height >= 3 {
			heading, body, _ := strings.Cut(view, "\n")
			view = heading + "\n" + ui.Muted(p.renderer).Render(strings.Join(notes, " · ")) + "\n" + body
		}
	}

	return ui.Fit(lipgloss.NewStyle().PaddingLeft(2).Render(view), p.width, p.height)
}
