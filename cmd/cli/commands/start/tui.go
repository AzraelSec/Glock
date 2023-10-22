package start

import (
	"context"
	"strings"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/pkg/tui"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var STATUS_BAR_HEIGHT = 1

type startTab struct {
	name   string
	stream *strings.Builder
}

type startTuiModel struct {
	completed *bool
	cancel    context.CancelFunc

	tabs      []startTab
	activeTab int

	viewport    viewport.Model
	historyMode bool
	ready       bool
}

type bufferUpdateMsg interface{}

func flushBuffer() tea.Msg {
	return bufferUpdateMsg(nil)
}

func (m startTuiModel) Init() tea.Cmd {
	return nil
}

func (m *startTuiModel) handleBufferFlush() {
	if !m.ready {
		return
	}

	m.viewport.SetContent(m.tabs[m.activeTab].stream.String())

	if !m.historyMode {
		m.viewport.GotoBottom()
	}
}

func (m *startTuiModel) handleResizeMsg(msg tea.WindowSizeMsg) {
	if !m.ready {
		m.ready = true
		m.viewport = viewport.New(msg.Width, msg.Height-STATUS_BAR_HEIGHT)
		m.viewport.SetContent(m.tabs[m.activeTab].stream.String())
		return
	}

	m.viewport.Width = msg.Width
	m.viewport.Height = msg.Height - STATUS_BAR_HEIGHT
}

func (m *startTuiModel) handleKeyPressMsg(msg tea.KeyMsg) bool {
	switch keypress := msg.String(); keypress {
	case "ctrl+c", "q":
		return true
	case "right", "l", "n", "tab":
		if m.activeTab == len(m.tabs)-1 {
			return false
		}
		m.activeTab = m.activeTab + 1
		m.viewport.SetContent(m.tabs[m.activeTab].stream.String())
	case "left", "h", "p", "shift+tab":
		if m.activeTab == 0 {
			return false
		}
		m.activeTab = m.activeTab - 1
		m.viewport.SetContent(m.tabs[m.activeTab].stream.String())
	case "up", "k":
		m.historyMode = true
	case "down", "j":
		if m.viewport.AtBottom() {
			m.historyMode = false
		}
	}
	return false
}

func (m startTuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch cmsg := msg.(type) {
	case tea.KeyMsg:
		if exit := m.handleKeyPressMsg(cmsg); exit {
			m.cancel()
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.handleResizeMsg(cmsg)
	case bufferUpdateMsg:
		m.handleBufferFlush()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m startTuiModel) View() string {
	if !m.ready {
		return "loading..."
	}

	doc := strings.Builder{}

	tabs := make([]tui.StatusBarTab, 0, len(m.tabs))
	for i, tab := range m.tabs {
		tabs = append(tabs, tui.StatusBarTab{
			Label:  tab.name,
			Active: i == m.activeTab,
		})
	}

	bar := tui.StatusBar{
		Completed:   *m.completed,
		HistoryMode: m.historyMode,
		Height:      STATUS_BAR_HEIGHT,
		Tabs:        tabs,
	}

	doc.WriteString(
		lipgloss.JoinVertical(lipgloss.Top, bar.Render(), m.viewport.View()))

	return doc.String()
}

func executeTUI(cm *config.ConfigManager, executableRepo []config.LiveRepo, disposableRepo []config.LiveRepo, executableService []config.Services, disposableService []config.Services) {
	
}
