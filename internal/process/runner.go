package process

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

const shellPath = "/bin/sh"

// drainGracePeriod bounds how long the wait goroutine lets the output
// scanners drain buffered output after the process exits. A descendant
// that inherited the pipe FDs (e.g. a daemonized dev server) can hold
// them open forever; without this deadline the drain would block
// forever and Stop/shutdown would hang.
const drainGracePeriod = 2 * time.Second

// OSProcess wraps an exec.Cmd with process-group signalling, async
// lifecycle management, and interleaved stdout/stderr capture.
type OSProcess struct {
	cmd      *exec.Cmd
	pid      int
	output   Output
	done     chan struct{}
	exitCode int
	signal   int
	exitErr  error
	mu       sync.Mutex
}

// NewOSProcess creates, starts, and returns an OSProcess.
//
// The command is executed via "sh -c" so that shell features (pipes,
// redirection, etc.) work out of the box.  The process is placed in its
// own process group (Setpgid) so that signals can be sent to the
// entire tree with syscall.Kill(-pid, sig).
//
// TODO: Windows does not support process groups or syscall.Kill; we
// will need a platform-specific implementation later.
func NewOSProcess(command, execDir string, env []string, output Output) (*OSProcess, error) {
	// Pre-flight checks so that fork/exec failures give a useful message.
	if _, err := os.Stat(shellPath); err != nil {
		return nil, fmt.Errorf("shell %s not found: %w", shellPath, err)
	}
	if execDir != "" {
		info, err := os.Stat(execDir)
		if err != nil {
			return nil, fmt.Errorf("exec dir %q does not exist: %w", execDir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("exec dir %q is not a directory", execDir)
		}
	}

	cmd := exec.Command(shellPath, "-c", command)
	if execDir != "" {
		cmd.Dir = execDir
	}
	cmd.Env = MergeEnvironment(env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Own the read ends ourselves: exec.Cmd.Wait closes StdoutPipe/StderrPipe
	// immediately on process exit, which can discard output before scanners run.
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		stdout.Close()
		stdoutWriter.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stdoutWriter.Close()
		stderr.Close()
		stderrWriter.Close()
		return nil, fmt.Errorf("start command: %w", err)
	}
	// Only the child holds writers now, so EOF reflects its descriptor lifetime.
	stdoutWriter.Close()
	stderrWriter.Close()

	op := &OSProcess{
		cmd:    cmd,
		pid:    cmd.Process.Pid,
		output: output,
		done:   make(chan struct{}),
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		op.scanStream(stdout, "stdout")
	}()
	go func() {
		defer wg.Done()
		op.scanStream(stderr, "stderr")
	}()

	// Reap the leader immediately, then let the scanners drain their own
	// pipes. Descendants can retain writers indefinitely, so the grace deadline
	// closes the readers and waits for their goroutines before reporting exit.
	go func() {
		waitErr := cmd.Wait()

		drained := make(chan struct{})
		go func() {
			wg.Wait()
			close(drained)
		}()
		select {
		case <-drained:
		case <-time.After(drainGracePeriod):
			if reporter, ok := op.output.(interface{ ReportTruncation() }); ok {
				reporter.ReportTruncation()
			}
			stdout.Close()
			stderr.Close()
			<-drained
		}
		stdout.Close()
		stderr.Close()

		op.mu.Lock()
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				op.exitCode = exitErr.ExitCode()
				if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
					op.signal = int(ws.Signal())
				}
			} else {
				op.exitCode = -1
			}
			op.exitErr = waitErr
		} else {
			op.exitCode = 0
		}
		op.mu.Unlock()
		close(op.done)
	}()

	return op, nil
}

// scanStream reads lines from r and appends them to op.output.
// Long lines are split into bounded fragments so pipe draining continues.
// Read failures are surfaced as marker fragments.
func (op *OSProcess) scanStream(r io.Reader, stream string) {
	reader := bufio.NewReaderSize(r, 32*1024)
	appendPart := func(part []byte, partial, newline bool) {
		line := string(part)
		if out, ok := op.output.(interface {
			AppendStream(string, string, bool, bool)
		}); ok {
			out.AppendStream(stream, line, partial, newline)
		} else {
			op.output.Append(line)
		}
	}
	continuing := false
	for {
		part, err := reader.ReadSlice('\n')
		if len(part) > 0 {
			newline := part[len(part)-1] == '\n'
			if newline {
				part = part[:len(part)-1]
			}
			partial := !newline && err == bufio.ErrBufferFull
			appendPart(part, partial, newline)
			continuing = partial
		} else if err == io.EOF && continuing {
			appendPart(nil, false, false)
			continuing = false
		}
		if err == nil || errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
			appendPart([]byte(fmt.Sprintf("[glock] output read error: %v", err)), false, true)
		}
		return
	}
}

// PID returns the OS process ID. It is 0 before the process is started.
func (op *OSProcess) PID() int {
	return op.pid
}

// SendSignal delivers sig to the process group (negative PID).
// Returns an error if the process has already exited, shrinking the
// PID/PGID-reuse window in which a late signal could hit an unrelated
// process group. (The race cannot be eliminated entirely: the kernel
// may reassign the PGID between the check and the kill.)
func (op *OSProcess) SendSignal(sig syscall.Signal) error {
	if op.pid == 0 {
		return fmt.Errorf("process has no PID")
	}
	select {
	case <-op.done:
		return fmt.Errorf("process %d has already exited", op.pid)
	default:
	}
	return syscall.Kill(-op.pid, sig)
}

// WaitChan returns a channel that is closed when the OS process exits.
func (op *OSProcess) WaitChan() <-chan struct{} {
	return op.done
}

// ExitCode returns the OS exit code. It is safe to call concurrently
// with the death goroutine. For signal-terminated processes it is -1.
func (op *OSProcess) ExitCode() int {
	op.mu.Lock()
	defer op.mu.Unlock()
	return op.exitCode
}

// Signal returns the signal that terminated the process (e.g.
// syscall.SIGINT), or 0 if the process exited normally. It is safe to
// call concurrently with the death goroutine.
func (op *OSProcess) Signal() int {
	op.mu.Lock()
	defer op.mu.Unlock()
	return op.signal
}

// MergeEnvironment returns unique entries with host values taking precedence.
func MergeEnvironment(files []string) []string {
	values := make(map[string]string)
	for _, entry := range append(slices.Clone(files), os.Environ()...) {
		if key, _, ok := strings.Cut(entry, "="); ok {
			values[key] = entry
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, values[key])
	}
	return out
}

// Terminate gracefully signals the process group, escalates, and reaps it.
// The done check makes repeated calls safe after completion.
func (op *OSProcess) Terminate(timeout time.Duration) {
	select {
	case <-op.done:
		return
	default:
	}
	_ = op.SendSignal(syscall.SIGINT)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-op.done:
	case <-timer.C:
		_ = op.SendSignal(syscall.SIGKILL)
		<-op.done
	}
}
