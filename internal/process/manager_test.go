package process

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortTimeout is used in tests so that Stop/Kill escalations happen
// quickly if something goes wrong.
const shortTimeout = 500 * time.Millisecond

func TestManager_Start(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "echo hello",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Fast-exiting process should transition to Stopped via
	// handleUnexpectedDeath before we can read it.
	time.Sleep(50 * time.Millisecond)

	proc, ok := pm.Get("a")
	if !ok {
		t.Fatal("process not found")
	}
	if proc.State != Stopped {
		t.Fatalf("expected Stopped, got %s", proc.State)
	}
	if proc.PID != 0 {
		t.Fatalf("expected PID 0, got %d", proc.PID)
	}
	if proc.Output.Len() != 1 || proc.Output.Lines()[0] != "hello" {
		t.Fatalf("unexpected output: %v", proc.Output.Lines())
	}
}

func TestManager_Start_AlreadyRunning(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "sleep 10",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "sleep 10",
	}); err == nil {
		t.Fatal("expected error starting already-running process")
	}
}

func TestManager_Stop(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "sleep 10",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	proc, _ := pm.Get("a")
	if proc.State != Running {
		t.Fatalf("expected Running, got %s", proc.State)
	}
	if proc.PID == 0 {
		t.Fatal("expected non-zero PID")
	}

	if err := pm.Stop("a"); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	proc, _ = pm.Get("a")
	if proc.State != Stopped && proc.State != Failed {
		t.Fatalf("expected terminal state, got %s", proc.State)
	}
	if proc.PID != 0 {
		t.Fatalf("expected PID 0 after stop, got %d", proc.PID)
	}
}

func TestManager_Stop_NotRunning(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	// Start and let it finish.
	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "echo done",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := pm.Stop("a"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning, got: %v", err)
	}
}

func TestManager_Stop_AlreadyStopping(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	// Shell ignores SIGINT and loops forever so Stop() stays in
	// Stopping until the timeout/SIGKILL escalation.
	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "trap '' INT; while true; do sleep 1; done",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Give the shell time to set up the signal handler before we send
	// SIGINT; otherwise it may die with the default disposition.
	time.Sleep(150 * time.Millisecond)

	// First Stop blocks until the timeout fires.
	go pm.Stop("a")

	// Give the first Stop time to set Stopping state.
	time.Sleep(50 * time.Millisecond)

	if err := pm.Stop("a"); !errors.Is(err, ErrStopping) {
		t.Fatalf("expected ErrStopping, got: %v", err)
	}
}

func TestManager_Kill(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "sleep 10",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	proc, _ := pm.Get("a")
	if proc.State != Running {
		t.Fatalf("expected Running, got %s", proc.State)
	}

	if err := pm.Kill("a"); err != nil {
		t.Fatalf("kill failed: %v", err)
	}

	// Kill is an intentional termination: Stopped, not Failed.
	proc, _ = pm.Get("a")
	if proc.State != Stopped {
		t.Fatalf("expected Stopped, got %s", proc.State)
	}
	if proc.PID != 0 {
		t.Fatalf("expected PID 0 after kill, got %d", proc.PID)
	}
}

func TestManager_Kill_Escalation(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	// Shell ignores SIGINT and loops forever so Stop() stays in
	// Stopping until we escalate with Kill().
	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "trap '' INT; while true; do sleep 1; done",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Let the shell set up its signal handler before we send SIGINT.
	time.Sleep(150 * time.Millisecond)

	// Begin a graceful stop in the background.
	go pm.Stop("a")

	// Give Stop time to enter the Stopping state and wait on the timer.
	time.Sleep(50 * time.Millisecond)

	// Kill while stopping should escalate to SIGKILL and let the
	// in-flight Stop call finish the transition.
	if err := pm.Kill("a"); err != nil {
		t.Fatalf("kill escalation failed: %v", err)
	}

	// Wait for the background Stop to finish.
	time.Sleep(100 * time.Millisecond)

	// Kill-during-Stop is an intentional termination: the state becomes
	// Stopped, not Failed.
	proc, _ := pm.Get("a")
	if proc.State != Stopped {
		t.Fatalf("expected Stopped after escalation, got %s", proc.State)
	}
}

func TestManager_Restart(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "echo first",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := pm.Restart("a"); err != nil {
		t.Fatalf("restart failed: %v", err)
	}

	// After restart the process should be Running again.
	proc, _ := pm.Get("a")
	if proc.State != Running {
		t.Fatalf("expected Running after restart, got %s", proc.State)
	}
	if proc.PID == 0 {
		t.Fatal("expected non-zero PID after restart")
	}
}

func TestManager_Restart_NotTracked(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	if err := pm.Restart("a"); err == nil {
		t.Fatal("expected error restarting untracked component")
	}
}

func TestManager_GetAndAll(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	if err := pm.Start(StartRequest{Component: "a", Command: "echo a"}); err != nil {
		t.Fatalf("start a failed: %v", err)
	}
	if err := pm.Start(StartRequest{Component: "b", Command: "echo b"}); err != nil {
		t.Fatalf("start b failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	all := pm.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 processes, got %d", len(all))
	}

	pa, ok := pm.Get("a")
	if !ok {
		t.Fatal("expected to find process a")
	}
	if pa.Component != "a" {
		t.Fatalf("expected component a, got %s", pa.Component)
	}

	_, ok = pm.Get("z")
	if ok {
		t.Fatal("expected process z to not exist")
	}
}

func TestManager_OutputCapture(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "echo line1; echo line2; echo line3",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	proc, _ := pm.Get("a")
	if proc.Output.Len() != 3 {
		t.Fatalf("expected 3 output lines, got %d", proc.Output.Len())
	}
	lines := proc.Output.Lines()
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Fatalf("unexpected output: %v", lines)
	}
}

func TestManager_Stop_TimeoutEscalation(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)

	// Shell ignores SIGINT and loops forever so Stop is forced to
	// escalate to SIGKILL after the timeout.
	if err := pm.Start(StartRequest{
		Component: "a",
		Command:   "trap '' INT; while true; do sleep 1; done",
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Let the shell set up its signal handler before we test escalation.
	time.Sleep(150 * time.Millisecond)

	if err := pm.Stop("a"); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// SIGKILL escalation is still an intentional termination: Stopped,
	// not Failed.
	proc, _ := pm.Get("a")
	if proc.State != Stopped {
		t.Fatalf("expected Stopped after SIGKILL escalation, got %s", proc.State)
	}
}

func TestTrackedExitBelongsToLaunch(t *testing.T) {
	pm := NewProcessManager(nil, shortTimeout)
	first, err := pm.StartTracked(StartRequest{Component: "svc", Command: "exit 7"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if p, ok := pm.Get("svc"); ok && p.State == Failed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first process did not exit")
		}
		time.Sleep(time.Millisecond)
	}
	second, err := pm.StartTracked(StartRequest{Component: "svc", Command: "true"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		exit <-chan ProcessState
		want ProcessState
	}{{first, Failed}, {second, Stopped}} {
		select {
		case got := <-tc.exit:
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing exit")
		}
	}
}

func TestStopPreservesLeaderFailureDuringOutputDrain(t *testing.T) {
	dir := t.TempDir()
	pm := NewProcessManager(nil, shortTimeout)
	exit, err := pm.StartTracked(StartRequest{
		Component: "svc", ExecDir: dir,
		Command: "(while [ ! -f release ]; do sleep .01; done) & exit 7",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(filepath.Join(dir, "release"), nil, 0600)
		pm.WaitOutput()
	})
	select {
	case state := <-exit:
		if state != Failed {
			t.Fatalf("expected failed before output drains, got %s", state)
		}
	case <-time.After(drainGracePeriod / 2):
		t.Fatal("leader outcome blocked by output drain")
	}
	if err := pm.Stop("svc"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("stop of failed process returned %v", err)
	}
	if proc, _ := pm.Get("svc"); proc.State != Failed {
		t.Fatalf("stop changed process failure to %s", proc.State)
	}
}
