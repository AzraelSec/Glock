package ui

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
	"golang.org/x/term"
)

var (
	accent = lipgloss.AdaptiveColor{Light: "#5531A5", Dark: "#C4ACFF"}
	muted  = lipgloss.AdaptiveColor{Light: "#575C67", Dark: "#ADB5C2"}
)

func IsInteractive(in io.Reader, out io.Writer) bool {
	inputWidth, _ := TerminalSize(in)
	outputWidth, _ := TerminalSize(out)
	return inputWidth > 0 && outputWidth > 0 && os.Getenv("TERM") != "dumb"
}

func TerminalSize(stream any) (width, height int) {
	if file, ok := stream.(*os.File); ok && file != nil {
		width, height, _ = term.GetSize(int(file.Fd()))
	}
	return width, height
}

func Title(renderer *lipgloss.Renderer) lipgloss.Style {
	return renderer.NewStyle().Foreground(accent).Bold(true)
}

func Muted(renderer *lipgloss.Renderer) lipgloss.Style {
	return renderer.NewStyle().Foreground(muted)
}

func NewHelp(renderer *lipgloss.Renderer) help.Model {
	h := help.New()
	h.Styles.ShortKey = renderer.NewStyle().Bold(true)
	h.Styles.ShortDesc = Muted(renderer)
	h.Styles.ShortSeparator = h.Styles.ShortDesc
	h.Styles.FullKey = h.Styles.ShortKey
	h.Styles.FullDesc = h.Styles.ShortDesc
	h.Styles.FullSeparator = h.Styles.ShortSeparator
	return h
}

// Fit clips all lines to the terminal and avoids writing into its last column.
func Fit(view string, width, height int) string {
	if width <= 1 || height <= 0 {
		return ""
	}
	lines := strings.Split(view, "\n")
	lines = lines[:min(len(lines), height)]
	for i, line := range lines {
		line = strings.TrimRight(line, " ")
		if columns := width - 1; lipgloss.Width(line) > columns {
			line = truncate.StringWithTail(line, uint(columns), "…")
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// SingleLine keeps subprocess and configuration text from controlling the terminal.
func SingleLine(value string) string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	})
	return strings.Join(fields, " ")
}

func ErrorDetail(err error) string {
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		if stderr := SingleLine(string(exitErr.Stderr)); stderr != "" {
			return SingleLine(err.Error()) + " · " + stderr
		}
	}
	return SingleLine(err.Error())
}

func writeReport(out io.Writer, report string) error {
	_, err := io.WriteString(out, report+"\n")
	return err
}
