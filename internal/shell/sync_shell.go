package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

const SHELL_PROGRAMM = "/bin/sh"

type syncShell struct {
	process *exec.Cmd
	Pid     int
}

type ShellOps struct {
	ShellPath string
	ExecPath  string
	Env       map[string]string
	Cmd       string
}

func NewSyncShell(ctx context.Context, ops ShellOps) *syncShell {
	// TODO: make this an os-dependant procedure
	shellPath := SHELL_PROGRAMM
	if ops.ShellPath != "" {
		shellPath = ops.ShellPath
	}

	process := exec.CommandContext(ctx, shellPath, "-c", ops.Cmd)

	if ops.ExecPath != "" {
		process.Dir = ops.ExecPath
	}
	if len(ops.Env) != 0 {
		vars := os.Environ()
		for key, val := range ops.Env {
			vars = append(vars, fmt.Sprintf("%s=%s", key, val))
		}
		process.Env = vars
	}

	return &syncShell{
		process: process,
		Pid:     -1,
	}
}

func (ss *syncShell) Start(w io.Writer) (int, error) {
	ss.process.Stdout = w
	ss.process.Stderr = w
	ss.process.Stdin = nil // NOTE: ignore CTRL-C

	error := ss.process.Start()
	if error != nil {
		return -1, error
	}
	ss.Pid = ss.process.Process.Pid
	error = ss.process.Wait()
	return ss.process.ProcessState.ExitCode(), IgnoreInterrupt(error)
}
