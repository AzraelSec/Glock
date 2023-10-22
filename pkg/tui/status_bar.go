package tui

import (
	"github.com/charmbracelet/lipgloss"
)

type StatusBarTab struct {
	Label  string
	Active bool
}

type StatusBar struct {
	Tabs        []StatusBarTab
	HistoryMode bool
	Completed   bool
	Height      int
	Width       int
}

func (s StatusBar) Render() string {
	var (
		baseStyle       = lipgloss.NewStyle().Height(s.Height)
		glockTitleStyle = lipgloss.NewStyle().
				Inherit(baseStyle).
				Foreground(lipgloss.Color("#FFFDF5")).
				Padding(0, 1).Background(lipgloss.Color("#6124DF"))

		tabStyle = lipgloss.NewStyle().
				Inherit(baseStyle).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#000000"))

		activeTabStyle = lipgloss.NewStyle().
				Inherit(baseStyle).
				Background(lipgloss.Color("#960019"))
	)

	titleStr := "Glock"
	if s.HistoryMode {
		titleStr = "Glock 🔍"
	}
	if s.Completed {
		titleStr = "Glock [EXITED]"
	}
	glockTitle := glockTitleStyle.Render(titleStr)
	glockTitleW := lipgloss.Width(glockTitle)

	main := tabStyle.Width(s.Width - glockTitleW).Render("")

	if len(s.Tabs) != 0 {
		blockSize := (s.Width - glockTitleW) / len(s.Tabs)

		tabs := make([]string, 0, len(s.Tabs))
		for _, tab := range s.Tabs {
			style := tabStyle
			if tab.Active {
				style = activeTabStyle
			}
			tabs = append(tabs, style.Copy().Width(blockSize).Render(tab.Label))
		}

		main = lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
	}

	return lipgloss.JoinHorizontal(lipgloss.Bottom, glockTitle, main)
}
