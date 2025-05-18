package tag

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/ui"
	"github.com/spf13/cobra"
)

type tagCmd struct {
	cm  *config.ConfigManager
	g   git.Git
	err error
}

type tagOutputPayload struct {
	branch string
	remote string
	tag    string
}

type tagInputPayload struct {
	gr            git.Repo
	sharedContext sharedContext
	pattern       string
	currentBranch bool
	pull          bool
	push          bool
}

func createTag(g git.Git, input tagInputPayload) (tagOutputPayload, error) {
	res := tagOutputPayload{}
	if !dir.DirExists(input.gr.Path) {
		return res, config.ErrRepoNotFound
	}

	tmpl, err := template.New("tag").Parse(input.pattern)
	if err != nil {
		return res, err
	}

	res.branch = string(input.gr.Refs)
	if input.currentBranch {
		br, err := g.CurrentBranch(input.gr)
		if err != nil {
			return res, fmt.Errorf("impossible to get the current branch - %w", err)
		}
		res.branch = string(br)
	}

	if input.pull {
		err := g.PullBranch(input.gr, git.BranchName(res.branch), false)
		if err != nil {
			return res, fmt.Errorf("impossible to pull the %s branch - %w", res.branch, err)
		}
	}

	var hydrated bytes.Buffer
	if err := tmpl.Execute(&hydrated, input.sharedContext.NewTagContext(res.branch)); err != nil {
		return res, err
	}

	res.tag = hydrated.String()

	if err := g.CreateLightweightTag(input.gr, res.tag, git.BranchName(res.branch)); err != nil {
		return res, fmt.Errorf("impossible to create the lightweight tag on branch %s - %w", res.branch, err)
	}

	if !input.push {
		return res, nil
	}

	remotes, err := g.ListRemotes(input.gr)
	if err != nil {
		return res, fmt.Errorf("impossible to list remotes - %w", err)
	}
	if len(remotes) != 1 {
		return res, errors.New("impossible to push tag on repos with multiple remotes")
	}
	res.remote = string(remotes[0])

	if err := g.PushTag(input.gr, git.Tag(res.tag), remotes[0]); err != nil {
		return res, fmt.Errorf("impossible to push tag %s on %s - %w", res.tag, string(remotes[0]), err)
	}
	return res, nil
}

func New(dm *dependency.Manager) *tagCmd {
	tcmd := &tagCmd{}
	tcmd.g, tcmd.err = dm.GetGit()
	if tcmd.err != nil {
		return tcmd
	}
	tcmd.cm, tcmd.err = dm.GetConfigManager()
	return tcmd
}

type tagRunnerFunc func(tagInputPayload) (tagOutputPayload, error)

func runnerArgs(g git.Git, repos []config.LiveRepo, tagPattern string, useCurrent, skipPush, pullBefore bool) (tagRunnerFunc, []tagInputPayload) {
	now := time.Now()

	updateArgs := make([]tagInputPayload, 0, len(repos))
	updateFn := func(args tagInputPayload) (tagOutputPayload, error) {
		return createTag(g, args)
	}

	for _, repo := range repos {
		updateArgs = append(updateArgs, tagInputPayload{
			gr: repo.GitConfig,
			sharedContext: sharedContext{
				Now: now,
			},
			pattern:       tagPattern,
			currentBranch: useCurrent,
			pull:          pullBefore,
			push:          !skipPush,
		})
	}

	return updateFn, updateArgs
}

func (t *tagCmd) Command() *cobra.Command {
	var disableTui, useCurrent, skipPush, pullBefore *bool

	cmd := &cobra.Command{
		Use:     "tag",
		Aliases: []string{"yeet"},
		Short:   "Create and push a lightweight tag for each repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			if t.err != nil {
				return t.err
			}

			if t.cm.TagPattern == "" {
				return errors.New("you need to define `tag_pattern` in your config to use this command")
			}

			repos := make([]config.LiveRepo, 0)
			for _, repo := range t.cm.Repos {
				if repo.Config.ExcludeTag {
					continue
				}
				repos = append(repos, repo)
			}

			if len(repos) == 0 {
				fmt.Println(ui.YELLOW.Render("no repos available for tagging"))
				return nil
			}

			isYeet := cmd.CalledAs() == "yeet"
			if !*disableTui {
				return newTui(t.g, repos, t.cm.TagPattern, *useCurrent, *skipPush, *pullBefore, isYeet).run()
			}
			newCli(t.g, repos, t.cm.TagPattern, *useCurrent, *skipPush, *pullBefore, isYeet).run()
			return nil
		},
	}

	disableTui = cmd.Flags().Bool("disableTui", false, "Disable TUI")
	useCurrent = cmd.Flags().BoolP("current", "c", false, "Use current branch instead of the main branch")
	skipPush = cmd.Flags().BoolP("skip-push", "s", false, "Avoid pushing the tags on remote")
	pullBefore = cmd.Flags().BoolP("pre-pull", "p", true, "Pull the last version of the target branch before tagging")

	return cmd
}
