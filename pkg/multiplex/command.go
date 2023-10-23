package multiplex

import "context"

const (
	CMD_READY = iota
	CMD_RUNNING
	CMD_COMPLETE
)

type cmdStatus int

type command struct {
	status cmdStatus
	pid    int

	ctx       context.Context
	cancelCtx context.CancelFunc

	path string
	cmd  string
}

func (c *command) Run() error {
	return nil
}

// note: should this be private?
func NewCommand(ctx context.Context, path, cmd string) command {
	ctx, cancel := context.WithCancel(ctx)
	return command{
		ctx:       ctx,
		cancelCtx: cancel,
		path:      path,
		cmd:       cmd,
		status:    CMD_READY,
		pid:       NO_PID,
	}
}
