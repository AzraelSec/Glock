package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const SHELL_PROGRAMM = "/bin/sh"

type syncShell struct {
	process *exec.Cmd
	ctx     context.Context
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
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// A descendant can keep stderr open after the updater has exited.
	process.WaitDelay = 2 * time.Second
	process.Cancel = func() error {
		err := syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}

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
		ctx:     ctx,
		Pid:     -1,
	}
}

func (ss *syncShell) Start(w io.Writer) (int, error) {
	ss.process.Stdout = w
	ss.process.Stderr = w
	var stderr errorOutput
	if w == nil {
		ss.process.Stderr = &stderr
	}

	if err := ss.process.Start(); err != nil {
		return -1, err
	}
	ss.Pid = ss.process.Process.Pid
	err := ss.process.Wait()
	if err != nil {
		if ctxErr := ss.ctx.Err(); ctxErr != nil {
			err = ctxErr
		} else if detail := strings.TrimSpace(stderr.String()); detail != "" {
			err = fmt.Errorf("%w: %s", err, detail)
		}
	}
	return ss.process.ProcessState.ExitCode(), err
}

// errorOutput retains the start of stderr while draining the entire stream.
type errorOutput struct{ bytes.Buffer }

func (b *errorOutput) Write(p []byte) (int, error) {
	const limit = 4096
	size := min(len(p), limit-b.Len())
	_, _ = b.Buffer.Write(p[:size])
	return len(p), nil
}
