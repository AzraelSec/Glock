package task

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/process"
)

// Result reports one task execution's terminal state. Err distinguishes
// cancellation, timeout, launch errors, and nonzero command exits.
type Result struct {
	State    TaskState
	ExitCode int
	Err      error
}

// TaskManager owns task executions, including their cancellation and reaping.
type TaskManager struct {
	outputSink  process.OutputSink
	tasks       map[string]*Task
	runners     map[string]*process.OSProcess
	cancels     map[string]context.CancelFunc
	mu          sync.RWMutex
	active      sync.WaitGroup
	publisher   event.Publisher
	stopTimeout time.Duration
}

// NewTaskManagerWithTimeout supplies a stop grace period for embedders/tests.
func NewTaskManagerWithTimeout(publisher event.Publisher, timeout time.Duration, sinks ...process.OutputSink) *TaskManager {
	if timeout <= 0 {
		timeout = process.DefaultStopTimeout
	}
	var sink process.OutputSink
	if len(sinks) > 0 {
		sink = sinks[0]
	}
	return &TaskManager{outputSink: sink,
		tasks:       make(map[string]*Task),
		runners:     make(map[string]*process.OSProcess),
		cancels:     make(map[string]context.CancelFunc),
		publisher:   publisher,
		stopTimeout: timeout,
	}
}

func NewTaskManager(publisher event.Publisher) *TaskManager {
	return NewTaskManagerWithTimeout(publisher, process.DefaultStopTimeout)
}

// Run is the background-context compatibility wrapper for RunContext.
func (tm *TaskManager) Run(t *Task) (<-chan Result, error) {
	return tm.RunContext(context.Background(), t)
}

// RunContext reserves the name before fork/exec. Cancellation is honored even
// during registration, and completion is delivered exactly once after reaping.
func (tm *TaskManager) RunContext(parent context.Context, t *Task) (<-chan Result, error) {
	tm.mu.Lock()
	if err := parent.Err(); err != nil {
		tm.mu.Unlock()
		return nil, err
	}
	if existing, ok := tm.tasks[t.Name]; ok && existing.State == Running {
		tm.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", process.ErrAlreadyRunning, t.Name)
	}
	cp := *t
	cp.Env = slices.Clone(t.Env)
	if cp.Output == nil {
		cp.Output = process.NewRingOutput(0)
	} else {
		cp.Output.Clear()
	}
	cp.Output = process.WithStream(cp.Output, "task", cp.Name, tm.outputSink)
	cp.State = Running
	ctx, cancel := context.WithCancel(parent)
	tm.tasks[t.Name] = &cp
	tm.cancels[t.Name] = cancel
	tm.active.Add(1)
	tm.mu.Unlock()
	completion := make(chan Result, 1)
	runner, err := process.NewOSProcess(cp.Command, cp.ExecDir, cp.Env, cp.Output)
	if err != nil {
		err = fmt.Errorf("start task %s: %w", cp.Name, err)
		tm.finish(&cp, Result{State: Failed, ExitCode: -1, Err: err}, completion)
		cancel()
		tm.active.Done()
		return completion, err
	}
	tm.mu.Lock()
	tm.runners[cp.Name] = runner
	tm.mu.Unlock()
	tm.emitEvent(event.TaskStarted, cp.Name, &event.TaskStartedPayload{PID: runner.PID()})
	go func() {
		defer tm.active.Done()
		defer cancel()
		var runErr error
		select {
		case <-runner.ExitChan():
		case <-ctx.Done():
			// Preserve the result if cancellation races with output draining.
			select {
			case <-runner.ExitChan():
			default:
				runErr = ctx.Err()
				runner.Terminate(tm.stopTimeout)
			}
		}
		code := runner.ExitCode()
		state := Finished
		if runErr != nil || code != 0 {
			state = Failed
			if runErr == nil {
				runErr = fmt.Errorf("task %q exited with code %d", cp.Name, code)
			}
		}
		result := Result{State: state, ExitCode: code, Err: runErr}
		<-runner.WaitChan()
		tm.finish(&cp, result, completion)
	}()
	return completion, nil
}

func (tm *TaskManager) finish(t *Task, res Result, completion chan Result) {
	tm.mu.Lock()
	t.State = res.State
	delete(tm.runners, t.Name)
	delete(tm.cancels, t.Name)
	tm.mu.Unlock()
	if res.State == Finished {
		tm.emitEvent(event.TaskFinished, t.Name, &event.TaskFinishedPayload{ExitCode: res.ExitCode})
	} else {
		tm.emitEvent(event.TaskFailed, t.Name, &event.TaskFailedPayload{ExitCode: res.ExitCode})
	}
	completion <- res
	close(completion)
}

// CancelAllAndWait cancels and reaps every admitted task. The owner must stop
// admitting ordinary work first; subsequent cleanup tasks may still be run.
func (tm *TaskManager) CancelAllAndWait() {
	tm.mu.RLock()
	cancels := make([]context.CancelFunc, 0, len(tm.cancels))
	for _, cancel := range tm.cancels {
		cancels = append(cancels, cancel)
	}
	tm.mu.RUnlock()
	for _, cancel := range cancels {
		cancel()
	}
	tm.active.Wait()
}

// Get and All return state copies; Output remains synchronized and live inside
// the engine. Frontend transport copies its lines rather than exposing it.
func (tm *TaskManager) Get(name string) (*Task, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	t, ok := tm.tasks[name]
	if !ok {
		return nil, false
	}
	cp := *t
	cp.Env = slices.Clone(t.Env)
	return &cp, true
}

func (tm *TaskManager) All() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	out := make([]*Task, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		cp := *t
		cp.Env = slices.Clone(t.Env)
		out = append(out, &cp)
	}
	return out
}

func (tm *TaskManager) emitEvent(name, component string, payload any) {
	if tm.publisher != nil {
		tm.publisher.Publish(event.Event{Name: name, Component: component, Payload: payload})
	}
}
