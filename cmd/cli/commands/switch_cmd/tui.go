package switchcmd

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/AzraelSec/glock/pkg/utils"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tui struct {
	*switchGit
}

func newTui(sg *switchGit) *tui {
	return &tui{switchGit: sg}
}

func (t *tui) collectBranches(repo git.Repo) ([]string, error) {
	brs := make([]string, 0)
	if !dir.DirExists(repo.Path) {
		return brs, config.RepoNotFoundErr
	}

	res, err := t.ListBranches(repo)
	if err != nil {
		return brs, err
	}

	for _, br := range res {
		brs = append(brs, string(br))
	}
	return brs, nil
}

func (t *tui) collectAllBranches() []string {
	brsArgs := make([]git.Repo, 0, len(t.repos))
	for _, repo := range t.repos {
		brsArgs = append(brsArgs, repo.GitConfig)
	}
	brsFn := func(repo git.Repo) ([]string, error) {
		return t.collectBranches(repo)
	}

	results := runner.Run(brsFn, brsArgs)

	brs := make([]string, 0)
	for _, item := range results {
		if item.Error != nil {
			continue
		}
		brs = append(brs, item.Res...)
	}

	res := utils.Uniq(brs)
	slices.Sort(res)
	return res
}

func (t *tui) initModel(force bool) *model {
	l := list.New([]list.Item{}, itemDelegate{}, 20, 14)
	l.Styles.PaginationStyle = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	l.Styles.HelpStyle = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	l.Styles.TitleBar = lipgloss.NewStyle().MarginLeft(2)

	return &model{
		status: LOADING_LIST,
		force:  force,

		results: make([]runner.Result[struct{}], 0),

		getBranches: t.collectAllBranches,
		startSwitch: func(b string) []runner.Result[struct{}] {
			return t.performSwitch(b, force)
		},

		list:    l,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points)),
	}
}

func (t *tui) run(force bool) {
	program := tea.NewProgram(t.initModel(force))
	m, err := program.Run()
	if err != nil {
		panic(err)
	}

	tm := m.(*model)
	if tm.status != DONE {
		return
	}

	printResults(t.repos, tm.results)
}

const (
	LOADING_LIST = iota
	READY
	SWITCHING
	DONE
)

type status int

type model struct {
	status status
	force  bool

	results []runner.Result[struct{}]

	getBranches func() []string
	startSwitch func(string) []runner.Result[struct{}]

	spinner spinner.Model
	list    list.Model
}

type branchesListDoneMsg []string

func (t *model) fetchBranches() tea.Msg {
	branches := t.getBranches()
	return branchesListDoneMsg(branches)
}

type switchStartMsg string
type switchDoneMsg struct{}

func switchStart(item item) tea.Cmd {
	return func() tea.Msg {
		return switchStartMsg(item)
	}
}
func (m *model) switchRun(target string) tea.Cmd {
	return func() tea.Msg {
		m.results = m.startSwitch(target)
		return switchDoneMsg{}
	}
}

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
)

type item string

func (i item) FilterValue() string { return string(i) }

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.fetchBranches, m.spinner.Tick)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		if m.status != READY {
			break
		}

		switch keypress := msg.String(); keypress {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			m.status = SWITCHING
			item := m.list.SelectedItem().(item)
			return m, switchStart(item)
		}

	case branchesListDoneMsg:
		its := make([]list.Item, 0, len(msg))
		for _, it := range msg {
			its = append(its, item(it))
		}
		m.status = READY
		return m, m.list.SetItems(its)

	case switchStartMsg:
		cmds = append(cmds, m.switchRun(string(msg)))

	case switchDoneMsg:
		m.status = DONE
		return m, tea.Quit
	}

	var listCmd tea.Cmd
	m.list, listCmd = m.list.Update(msg)

	var spinnerCmd tea.Cmd
	m.spinner, spinnerCmd = m.spinner.Update(msg)
	cmds = append(cmds, listCmd, spinnerCmd)

	return m, tea.Batch(cmds...)
}

func (m *model) View() string {
	switch m.status {
	case LOADING_LIST:
		return fmt.Sprintf("%s loading list...\n", m.spinner.View())
	case READY:
		return m.list.View()
	case SWITCHING:
		return fmt.Sprintf("%s switching...\n", m.spinner.View())
	default:
		return ""
	}
}
