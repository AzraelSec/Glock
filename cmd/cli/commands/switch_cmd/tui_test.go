package switchcmd

import (
	"io"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestBranchPickerAppliesLatestFilterBeforeSwitching(t *testing.T) {
	p := newBranchPicker(nil, lipgloss.NewRenderer(io.Discard), false)
	p.resize(40, 10)
	p.Update(branchesLoaded{names: []string{"main", "preview"}})
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	_, oldQuery := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	_, newQuery := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("review")})
	p.Update(filterResult(t, newQuery))
	p.Update(filterResult(t, oldQuery))

	enter := tea.KeyMsg{Type: tea.KeyEnter}
	p.Update(enter)
	if p.target != "" || p.list.FilterState() != list.FilterApplied {
		t.Fatalf("Enter while editing should only apply filter, got state %v and target %q", p.list.FilterState(), p.target)
	}
	p.Update(enter)
	if p.review == nil || p.review.target != "preview" || p.target != "" {
		t.Fatalf("expected review of preview, got %#v and %q", p.review, p.target)
	}
	p.Update(enter)
	if p.target != "" {
		t.Fatal("confirmed a review before the repository results arrived")
	}
	p.Update(impactLoaded{p.review.revision, []branchImpact{{kind: eligible}}})
	p.Update(enter)
	if p.target != "preview" {
		t.Fatalf("selected %q, want preview", p.target)
	}
}

func TestBranchPickerDoesNotSwitchWithoutAMatchingBranch(t *testing.T) {
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	p := newBranchPicker(nil, lipgloss.NewRenderer(io.Discard), false)
	p.resize(40, 10)
	p.Update(branchesLoaded{})
	p.Update(enter)
	if p.target != "" {
		t.Fatalf("empty list selected %q", p.target)
	}

	p.Update(branchesLoaded{names: []string{"main"}})
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	_, query := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("missing")})
	p.Update(enter)
	p.Update(filterResult(t, query))
	p.Update(enter)
	p.Update(enter)
	if p.target != "" || p.list.FilterState() != list.Filtering {
		t.Fatalf("nonmatching filter was dismissed, got state %v and target %q", p.list.FilterState(), p.target)
	}
}

func TestBranchPickerEnablesPagingAfterResize(t *testing.T) {
	p := newBranchPicker(nil, lipgloss.NewRenderer(io.Discard), false)
	p.resize(80, 30)
	p.Update(branchesLoaded{names: []string{"a", "b", "c", "d", "e", "f"}})
	if p.list.Paginator.TotalPages != 1 {
		t.Fatal("branches should fit in one page at the initial size")
	}
	p.Update(tea.WindowSizeMsg{Width: 40, Height: 6})
	p.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if p.list.Paginator.Page == 0 {
		t.Fatal("page down did not move the selection after a resize")
	}
}

func TestBranchPickerCanReturnFromReviewAndIgnoresStaleImpact(t *testing.T) {
	p := newBranchPicker(nil, lipgloss.NewRenderer(io.Discard), false)
	p.resize(60, 15)
	p.Update(branchesLoaded{names: []string{"main"}})
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	p.Update(enter)
	first := p.review.revision
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.review != nil || p.target != "" {
		t.Fatal("escape should return to the branch list")
	}
	p.Update(enter)
	p.Update(impactLoaded{first, []branchImpact{{kind: eligible}}})
	p.Update(impactLoaded{p.review.revision, []branchImpact{{kind: alreadyOnBranch}}})
	p.Update(enter)
	if p.target != "" || p.review.canSwitch() {
		t.Fatal("review without eligible repositories must not confirm")
	}
}

func filterResult(t *testing.T, cmd tea.Cmd) filterMatches {
	t.Helper()
	for cmd != nil {
		switch msg := cmd().(type) {
		case filterMatches:
			return msg
		case tea.BatchMsg:
			// Bubbles 0.16 sends the filter query after the text-input command.
			cmd = msg[len(msg)-1]
		default:
			cmd = nil
		}
	}
	t.Fatal("expected an asynchronous filter result")
	return filterMatches{}
}
