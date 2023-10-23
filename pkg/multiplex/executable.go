package multiplex

import (
	"context"
	"io"
)

type executable struct {
	repo     string
	path     string
	status   status
	ctx      context.Context
	out      io.Writer
	cmds     []command
	tearDown []command
}

func NewExecutable(ctx context.Context, repo, path string, out io.Writer, cmds, tearDowns []string) executable {
	cmdObjs := make([]command, 0, len(cmds))
	tearDowsObjs := make([]command, 0, len(tearDowns))
	for _, cmd := range cmds {
		cmdObjs = append(cmdObjs, NewCommand(ctx, path, cmd))
	}
	for _, cmd := range tearDowns {
		tearDowsObjs = append(tearDowsObjs, NewCommand(ctx, path, cmd))
	}
	return executable{
		repo:     repo,
		path:     path,
		status:   READY,
		out:      out,
		cmds:     cmdObjs,
		tearDown: tearDowsObjs,
	}
}
