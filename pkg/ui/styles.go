package ui

import "github.com/charmbracelet/lipgloss"

var (
	YELLOW = lipgloss.NewStyle().Foreground(lipgloss.Color("#fcc00a"))
	RED    = lipgloss.NewStyle().Foreground(lipgloss.Color("#eb4034"))
	GREEN  = lipgloss.NewStyle().Foreground(lipgloss.Color("#04ba3d"))
)

var (
	STRIKE = lipgloss.NewStyle().Strikethrough(true)
)
