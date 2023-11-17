package update

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/ui"
)

type cli struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	repos      []config.LiveRepo
	updateFn   updateRunnerFunc
	updateArgs []updateInputPayload

	withOutput bool
}

func newCli(repos []config.LiveRepo, output bool) *cli {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)

	updateFn, updateArgs := runnerArgs(ctx, repos, output)
	return &cli{ctx, cancelCtx,  repos, updateFn, updateArgs, output}
}

func (c *cli) run() {
	defer c.cancelCtx()
	results := runner.Run(c.updateFn, c.updateArgs)
	if !c.withOutput {
		c.printResult(results)
	}
}

// todo: is it worth to put effort in not duplicating this?
func (c *cli) printResult(results []runner.Result[updateOutputPayload]) {
	var buff bytes.Buffer

	for idx, repo := range c.repos {
		res := results[idx]

		if idx != 0 {
			buff.WriteString("\n")
		}

		if res.Error != nil {
			buff.WriteString("⛔ ")
			buff.WriteString(fmt.Sprintf("%s: %s", repo.Name, ui.RED.Render(res.Error.Error())))
			continue
		}

		if res.Res.Ignored {
			buff.WriteString("🫥 ")
			buff.WriteString(ui.STRIKE.Render(fmt.Sprintf("%s: ignored", repo.Name)))
			continue
		}

		buff.WriteString(fmt.Sprintf("✅ %s", repo.Name))
		buff.WriteString(fmt.Sprintf(" [%s] ", ui.YELLOW.Render(res.Res.UpdaterTag)))

		if res.Res.Inferred {
			buff.WriteString(ui.YELLOW.Render("(inferred)"))
		} else {
			buff.WriteString(ui.YELLOW.Render("(configured)"))
		}

		buff.WriteString(fmt.Sprintf(": %s", ui.GREEN.Render("repo updated successfully!")))
	}

	fmt.Print(buff.String())
}
