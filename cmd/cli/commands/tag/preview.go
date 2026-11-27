package tag

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func previewTag(g git.Git, input tagInputPayload) (tagOutputPayload, error) {
	result, err := resolveTag(g, input)
	if err != nil {
		return result, err
	}
	if err := g.CheckTag(input.gr, result.tag, git.BranchName(result.branch)); err != nil {
		return result, err
	}
	if !input.pull && !input.push {
		return result, nil
	}
	remotes, err := g.ListRemotes(input.gr)
	if err != nil {
		return result, fmt.Errorf("list remotes: %w", err)
	}
	if input.pull && !slices.Contains(remotes, git.Remote("origin")) {
		return result, errors.New("pre-pull requires the origin remote")
	}
	if input.push {
		remote, err := pushRemote(remotes)
		if err != nil {
			return result, err
		}
		result.remote = string(remote)
	}
	return result, nil
}

func (t *tagCmd) preview(cmd *cobra.Command, repos []config.LiveRepo, current, skipPush, pull bool) error {
	_, args := runnerArgs(t.g, repos, t.cm.TagPattern, current, skipPush, pull)
	results := runner.Run(func(input tagInputPayload) (tagOutputPayload, error) {
		return previewTag(t.g, input)
	}, args)

	out := cmd.OutOrStdout()
	width, _ := ui.TerminalSize(out)
	report := formatTagPreview(lipgloss.NewRenderer(out), width, repos, results, current, skipPush, pull)
	_, err := io.WriteString(out, report+"\n")
	return err
}

func commonPreviewValue(results []runner.Result[tagOutputPayload], field func(tagOutputPayload) string) (value string, varies bool) {
	for _, result := range results {
		candidate := field(result.Res)
		if candidate == "" {
			continue
		}
		if value != "" && value != candidate {
			return "", true
		}
		value = candidate
	}
	return value, false
}

func formatTagPreview(renderer *lipgloss.Renderer, width int, repos []config.LiveRepo, results []runner.Result[tagOutputPayload], current, skipPush, pull bool) string {
	tag, showTag := commonPreviewValue(results, func(result tagOutputPayload) string { return result.tag })
	branch, showBranch := commonPreviewValue(results, func(result tagOutputPayload) string { return result.branch })
	remote, showRemote := commonPreviewValue(results, func(result tagOutputPayload) string { return result.remote })

	switch {
	case showTag:
		tag = "per repository"
	case tag == "":
		tag = "unresolved"
	}
	if branch == "" {
		branch = "configured refs"
		if current {
			branch = "current branches"
		}
	}

	switch {
	case skipPush:
		remote = "off"
	case showRemote:
		remote = "per repository"
	case remote == "":
		remote = "on (unresolved)"
	}

	prePull := "off"
	if pull {
		prePull = "origin"
	}
	title := ui.Title(renderer)
	header := []string{
		title.Render("Tag preview"),
		"Tag       " + ui.SingleLine(tag),
		"Source    " + ui.SingleLine(branch),
		"Push      " + ui.SingleLine(remote),
		"Pre-pull  " + prePull,
	}
	for i, line := range header {
		header[i] = ui.WrapWords(line, width)
	}

	var repoColumnWidth int
	if showBranch {
		for i, result := range results {
			if result.Res.branch != "" {
				nameWidth := lipgloss.Width(ui.SingleLine(repos[i].Name))
				branchWidth := lipgloss.Width(ui.SingleLine(result.Res.branch))
				if width == 0 || nameWidth+4+branchWidth <= width {
					repoColumnWidth = max(repoColumnWidth, nameWidth)
				}
			}
		}
	}

	var eligible, unavailable []string
	repoStyle := renderer.NewStyle().Bold(true)
	for i, result := range results {
		var details []string
		hasBranch := showBranch && result.Res.branch != ""
		if hasBranch {
			details = append(details, ui.SingleLine(result.Res.branch))
		}
		if showTag && result.Res.tag != "" {
			details = append(details, "tag "+ui.SingleLine(result.Res.tag))
		}
		if showRemote && result.Res.remote != "" {
			details = append(details, "→ "+ui.SingleLine(result.Res.remote))
		}

		name := repoStyle.Render(ui.SingleLine(repos[i].Name))
		marker := "✓ "
		if result.Error != nil {
			marker = "× "
			details = append(details, ui.ErrorDetail(result.Error))
		}
		row := marker + name
		if len(details) > 0 {
			detail := strings.Join(details, " · ")
			row += "  " + detail
			if hasBranch {
				padding := max(0, repoColumnWidth-lipgloss.Width(name))
				if width == 0 || lipgloss.Width(row)+padding <= width {
					row = marker + name + strings.Repeat(" ", padding+2) + detail
				}
			}
		}
		if width > 2 && lipgloss.Width(row) > width {
			row = strings.ReplaceAll(ui.WrapWords(row, width-2), "\n", "\n  ")
		} else if width > 0 && lipgloss.Width(row) > width {
			row = ui.WrapWords(row, width)
		}

		if result.Error != nil {
			unavailable = append(unavailable, row)
		} else {
			eligible = append(eligible, row)
		}
	}

	sections := []string{strings.Join(header, "\n")}
	for _, group := range []struct {
		label string
		rows  []string
	}{{"Eligible", eligible}, {"Unavailable", unavailable}} {
		if len(group.rows) > 0 {
			heading := title.Render(fmt.Sprintf("%s · %d", group.label, len(group.rows)))
			sections = append(sections, ui.WrapWords(heading, width)+"\n"+strings.Join(group.rows, "\n"))
		}
	}
	return strings.Join(sections, "\n\n")
}
