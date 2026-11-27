package ui

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

func NewSpinner(renderer *lipgloss.Renderer) spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(Title(renderer)))
}
