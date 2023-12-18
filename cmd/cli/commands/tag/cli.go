package tag

import (
	"bytes"
	"fmt"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/AzraelSec/glock/pkg/ui"
)

type cli struct {
	repos   []config.LiveRepo
	tagFn   tagRunnerFunc
	tagArgs []tagInputPayload
  isYeet bool
}

func newCli(g git.Git, repos []config.LiveRepo, tagPattern string, useCurrent, skipPush, pullBefore, isYeet bool) *cli {
	tagFn, tagArgs := runnerArgs(g, repos, tagPattern, useCurrent, skipPush, pullBefore)
	return &cli{repos, tagFn, tagArgs, isYeet}
}

func (c *cli) run() {
	var out bytes.Buffer
  if c.isYeet {
    out.WriteString(YEET_ASCII_IMAGE)
  }
	res := runner.Run(c.tagFn, c.tagArgs)

	for idx, r := range res {
		var buff bytes.Buffer

		if idx != 0 {
			out.WriteString("\n")
		}

		if r.Error != nil {
			buff.WriteString("⛔ ")
			buff.WriteString(fmt.Sprintf("%s: %s", c.repos[idx].Name, ui.RED.Render(r.Error.Error())))
			out.WriteString(buff.String())
			continue
		}

		buff.WriteString(fmt.Sprintf("✅ %s", c.repos[idx].Name))
		if r.Res.remote != "" {
			buff.WriteString(fmt.Sprintf(" [%s => %s] :", ui.ORANGE.Render(r.Res.branch), ui.ORANGE.Render(r.Res.remote)))
		} else {
			buff.WriteString(fmt.Sprintf(" [%s] :", ui.ORANGE.Render(r.Res.branch)))
		}
		buff.WriteString(fmt.Sprintf(" %s", ui.YELLOW.Render(r.Res.tag)))
		out.WriteString(buff.String())
	}

	fmt.Println(out.String())
}
