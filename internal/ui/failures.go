package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

var (
	toggleFailure   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details"))
	previousFailure = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑", "prev failure"))
	nextFailure     = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓", "next failure"))
	browseFailures  = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "failures"))
	closeResults    = key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "close"))
)

func (m *batchModel) failureIndices() []int {
	var indices []int
	for i, result := range m.results {
		if result != nil && result.Outcome == Failure {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m *batchModel) handleFailureKey(msg tea.KeyMsg) bool {
	switch {
	case key.Matches(msg, toggleFailure):
		if m.failureCursor < 0 {
			return false
		}
		m.expanded[m.failureCursor] = !m.expanded[m.failureCursor]
	case key.Matches(msg, previousFailure, nextFailure):
		if m.failureCursor < 0 {
			return false
		}
		failures := m.failureIndices()
		pos := slices.Index(failures, m.failureCursor)
		if key.Matches(msg, nextFailure) {
			pos++
		} else {
			pos--
		}
		m.failureCursor = failures[max(0, min(len(failures)-1, pos))]
	default:
		return false
	}

	m.refreshRows()
	m.revealFailure(key.Matches(msg, toggleFailure))
	return true
}

func (m *batchModel) revealFailure(alignTop bool) {
	line, ok := m.failureLines[m.failureCursor]
	if ok && (alignTop || line < m.viewport.YOffset || line >= m.viewport.YOffset+m.viewport.Height) {
		m.viewport.SetYOffset(line)
	}
}

func (m *batchModel) updateRows() string {
	width := m.viewport.Width
	m.failureLines = make(map[int]int)
	var lines []string
	add := func(text string) { lines = append(lines, strings.Split(text, "\n")...) }
	for _, group := range []struct {
		label   string
		include func(*Result) bool
	}{
		{"Running", func(result *Result) bool { return result == nil }},
		{"Failures", func(result *Result) bool { return result != nil && result.Outcome == Failure }},
		{"Finished", func(result *Result) bool { return result != nil && result.Outcome != Failure }},
	} {
		var indices []int
		for i, result := range m.results {
			if group.include(result) {
				indices = append(indices, i)
			}
		}
		if len(indices) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		add(Title(m.renderer).Render(fmt.Sprintf("%s · %d", group.label, len(indices))))
		for _, i := range indices {
			result := m.results[i]
			if result == nil || result.Outcome != Failure {
				add(m.row(i, width, true))
				continue
			}
			m.failureLines[i] = len(lines)
			prefix := "  "
			if i == m.failureCursor {
				prefix = Title(m.renderer).Render("› ")
			}
			marker := "+ "
			if m.expanded[i] {
				marker = "− "
			}
			add(prefix + marker + m.row(i, max(1, width-4), false))
			if m.expanded[i] {
				for line := range strings.SplitSeq(result.Detail, "\n") {
					detail := WrapWords(SingleLine(line), max(1, width-4))
					add("    " + strings.ReplaceAll(detail, "\n", "\n    "))
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}
