package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestBatchWritesPlainResultsInRepositoryOrder(t *testing.T) {
	release := make(chan struct{})
	batch := Batch{
		Title: "Dependency updates",
		Tasks: []Task{
			{Name: "api", Run: func() Result {
				<-release
				return Result{Label: "Updated"}
			}},
			{Name: "web", Run: func() Result {
				close(release)
				return Result{Outcome: Failure, Label: "Failed", Detail: "first line\nsecond line"}
			}},
		},
	}
	var out bytes.Buffer
	if err := batch.Run(t.Context(), strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}

	want := "Dependency updates\n1 succeeded · 1 failed\n\n✓ api  Updated\n× web  Failed · first line second line\n"
	if out.String() != want {
		t.Fatalf("unexpected report:\n%s", out.String())
	}
}

func TestBatchResizeAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out bytes.Buffer
	m := newBatchModel(ctx, Batch{
		Title: "Dependency updates", Activity: "Updating repositories", Cancel: cancel,
		Tasks: []Task{{Name: "long-repository-name"}, {Name: "second-repository"}},
	}, lipgloss.NewRenderer(&out), 80, 24)

	m.Update(tea.WindowSizeMsg{Width: 16, Height: 5})
	if width, height := lipgloss.Size(m.View()); width >= 16 || height > 5 {
		t.Fatalf("view measures %d x %d, terminal measures 16 x 5", width, height)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if ctx.Err() == nil || !m.stopping {
		t.Fatal("interrupt did not cancel the workers")
	}
	_, cmd := m.Update(taskDoneMsg{index: 1, result: Result{Outcome: Canceled, Label: "Canceled"}})
	if cmd != nil {
		t.Fatal("batch quit before all worker results arrived")
	}
	_, cmd = m.Update(taskDoneMsg{index: 0, result: Result{Label: "Updated"}})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("batch did not quit after all results arrived")
	}
	if summary := m.report(); !strings.Contains(summary, "1 canceled") || strings.Contains(summary, "1 failed") {
		t.Fatalf("wrong cancellation summary:\n%s", summary)
	}
}

func TestBatchLetsYouInspectFailuresAfterCompletion(t *testing.T) {
	m := newBatchModel(t.Context(), Batch{
		Title: "Dependency updates", ExpandFailures: true,
		Tasks: []Task{{Name: "api"}, {Name: "web"}},
	}, lipgloss.NewRenderer(&bytes.Buffer{}), 80, 20)

	m.Update(taskDoneMsg{index: 1, result: Result{Outcome: Failure, Label: "Failed", Detail: "missing package"}})
	_, cmd := m.Update(taskDoneMsg{index: 0, result: Result{Label: "Updated"}})
	if cmd != nil || strings.Contains(m.View(), "missing package") {
		t.Fatal("failure should be collapsed in the completed inspection screen")
	}
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	m.Update(enter)
	if !strings.Contains(m.View(), "missing package") {
		t.Fatal("enter should reveal failure details")
	}
	m.Update(enter)
	if strings.Contains(m.View(), "missing package") {
		t.Fatal("enter should also collapse failure details")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil || !strings.Contains(m.report(), "missing package") {
		t.Fatal("closing inspection should keep the details in the final report")
	}
}

func TestGroupReportPutsAttentionBeforeClean(t *testing.T) {
	batch := Batch{
		Title: "Repository status", Groups: []string{"Needs attention", "Clean and tracked"},
		Tasks: []Task{
			{Name: "api", Run: func() Result { return Result{Label: "Clean", Group: "Clean and tracked"} }},
			{Name: "web", Run: func() Result { return Result{Outcome: Attention, Label: "Dirty", Group: "Needs attention"} }},
		},
	}
	var out bytes.Buffer
	if err := batch.Run(t.Context(), nil, &out); err != nil {
		t.Fatal(err)
	}
	want := "Repository status\n2 repositories checked\n\nNeeds attention · 1\n! web  Dirty\n\nClean and tracked · 1\n✓ api  Clean\n"
	if out.String() != want {
		t.Fatalf("unexpected report:\n%s", out.String())
	}
}
