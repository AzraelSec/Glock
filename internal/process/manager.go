package process

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/AzraelSec/glock/internal/event"
)

// DefaultStopTimeout is the grace period between the initial stop signal
// and the SIGKILL escalation.
const DefaultStopTimeout = 15 * time.Second

// StartRequest holds the fully-resolved parameters needed to launch a
// component's OS process.
type StartRequest struct {
	Component string   // component name (acts as the manager key)
	Command   string   // shell command to execute (passed to "sh -c")
	ExecDir   string   // working directory
	Env       []string // KEY=VAL environment entries
}

// ProcessManager is the single owner of all OS process lifecycle
// transitions.  It tracks each process's state, detects process death,
// and enforces the Stopping lock: no Start/Stop/Restart commands are
// accepted while a process is in the Stopping state.
type ProcessManager struct {
	outputSink OutputSink
	processes  map[string]*Process
	runners    map[string]*OSProcess
	requests   map[string]StartRequest
	starting   map[string]struct{}
	mu         sync.RWMutex
	publisher  event.Publisher
	timeout    time.Duration
}

// NewProcessManager returns a manager ready to track processes.
// If publisher is nil events are silently discarded.
func NewProcessManager(publisher event.Publisher, timeout time.Duration, sinks ...OutputSink) *ProcessManager {
	if timeout <= 0 {
		timeout = DefaultStopTimeout
	}
	var sink OutputSink
	if len(sinks) > 0 {
		sink = sinks[0]
	}
	return &ProcessManager{outputSink: sink,
		processes: make(map[string]*Process),
		runners:   make(map[string]*OSProcess),
		requests:  make(map[string]StartRequest),
		starting:  make(map[string]struct{}),
		publisher: publisher,
		timeout:   timeout,
	}
}

// Start launches a new OS process for the given component.
// The process state is set to Running immediately.
//
// If the component already has a Running process ErrAlreadyRunning is
// returned. A component in the Stopping state is rejected (ErrStopping)
// so an in-flight Stop can never corrupt a successor's state. Stopped
// or Failed processes may be restarted.
func (pm *ProcessManager) Start(req StartRequest) error {
	// Clone the caller's env slice so post-Start mutation cannot leak
	// into the runtime snapshot or the stored restart request.
	req.Env = slices.Clone(req.Env)

	pm.mu.Lock()
	if _, ok := pm.starting[req.Component]; ok {
		pm.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrStarting, req.Component)
	}
	if proc, exists := pm.processes[req.Component]; exists {
		switch proc.State {
		case Running:
			pm.mu.Unlock()
			return fmt.Errorf("%w: %s", ErrAlreadyRunning, req.Component)
		case Stopping:
			pm.mu.Unlock()
			return fmt.Errorf("%w: %s", ErrStopping, req.Component)
		}
	}
	// Reserve the key so a concurrent Start blocks here; the actual
	// fork/exec happens outside the lock.
	pm.starting[req.Component] = struct{}{}
	pm.mu.Unlock()

	output := WithStream(NewRingOutput(0), "component", req.Component, pm.outputSink)
	runner, err := NewOSProcess(req.Command, req.ExecDir, req.Env, output)
	if err != nil {
		pm.mu.Lock()
		delete(pm.starting, req.Component)
		pm.mu.Unlock()
		return err
	}

	proc := &Process{
		Component: req.Component,
		State:     Running,
		PID:       runner.pid,
		Output:    output,
		Env:       req.Env,
	}

	pm.mu.Lock()
	delete(pm.starting, req.Component)
	pm.processes[req.Component] = proc
	pm.runners[req.Component] = runner
	pm.requests[req.Component] = req
	pm.mu.Unlock()

	pm.emitEvent(event.ComponentStarted, req.Component, &event.ComponentStartedPayload{PID: runner.pid})

	// If the process exits before the monitor goroutine starts, handle it
	// synchronously so the state does not remain Running forever.
	select {
	case <-runner.WaitChan():
		pm.handleUnexpectedDeath(req.Component, runner)
	default:
		go pm.monitor(req.Component, runner)
	}

	return nil
}

// Stop sends a graceful stop signal (SIGINT) to the process group and
// waits up to the configured timeout.  If the process does not exit in
// time the signal is escalated to SIGKILL.
//
// State transitions: Running -> Stopping -> Stopped or Failed.
// No interactions are accepted while the process is Stopping.
func (pm *ProcessManager) Stop(name string) error {
	pm.mu.Lock()
	proc, ok := pm.processes[name]
	if !ok {
		pm.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if proc.State == Stopping {
		pm.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrStopping, name)
	}
	if proc.State != Running {
		pm.mu.Unlock()
		return fmt.Errorf("%w: %s (state: %s)", ErrNotRunning, name, proc.State)
	}

	proc.State = Stopping
	runner, ok := pm.runners[name]
	if !ok {
		// Roll back — a missing runner must not wedge the component in
		// the Stopping state forever.
		proc.State = Running
		pm.mu.Unlock()
		return fmt.Errorf("runner not found for %s", name)
	}
	pm.mu.Unlock()

	// TODO: support a configurable stop signal per-component in the future.
	// For now we always use SIGINT.
	if err := runner.SendSignal(syscall.SIGINT); err != nil {
		// The process may have already died before we could signal it.
		pm.transitionAfterStop(name, runner)
		return err
	}

	timer := time.NewTimer(pm.timeout)
	defer timer.Stop()

	select {
	case <-runner.WaitChan():
		pm.transitionAfterStop(name, runner)
	case <-timer.C:
		// Timeout fired — escalate to SIGKILL.
		_ = runner.SendSignal(syscall.SIGKILL)
		<-runner.WaitChan()
		pm.transitionAfterStop(name, runner)
	}

	return nil
}

// Kill sends SIGKILL to the process group immediately and waits for
// the process to die.  Like Stop, this is an intentional termination:
// the state transitions to Stopped, not Failed.
func (pm *ProcessManager) Kill(name string) error {
	pm.mu.Lock()
	proc, ok := pm.processes[name]
	if !ok {
		pm.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if proc.State == Stopping {
		// Escalation during an in-flight Stop — just send SIGKILL and
		// let the existing Stop call handle the transition. The runner
		// lookup must happen under the lock: the in-flight Stop deletes
		// the entry in transitionAfterStop.
		runner, ok := pm.runners[name]
		pm.mu.Unlock()
		if ok {
			_ = runner.SendSignal(syscall.SIGKILL)
		}
		return nil
	}
	if proc.State != Running {
		pm.mu.Unlock()
		return fmt.Errorf("%w: %s (state: %s)", ErrNotRunning, name, proc.State)
	}

	proc.State = Stopping
	runner, ok := pm.runners[name]
	if !ok {
		proc.State = Running
		pm.mu.Unlock()
		return fmt.Errorf("runner not found for %s", name)
	}
	pm.mu.Unlock()

	_ = runner.SendSignal(syscall.SIGKILL)
	<-runner.WaitChan()
	pm.transitionAfterStop(name, runner)

	return nil
}

// Restart stops the process (if running) and starts it again with the
// same StartRequest used in the previous Start call.
func (pm *ProcessManager) Restart(name string) error {
	pm.mu.RLock()
	req, ok := pm.requests[name]
	pm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no start request stored for %s", name)
	}
	if err := pm.Stop(name); err != nil {
		if errors.Is(err, ErrStopping) {
			return err
		}
		if !errors.Is(err, ErrNotRunning) && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("restart stop: %w", err)
		}
		// Process was not running or not tracked — safe to proceed.
	}
	return pm.Start(req)
}

// Get returns a snapshot of the Process state for a component. The
// returned pointer refers to a copy, so callers cannot mutate
// manager-owned state and never race with the monitor goroutine.
// Output remains a live (internally synchronized) reference.
func (pm *ProcessManager) Get(name string) (*Process, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.processes[name]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

// All returns snapshots of all tracked processes. Each entry is a copy;
// see Get.
func (pm *ProcessManager) All() []*Process {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]*Process, 0, len(pm.processes))
	for _, p := range pm.processes {
		cp := *p
		out = append(out, &cp)
	}
	return out
}

// ------------------------------------------------------------------
// Internal helpers
// ------------------------------------------------------------------

// monitor waits for the OS process to die unexpectedly (not via a
// Stop/Kill call) and transitions the manager state accordingly.
func (pm *ProcessManager) monitor(name string, runner *OSProcess) {
	<-runner.WaitChan()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc, ok := pm.processes[name]
	if !ok {
		return
	}
	// If a Stop/Kill is in progress it will handle the transition.
	if proc.State == Stopping {
		return
	}
	// Identity check: a newer Start may have replaced this runner; a
	// stale monitor must never transition a successor's state.
	if cur, ok := pm.runners[name]; !ok || cur != runner {
		return
	}

	exitCode := runner.ExitCode()
	if exitCode == 0 {
		proc.State = Stopped
		pm.emitEvent(event.ComponentStopped, name, &event.ComponentStoppedPayload{ExitCode: exitCode})
	} else {
		proc.State = Failed
		pm.emitEvent(event.ProcessFailed, name, &event.ProcessFailedPayload{ExitCode: exitCode, Signal: runner.Signal()})
	}
	proc.PID = 0
	delete(pm.runners, name)
}

// handleUnexpectedDeath is the synchronous counterpart of monitor for
// processes that exit before the monitor goroutine can be started.
func (pm *ProcessManager) handleUnexpectedDeath(name string, runner *OSProcess) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc, ok := pm.processes[name]
	if !ok {
		return
	}
	// Defensive: if someone is already handling this, skip.
	if proc.State == Stopping {
		return
	}
	if cur, ok := pm.runners[name]; !ok || cur != runner {
		return
	}

	exitCode := runner.ExitCode()
	if exitCode == 0 {
		proc.State = Stopped
		pm.emitEvent(event.ComponentStopped, name, &event.ComponentStoppedPayload{ExitCode: exitCode})
	} else {
		proc.State = Failed
		pm.emitEvent(event.ProcessFailed, name, &event.ProcessFailedPayload{ExitCode: exitCode, Signal: runner.Signal()})
	}
	proc.PID = 0
	delete(pm.runners, name)
}

// transitionAfterStop finalises state after a Stop or Kill call has
// observed the process death.  It must be called while NOT holding pm.mu.
//
// Stop and Kill are intentional terminations, so the state becomes
// Stopped regardless of exit code or signal; Failed is reserved for
// unexpected deaths detected by monitor.
func (pm *ProcessManager) transitionAfterStop(name string, runner *OSProcess) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc, ok := pm.processes[name]
	if !ok {
		return
	}
	// Identity check: if a newer Start replaced this runner while the
	// Stop was in flight, leave the successor alone.
	if cur, ok := pm.runners[name]; !ok || cur != runner {
		return
	}

	proc.State = Stopped
	pm.emitEvent(event.ComponentStopped, name, &event.ComponentStoppedPayload{
		ExitCode: runner.ExitCode(),
		Signal:   runner.Signal(),
	})
	proc.PID = 0
	delete(pm.runners, name)
}

func (pm *ProcessManager) emitEvent(name string, component string, payload any) {
	if pm.publisher != nil {
		pm.publisher.Publish(event.Event{
			Name:      name,
			Component: component,
			Payload:   payload,
		})
	}
}
