package task

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func awaitResult(t *testing.T, completion <-chan Result) Result {
	t.Helper()
	select {
	case result := <-completion:
		if _, ok := <-completion; ok {
			t.Fatal("completion was not closed")
		}
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("task did not terminate")
		return Result{}
	}
}

func waitTaskCondition(t *testing.T, condition func() bool) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for !condition() {
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatal("condition did not become true")
		}
	}
}

func TestRunContextCancellationEscalatesProcessGroup(t *testing.T) {
	tm := NewTaskManager(nil)
	tm.stopTimeout = 30 * time.Millisecond
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completion, err := tm.RunContext(ctx, &Task{Name: "cancel", ExecDir: dir,
		Command: `trap '' INT; sleep 60 & child=$!; echo "$child" > child.pid; wait "$child"`})
	if err != nil {
		t.Fatal(err)
	}
	waitTaskCondition(t, func() bool { _, err := os.Stat(filepath.Join(dir, "child.pid")); return err == nil })
	tm.mu.RLock()
	runner := tm.runners["cancel"]
	tm.mu.RUnlock()
	pid := runner.PID()
	raw, err := os.ReadFile(filepath.Join(dir, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	child, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	result := awaitResult(t, completion)
	if result.State != Failed || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("task leader was not reaped: %v", err)
	}
	// The host init may briefly retain an orphan zombie, but no descendant
	// may remain executing after cancellation completes.
	waitTaskCondition(t, func() bool {
		raw, err := os.ReadFile("/proc/" + strconv.Itoa(child) + "/stat")
		if os.IsNotExist(err) {
			return true
		}
		if err != nil {
			return false
		}
		_, tail, _ := strings.Cut(string(raw), ") ")
		return strings.HasPrefix(tail, "Z ")
	})
}

func TestRunContextDeadlineAndPreCanceledAdmission(t *testing.T) {
	tm := NewTaskManager(nil)
	tm.stopTimeout = 30 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	completion, err := tm.RunContext(ctx, &Task{Name: "timeout", Command: "sleep 60"})
	if err != nil {
		t.Fatal(err)
	}
	result := awaitResult(t, completion)
	if !errors.Is(result.Err, context.DeadlineExceeded) || result.State != Failed {
		t.Fatalf("unexpected result: %+v", result)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := tm.RunContext(canceled, &Task{Name: "never", Command: "sleep 60"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("wanted cancellation, got %v", err)
	}
	if _, ok := tm.Get("never"); ok {
		t.Fatal("pre-canceled task was admitted")
	}
}

func TestCancelAllWaitsAndAllowsCleanup(t *testing.T) {
	tm := NewTaskManager(nil)
	tm.stopTimeout = 30 * time.Millisecond
	completion, err := tm.Run(&Task{Name: "ordinary", Command: "sleep 60"})
	if err != nil {
		t.Fatal(err)
	}
	tm.CancelAllAndWait()
	if res := awaitResult(t, completion); !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("unexpected result: %+v", res)
	}
	completion, err = tm.Run(&Task{Name: "cleanup", Command: "exit 0"})
	if err != nil {
		t.Fatal(err)
	}
	if res := awaitResult(t, completion); res.State != Finished {
		t.Fatalf("cleanup failed: %+v", res)
	}
}

func TestLaunchFailureHasCompletionResult(t *testing.T) {
	tm := NewTaskManager(nil)
	completion, err := tm.Run(&Task{Name: "invalid", ExecDir: filepath.Join(t.TempDir(), "missing"), Command: "echo never"})
	if err == nil {
		t.Fatal("expected launch error")
	}
	result := awaitResult(t, completion)
	if result.State != Failed || result.ExitCode != -1 || result.Err == nil {
		t.Fatalf("unexpected result: %+v", result)
	}
}
