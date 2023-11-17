package ui

import "github.com/charmbracelet/bubbles/spinner"

func NewSpinner() spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.Points))
}
