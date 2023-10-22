package start

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/shell"
	"github.com/AzraelSec/godotenv"
)

type startInputPayload struct {
	path string
	cmd  string
	out  io.Writer
}

type startOutputPayload struct {
	Pid     int
	RetCode int
}

func processRun(ctx context.Context, envFilenames []string, payload startInputPayload) (startOutputPayload, error) {
	if !dir.DirExists(payload.path) {
		return startOutputPayload{}, config.RepoNotFoundErr
	}

	denv, err := godotenv.ReadFrom(payload.path, false, envFilenames...)
	if err != nil {
		denv = map[string]string{}
	}

	res := startOutputPayload{
		Pid:     -1,
		RetCode: -1,
	}

	startProcess := shell.NewSyncShell(shell.ShellOps{
		ExecPath: payload.path,
		Cmd:      payload.cmd,
		Ctx:      ctx,
		Env:      denv,
	})

	rc, err := startProcess.Start(payload.out)
	if err != nil {
		return res, err
	}

	res.RetCode = rc
	res.Pid = startProcess.Pid

	return res, nil
}
