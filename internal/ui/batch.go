package ui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/AzraelSec/glock/internal/runner"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
)

type Outcome int

const (
	Success Outcome = iota
	Attention
	Failure
	Skipped
	Canceled
	outcomeCount
)

type Result struct {
	Outcome Outcome
	Label   string
	Detail  string
	Group   string
}

type Task struct {
	Name string
	Run  func() Result
}

type Batch struct {
	Title          string
	Activity       string
	Empty          string
	Banner         string
	Tasks          []Task
	Cancel         context.CancelFunc
	ReadOnly       bool
	Inline         bool
	Groups         []string
	ExpandFailures bool
}

func (b Batch) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	width, height := TerminalSize(out)
	m := newBatchModel(ctx, b, lipgloss.NewRenderer(out), width, height)
	if len(b.Tasks) == 0 {
		return writeReport(out, m.report())
	}
	if !IsInteractive(in, out) {
		for i, result := range runner.Run(func(task Task) (Result, error) {
			return task.Run(), nil
		}, b.Tasks) {
			m.setResult(i, result.Res)
		}
		return writeReport(out, m.report())
	}

	opts := []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(out)}
	if !b.Inline {
		opts = append(opts, tea.WithAltScreen(), tea.WithMouseCellMotion())
	} else {
		if err := writeReport(out, m.heading()); err != nil {
			return err
		}
	}
	if b.Cancel != nil {
		// The caller owns signal handling so canceled workers can be reaped.
		opts = append(opts, tea.WithoutSignalHandler())
	}
	if _, err := tea.NewProgram(m, opts...).Run(); err != nil {
		return err
	}
	if m.finished == len(b.Tasks) {
		if b.Inline {
			return writeReport(out, m.totals())
		}
		return writeReport(out, m.report())
	}
	return nil
}

type taskDoneMsg struct {
	index  int
	result Result
}

type batchCanceledMsg struct{}
type inlineFlushedMsg struct{}

type batchModel struct {
	ctx context.Context
	Batch
	renderer *lipgloss.Renderer
	spinner  spinner.Model
	viewport viewport.Model
	help     help.Model
	width    int
	height   int

	results        []*Result
	counts         [outcomeCount]int
	finished       int
	stopping       bool
	failureCursor  int
	expanded       map[int]bool
	failureLines   map[int]int
	inlinePending  []string
	inlinePrinting bool
}

var (
	interrupt = key.NewBinding(key.WithKeys("ctrl+c", "break"), key.WithHelp("ctrl+c", "cancel"))
	scroll    = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll"))
	page      = key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", "page"))
)

func newBatchModel(ctx context.Context, batch Batch, renderer *lipgloss.Renderer, width, height int) *batchModel {
	m := &batchModel{
		ctx: ctx, Batch: batch, renderer: renderer, spinner: NewSpinner(renderer),
		viewport: viewport.New(0, 0), help: NewHelp(renderer),
		results:       make([]*Result, len(batch.Tasks)),
		failureCursor: -1, expanded: make(map[int]bool),
	}
	m.resize(width, height)
	return m
}

func (m *batchModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick}
	for index, task := range m.Tasks {
		cmds = append(cmds, func() tea.Msg {
			return taskDoneMsg{index: index, result: task.Run()}
		})
	}
	if m.Cancel != nil {
		cmds = append(cmds, func() tea.Msg {
			<-m.ctx.Done()
			return batchCanceledMsg{}
		})
	}
	return tea.Batch(cmds...)
}

func (m *batchModel) setResult(index int, result Result) {
	m.results[index] = &result
	m.counts[result.Outcome]++
	m.finished++
}

func (m *batchModel) resize(width, height int) {
	m.width, m.height = width, height
	m.viewport.Width = max(1, min(width-3, 100))
	m.viewport.Height = max(0, height-6)
	if height < 7 {
		m.viewport.Height = max(0, height-3)
	}
	m.help.Width = m.viewport.Width
	if width > 0 && !m.Inline {
		m.refreshRows()
	}
}

func (m *batchModel) refreshRows() {
	if m.ExpandFailures {
		m.viewport.SetContent(m.updateRows())
	} else {
		m.viewport.SetContent(m.rows(m.viewport.Width))
	}
	m.viewport.SetYOffset(m.viewport.YOffset)
}

func (m *batchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case taskDoneMsg:
		m.setResult(msg.index, msg.result)
		if m.Inline {
			m.inlinePending = append(m.inlinePending, m.row(msg.index, m.contentWidth(), true))
			return m, m.flushInline()
		}
		firstFailure := msg.result.Outcome == Failure && m.failureCursor == -1
		if firstFailure {
			m.failureCursor = msg.index
		}
		if m.complete() && !m.inspecting() {
			return m, tea.Quit
		}
		m.refreshRows()
		if m.ExpandFailures && (firstFailure || m.complete()) {
			m.revealFailure(false)
		}
	case inlineFlushedMsg:
		m.inlinePrinting = false
		return m, m.flushInline()
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case batchCanceledMsg:
		if m.complete() {
			return m, tea.Quit
		}
		m.stopping = true
	case tea.KeyMsg:
		if m.inspecting() && m.complete() && key.Matches(msg, closeResults) {
			m.stopping = true
			return m, tea.Quit
		}
		if m.ExpandFailures && m.handleFailureKey(msg) {
			return m, nil
		}
		if key.Matches(msg, interrupt) {
			if m.complete() && !m.Inline {
				return m, tea.Quit
			}
			if m.ReadOnly {
				return m, tea.Quit
			}
			m.stopping = true
			if m.Cancel != nil {
				m.Cancel()
			}
			return m, nil
		}
	case spinner.TickMsg:
		if m.complete() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *batchModel) flushInline() tea.Cmd {
	if m.inlinePrinting {
		return nil
	}
	if len(m.inlinePending) == 0 {
		if m.complete() {
			return tea.Quit
		}
		return nil
	}
	lines := strings.Join(m.inlinePending, "\n")
	m.inlinePending = m.inlinePending[:0]
	m.inlinePrinting = true
	return tea.Sequence(tea.Println(lines), func() tea.Msg { return inlineFlushedMsg{} })
}

func (m *batchModel) View() string {
	if m.complete() && !m.inspecting() {
		return ""
	}
	activity := m.Activity
	if m.stopping {
		activity = "Waiting for Git operations to finish"
		if m.Cancel != nil {
			activity = "Canceling updates"
		}
	}
	progress := fmt.Sprintf("%s %s · %d/%d complete", m.spinner.View(), activity, m.finished, len(m.Tasks))
	if m.Inline {
		return Fit(progress, m.width, 1)
	}
	if m.complete() {
		progress = m.totals()
	}
	heading := Title(m.renderer).Render(SingleLine(m.Title))
	var bindings []key.Binding
	if m.inspecting() && m.complete() {
		bindings = append(bindings, closeResults)
	}
	if m.ExpandFailures && m.failureCursor >= 0 {
		bindings = append(bindings, toggleFailure, browseFailures)
	}
	if (m.ReadOnly || m.Cancel != nil) && !m.stopping && !m.complete() {
		bindings = append(bindings, interrupt)
	}
	if m.viewport.TotalLineCount() > m.viewport.Height {
		if !m.ExpandFailures || m.failureCursor < 0 {
			bindings = append(bindings, scroll)
		}
		bindings = append(bindings, page)
	}
	footer := m.help.ShortHelpView(bindings)
	if m.height < 7 {
		if m.height <= 3 {
			return Fit(strings.Join([]string{progress, heading, footer}, "\n"), m.width, m.height)
		}
		return Fit(strings.Join([]string{progress, heading, m.viewport.View(), footer}, "\n"), m.width, m.height)
	}
	return Fit(lipgloss.NewStyle().PaddingLeft(2).Render(strings.Join([]string{
		heading, progress, "", m.viewport.View(), "", footer,
	}, "\n")), m.width, m.height)
}

var outcomeStyles = [outcomeCount]struct {
	marker  string
	summary string
	color   lipgloss.AdaptiveColor
}{
	Success:   {"✓", "succeeded", lipgloss.AdaptiveColor{Light: "#18653C", Dark: "#80D9A2"}},
	Attention: {"!", "with warnings", lipgloss.AdaptiveColor{Light: "#865100", Dark: "#FFCA79"}},
	Failure:   {"×", "failed", lipgloss.AdaptiveColor{Light: "#AD2929", Dark: "#FF9B9B"}},
	Skipped:   {"–", "skipped", muted},
	Canceled:  {"–", "canceled", muted},
}

func (m *batchModel) rows(width int) string {
	var lines []string
	for i := range m.Tasks {
		lines = append(lines, m.row(i, width, true))
	}
	return strings.Join(lines, "\n")
}

func (m *batchModel) row(index, width int, showDetail bool) string {
	name := m.renderer.NewStyle().Bold(true).Render(SingleLine(m.Tasks[index].Name))
	result := m.results[index]
	row := Muted(m.renderer).Render("·") + " " + name
	if result != nil {
		style := outcomeStyles[result.Outcome]
		state := m.renderer.NewStyle().Foreground(style.color)
		row = state.Render(style.marker) + " " + name + "  " + state.Render(result.Label)
		if showDetail && result.Detail != "" {
			row += " · " + SingleLine(result.Detail)
		}
	}
	if width > 2 && lipgloss.Width(row) > width {
		row = WrapWords(row, width-2)
		row = strings.ReplaceAll(row, "\n", "\n  ")
	} else if width > 0 && lipgloss.Width(row) > width {
		row = WrapWords(row, width)
	}
	return row
}

func (m *batchModel) complete() bool { return m.finished == len(m.Tasks) }

func (m *batchModel) inspecting() bool {
	return m.ExpandFailures && m.counts[Failure] > 0 && !m.stopping && m.ctx.Err() == nil
}

// WrapWords wraps text by terminal display width and breaks overlong words.
func WrapWords(text string, width int) string {
	writer := wordwrap.NewWriter(width)
	writer.Breakpoints = nil
	_, _ = io.WriteString(writer, text)
	_ = writer.Close()
	return wrap.String(writer.String(), width)
}

func (m *batchModel) contentWidth() int {
	if m.width > 0 {
		return max(1, min(m.width-3, 100))
	}
	return 0
}

func (m *batchModel) heading() string {
	heading := Title(m.renderer).Render(SingleLine(m.Title))
	width := m.contentWidth()
	if width > 0 {
		heading = WrapWords(heading, width)
	}
	if m.Banner != "" && (width == 0 || lipgloss.Width(m.Banner) <= width) {
		heading = strings.TrimRight(m.Banner, "\n") + "\n\n" + heading
	}
	return heading
}

func (m *batchModel) totals() string {
	var summary []string
	for state, count := range m.counts {
		if count > 0 {
			summary = append(summary, fmt.Sprintf("%d %s", count, outcomeStyles[state].summary))
		}
	}
	totals := Muted(m.renderer).Render(strings.Join(summary, " · "))
	if width := m.contentWidth(); width > 0 {
		totals = WrapWords(totals, width)
	}
	return totals
}

func (m *batchModel) report() string {
	if len(m.Tasks) == 0 {
		return m.heading() + "\n" + m.Empty
	}
	width := m.contentWidth()
	if len(m.Groups) == 0 {
		return m.heading() + "\n" + m.totals() + "\n\n" + m.rows(width)
	}

	rows := make(map[string][]string)
	for i, result := range m.results {
		rows[result.Group] = append(rows[result.Group], m.row(i, width, true))
	}
	sections := []string{m.heading() + fmt.Sprintf("\n%d repositories checked", len(m.Tasks))}
	for _, group := range m.Groups {
		if len(rows[group]) > 0 {
			title := Title(m.renderer).Render(fmt.Sprintf("%s · %d", group, len(rows[group])))
			sections = append(sections, title+"\n"+strings.Join(rows[group], "\n"))
		}
	}
	return strings.Join(sections, "\n\n")
}
