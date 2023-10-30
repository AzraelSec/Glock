package status

import (
	"fmt"
	"os"
	"strings"

	"github.com/AzraelSec/glock/internal/runner"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

const DIRTY_ATTR = "DIRTY(🛠)"
const OUT_OF_SYNC_ATTR = "OOS(🧨)"
const (
	RUNNING = iota
	DONE
)

type state int
type model struct {
	collectAction func() []runner.Result[statusOutputPayload]
	results       []runner.Result[statusOutputPayload]

	state state

	spinner spinner.Model
}

func (s *status) initModel() *model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return &model{
		spinner:       sp,
		state:         RUNNING,
		collectAction: s.collect,
	}
}

type collectMsg struct{}
type collectDoneMsg struct{}

func collectCmd() tea.Msg {
	return collectMsg{}
}

func (m *model) startCollect() tea.Msg {
	m.results = m.collectAction()
	return collectDoneMsg{}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, collectCmd)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case collectMsg:
		return m, m.startCollect
	case collectDoneMsg:
		m.state = DONE
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
	}

	var spinnerCmd tea.Cmd
	m.spinner, spinnerCmd = m.spinner.Update(msg)
	return m, spinnerCmd
}

func (m *model) View() string {
	if m.state == RUNNING {
		return fmt.Sprintf("%s collecting info...\n", m.spinner.View())
	}
	return ""
}

func (s *status) runTui() {
	program := tea.NewProgram(s.initModel())
	m, err := program.Run()
	if err != nil {
		panic(err)
	}

	tm := m.(*model)
	if tm.state != DONE {
		return
	}

	s.print(tm.results)
}

func (s *status) print(results []runner.Result[statusOutputPayload]) {
	re := lipgloss.NewRenderer(os.Stdout)
	baseStyle := re.NewStyle().
		Padding(0, 1)
	headerStyle := baseStyle.Copy().
		Foreground(lipgloss.Color("252")).
		Bold(true)
	headers := []string{"REPO", "BRANCH", "INFO"}

	dataList := make([][]string, 0)
	for idx, res := range results {
		if res.Error != nil {
			dataList = append(dataList, []string{s.cm.Repos[idx].Name, "ERROR", res.Error.Error()})
			continue
		}

		attrs := []string{}
		if res.Res.dirty {
			attrs = append(attrs, DIRTY_ATTR)
		}
		if res.Res.remoteDiff {
			attrs = append(attrs, OUT_OF_SYNC_ATTR)
		}
		dataList = append(dataList, []string{s.cm.Repos[idx].Name, res.Res.branch, strings.Join(attrs, ", ")})
	}

	switchedTable := table.New().
		Border(lipgloss.RoundedBorder()).
		Headers(headers...).
		BorderStyle(re.NewStyle().Foreground(lipgloss.Color("238"))).
		Rows(dataList...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return headerStyle
			}
			return baseStyle
		})

	fmt.Println(switchedTable.Render())
}
