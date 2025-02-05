package tag

import (
	"bytes"
	"fmt"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/ui"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type tui struct {
	repos   []config.LiveRepo
	tagFn   tagRunnerFunc
	tagArgs []tagInputPayload
	isYeet  bool
}

func newTui(g git.Git, repos []config.LiveRepo, tagPattern string, useCurrent, skipPush, pullBefore, isYeet bool) *tui {
	tagFn, tagArgs := runnerArgs(g, repos, tagPattern, useCurrent, skipPush, pullBefore)
	return &tui{repos, tagFn, tagArgs, isYeet}
}

func (t *tui) initModel() *model {
	tasks := make([]*task, 0, len(t.repos))
	for idx, repo := range t.repos {
		tasks = append(tasks, &task{
			idx:  idx,
			name: repo.Name,
			run: func(idx int) (tagOutputPayload, error) {
				return t.tagFn(t.tagArgs[idx])
			},
		})
	}

	return &model{
		status:    RUNNING,
		tasks:     tasks,
		completed: make([]int, 0, len(tasks)),
		spinner:   ui.NewSpinner(),
		isYeet:    t.isYeet,
	}
}

func (t *tui) run() error {
	_, err := tea.NewProgram(t.initModel()).Run()
	return err
}

type task struct {
	idx    int
	name   string
	done   bool
	result tagOutputPayload
	err    error
	run    func(int) (tagOutputPayload, error)
}

const (
	RUNNING = iota
	DONE
)

type status int

type model struct {
	status status
	tasks  []*task

	completed []int

	spinner spinner.Model
	isYeet  bool
}

type (
	tagStartMsg struct{ idx int }
	tagDoneMsg  struct{ idx int }
)

func tagStartCmd(idx int) tea.Cmd {
	return func() tea.Msg {
		return tagStartMsg{idx}
	}
}

func (m *model) processTagStartCmd(idx int) tea.Cmd {
	return func() tea.Msg {
		task := m.tasks[idx]
		task.result, task.err = task.run(idx)
		return tagDoneMsg{idx}
	}
}

func (m *model) renderTaskRow(t *task) string {
	var buff bytes.Buffer

	if !t.done {
		buff.WriteString(m.spinner.View())
		buff.WriteString(fmt.Sprintf("%s: tagging...", t.name))
		return buff.String()
	}

	if t.err != nil {
		buff.WriteString("⛔ ")
		buff.WriteString(fmt.Sprintf("%s: %s", t.name, ui.RED.Render(t.err.Error())))
		return buff.String()
	}

	buff.WriteString(fmt.Sprintf("✅ %s", t.name))
	if t.result.remote != "" {
		buff.WriteString(fmt.Sprintf(" [%s => %s] :", ui.ORANGE.Render(t.result.branch), ui.ORANGE.Render(t.result.remote)))
	} else {
		buff.WriteString(fmt.Sprintf(" [%s] :", ui.ORANGE.Render(t.result.branch)))
	}
	buff.WriteString(fmt.Sprintf(" %s", ui.YELLOW.Render(t.result.tag)))

	return buff.String()
}

func (m *model) Init() tea.Cmd {
	updateBatch := make([]tea.Cmd, 0)
	updateBatch = append(updateBatch, m.spinner.Tick)
	for idx := range m.tasks {
		updateBatch = append(updateBatch, tagStartCmd(idx))
	}

	return tea.Batch(updateBatch...)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tagStartMsg:
		return m, m.processTagStartCmd(msg.idx)
	case tagDoneMsg:
		m.tasks[msg.idx].done = true
		m.completed = append(m.completed, msg.idx)
		if len(m.completed) == len(m.tasks) {
			m.status = DONE
			return m, tea.Quit
		}
	}

	var spinnerCmd tea.Cmd
	m.spinner, spinnerCmd = m.spinner.Update(msg)
	return m, spinnerCmd
}

func (m *model) View() string {
	var buff bytes.Buffer
	if m.isYeet {
		buff.WriteString(YEET_ASCII_IMAGE)
	}

	for _, task := range m.tasks {
		buff.WriteString(m.renderTaskRow(task))
		buff.WriteString("\n")
	}
	return buff.String()
}
