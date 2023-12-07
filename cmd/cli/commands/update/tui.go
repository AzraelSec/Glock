package update

import (
	"bytes"
	"context"
	"fmt"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/pkg/ui"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type tui struct {
	ctx        context.Context
	cancelCtx  context.CancelFunc
	repos      []config.LiveRepo
	updateFn   updateRunnerFunc
	updateArgs []updateInputPayload
}

func newTui(repos []config.LiveRepo) *tui {
	ctx, cancel := context.WithCancel(context.Background())
	updateFn, updateArgs := runnerArgs(ctx, repos, false)
	return &tui{ctx, cancel, repos, updateFn, updateArgs}
}

func (t *tui) initModel() *model {
	tasks := make([]*task, 0, len(t.repos))
	for idx, repo := range t.repos {
		tasks = append(tasks, &task{
			idx:  idx,
			name: repo.Name,
			update: func(idx int) (updateOutputPayload, error) {
				return t.updateFn(t.updateArgs[idx])
			},
		})
	}

	return &model{
		cancelCtx: t.cancelCtx,
		status:    RUNNING,
		tasks:     tasks,
		completed: make([]int, 0, len(tasks)),
		spinner:   ui.NewSpinner(),
	}
}

func (t *tui) run() error {
	m, err := tea.NewProgram(t.initModel()).Run()
	if err != nil {
		return err
	}

	tm := m.(*model)
	if tm.status == ABORTED {
		fmt.Println(ui.RED.Render("Update aborted!"))
	}
	return nil
}

type task struct {
	idx    int // note: relative to the repos array
	name   string
	done   bool
	result updateOutputPayload
	err    error
	update func(int) (updateOutputPayload, error)
}

const (
	RUNNING = iota
	ABORTED
	DONE
)

type status int

type model struct {
	cancelCtx context.CancelFunc
	status    status
	tasks     []*task

	completed []int

	spinner spinner.Model
}

type updateStartMsg struct{ idx int }
type updateDoneMsg struct{ idx int }
type abortMsg struct{}

func updateStartCmd(idx int) tea.Cmd {
	return func() tea.Msg {
		return updateStartMsg{idx}
	}
}

func (m *model) processUpdateStartCmd(idx int) tea.Cmd {
	return func() tea.Msg {
		task := m.tasks[idx]
		task.result, task.err = task.update(idx)

		return updateDoneMsg{idx}
	}
}

func (m *model) abortStartCmd() tea.Msg {
	m.status = ABORTED
	return abortMsg{}
}

func (m *model) abortCmd() tea.Msg {
	m.cancelCtx()
	return struct{}{}
}

func (m *model) renderTaskRow(t *task) string {
	var buff bytes.Buffer

	if !t.done {
		buff.WriteString(m.spinner.View())
		buff.WriteString(fmt.Sprintf("%s: updating...", t.name))
		return buff.String()
	}

	if t.err != nil {
		buff.WriteString("⛔ ")
		buff.WriteString(fmt.Sprintf("%s: %s", t.name, ui.RED.Render(t.err.Error())))
		return buff.String()
	}

	if t.result.Ignored {
		buff.WriteString("🫥 ")
		buff.WriteString(ui.STRIKE.Render(fmt.Sprintf("%s: ignored", t.name)))
		return buff.String()
	}

	buff.WriteString(fmt.Sprintf("✅ %s", t.name))
	buff.WriteString(fmt.Sprintf(" [%s] ", ui.YELLOW.Render(t.result.UpdaterTag)))

	if t.result.Inferred {
		buff.WriteString(ui.YELLOW.Render("(inferred)"))
	} else {
		buff.WriteString(ui.YELLOW.Render("(configured)"))
	}

	buff.WriteString(fmt.Sprintf(": %s", ui.GREEN.Render("repo updated successfully!")))

	return buff.String()
}

func (m *model) Init() tea.Cmd {
	updateBatch := make([]tea.Cmd, 0)
	updateBatch = append(updateBatch, m.spinner.Tick)
	for idx := range m.tasks {
		updateBatch = append(updateBatch, updateStartCmd(idx))
	}

	return tea.Batch(updateBatch...)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case updateStartMsg:
		return m, m.processUpdateStartCmd(msg.idx)
	case updateDoneMsg:
		m.tasks[msg.idx].done = true
		m.completed = append(m.completed, msg.idx)
		if len(m.completed) == len(m.tasks) {
			if m.status != ABORTED {
				m.status = DONE
			}
			return m, tea.Quit
		}
	case tea.KeyMsg:
		if msg.Type == tea.KeyBreak {
			return m, m.abortStartCmd
		}
	case abortMsg:
		return m, m.abortCmd
	}

	var spinnerCmd tea.Cmd
	m.spinner, spinnerCmd = m.spinner.Update(msg)
	return m, spinnerCmd
}

func (m *model) View() string {
	if m.status == ABORTED {
		return ""
	}

	var buff bytes.Buffer
	for _, task := range m.tasks {
		buff.WriteString(m.renderTaskRow(task))
		buff.WriteString("\n")
	}
	return buff.String()
}
