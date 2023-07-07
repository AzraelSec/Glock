package shell

import (
	"context"
	"io"
	"os/exec"
)

const SHELL_PROGRAMM = "/bin/sh"

type syncShell struct {
	ctx     *context.Context
	process *exec.Cmd
	Pid     int
}

type ShellOps struct {
	ShellPath string
	ExecPath  string
	Env       []string
	Cmd       string
	Ctx       context.Context
}

func NewSyncShell(ops ShellOps) *syncShell {
	// TODO: make this an os-dependant procedure
	shellPath := SHELL_PROGRAMM
	if ops.ShellPath != "" {
		shellPath = ops.ShellPath
	}
	args := []string{"-c", ops.Cmd}

	var process *exec.Cmd
	if ops.Ctx == nil {
		process = exec.Command(shellPath, args...)
	} else {
		process = exec.CommandContext(ops.Ctx, shellPath, args...)
	}

	if ops.ExecPath != "" {
		process.Dir = ops.ExecPath
	}
	if len(ops.Env) != 0 {
		process.Env = ops.Env
	}

	return &syncShell{
		ctx:     &ops.Ctx,
		process: process,
		Pid: -1,
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
	return ss.process.ProcessState.ExitCode(), error
}
