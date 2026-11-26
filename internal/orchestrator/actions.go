package orchestrator

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/task"
)

// maxConcurrentActions bounds how many component lifecycle operations
// (fork/exec or signal-and-wait) run simultaneously during workspace
// start/stop. Unbounded fan-out would fire a fork/exec and SIGINT storm
// from a single keypress on large workspaces.
var maxConcurrentActions = max(runtime.NumCPU(), 2)

// StartComponent reserves the entire lifecycle before running setup.
func (o *Orchestrator) StartComponent(name string) error {
	done, err := o.begin(name)
	if err != nil {
		return err
	}
	defer done()
	return o.startWithRollback(name)
}

// StartTrackedComponent leaves registered cleanup to the batch supervisor so
// a failed launch can be reported before its rollback completes.
func (o *Orchestrator) StartTrackedComponent(name string) (<-chan process.ProcessState, error) {
	done, err := o.begin(name)
	if err != nil {
		return nil, err
	}
	defer done()
	return o.launch(name, false)
}

func (o *Orchestrator) startWithRollback(name string) error {
	_, err := o.launch(name, true)
	return err
}

func (o *Orchestrator) launch(name string, rollback bool) (exit <-chan process.ProcessState, err error) {
	if p, ok := o.procMgr.Get(name); ok {
		if p.State == process.Running {
			return nil, process.ErrAlreadyRunning
		}
		if p.State == process.Stopping {
			return nil, process.ErrStopping
		}
	}
	// A completed/crashed previous execution still needs its cleanup.
	if o.needsCleanup(name) {
		if err := o.stopComponent(name); err != nil {
			return nil, err
		}
	}
	cfg := o.cfg.Components[name]
	dir := cfg.ExecutionDir(o.cfg.Sources, o.rootDir)
	env := loadEnv(dir, cfg.EnvFilenames)
	if err := o.workCtx.Err(); err != nil {
		return nil, err
	}
	o.lifecycleMu.Lock()
	o.cleanup[name] = cleanupState{env: env}
	o.lifecycleMu.Unlock()
	if rollback {
		defer func() {
			if err != nil {
				err = errors.Join(err, o.stopComponent(name))
			}
		}()
	}
	o.bus.Publish(event.Event{Name: event.ComponentStarting, Component: name})
	if cfg.On != nil {
		if err := o.startHooks(o.workCtx, cfg.On, name, dir, env, func(key string) {
			o.lifecycleMu.Lock()
			state := o.cleanup[name]
			state.steps = append(state.steps, key)
			o.cleanup[name] = state
			o.lifecycleMu.Unlock()
		}); err != nil {
			return nil, err
		}
	}
	if err := o.workCtx.Err(); err != nil {
		return nil, err
	}
	return o.procMgr.StartTracked(process.StartRequest{Component: name, Command: cfg.Run, ExecDir: dir, Env: env})
}

func (o *Orchestrator) needsCleanup(name string) bool {
	o.lifecycleMu.Lock()
	defer o.lifecycleMu.Unlock()
	_, ok := o.cleanup[name]
	return ok
}

// StopComponent awaits cleanup even when the main process already exited.
func (o *Orchestrator) StopComponent(name string) error {
	done, err := o.begin(name)
	if err != nil {
		return err
	}
	defer done()
	return o.stopComponent(name)
}

func (o *Orchestrator) stopComponent(name string) error {
	err := o.cleanupComponent(name)
	o.lifecycleMu.Lock()
	if o.stopping && err != nil {
		o.cleanupErrors = append(o.cleanupErrors, err)
	}
	o.lifecycleMu.Unlock()
	return err
}

// CleanupFailedComponent returns cleanup errors to the supervisor. If shutdown
// already owns the component, Stop will run and report its cleanup instead.
func (o *Orchestrator) CleanupFailedComponent(name string) error {
	done, err := o.begin(name)
	if errors.Is(err, ErrShuttingDown) {
		return nil
	}
	if err != nil {
		return err
	}
	defer done()
	if !o.needsCleanup(name) {
		return nil
	}
	return o.cleanupComponent(name)
}

func (o *Orchestrator) cleanupComponent(name string) error {
	o.lifecycleMu.Lock()
	state, eligible := o.cleanup[name]
	o.lifecycleMu.Unlock()
	if !eligible {
		return fmt.Errorf("%w: %s", process.ErrNotRunning, name)
	}
	o.bus.Publish(event.Event{Name: event.ComponentStopping, Component: name})
	var errs []error
	if proc, ok := o.procMgr.Get(name); ok && proc.State == process.Running {
		if err := o.procMgr.Stop(name); err != nil && !errors.Is(err, process.ErrNotRunning) {
			errs = append(errs, err)
		}
	}
	cfg := o.cfg.Components[name]
	if cfg.On != nil {
		errs = append(errs, o.cleanupHooks(cfg.On, state.steps, name,
			cfg.ExecutionDir(o.cfg.Sources, o.rootDir), state.env))
	}
	o.lifecycleMu.Lock()
	delete(o.cleanup, name)
	o.lifecycleMu.Unlock()
	return errors.Join(errs...)
}

// Restart follows the same cleanup/setup paths and reloads dotenv values.
func (o *Orchestrator) RestartComponent(name string) error {
	done, err := o.begin(name)
	if err != nil {
		return err
	}
	defer done()
	if o.needsCleanup(name) {
		if err := o.stopComponent(name); err != nil {
			return err
		}
	}
	return o.startWithRollback(name)
}

// RunTask admits an ordinary task owned by the application work context.
func (o *Orchestrator) RunTask(name string) error {
	done, err := o.begin("")
	if err != nil {
		return err
	}
	defer done()
	t, ok := o.cfg.Tasks[name]
	if !ok {
		return fmt.Errorf("task %q not found", name)
	}
	_, err = o.taskMgr.RunContext(o.workCtx, &task.Task{
		Name: name, Description: t.Description, Command: t.Run, ExecDir: o.rootDir,
		State: task.Pending, Output: process.NewRingOutput(0),
	})
	return err
}

// StartWorkspace starts all non-running components in the given workspace.
// Components that are already Running are skipped. Components in Sleeping,
// Stopped, or Failed state are started (restarted if needed). The method
// runs component starts concurrently and returns an aggregated error if
// any component failed to start. An unknown workspace name is an error.
//
// The workspace.started event is published only when every component
// start succeeded, so the event stream never claims success on partial
// failure.
func (o *Orchestrator) StartWorkspace(name string) error {
	done, err := o.begin("")
	if err != nil {
		return err
	}
	defer done()
	names, ok := o.componentNamesForWorkspace(name)
	if !ok {
		return fmt.Errorf("workspace %q not found", name)
	}
	o.bus.Publish(event.Event{
		Name:    event.WorkspaceStarting,
		Payload: &event.WorkspacePayload{Workspace: name, Components: len(names)},
	})

	var wg sync.WaitGroup
	errCh := make(chan error, len(names))
	sem := make(chan struct{}, maxConcurrentActions)

	for _, compName := range names {
		// Skip components that are already running or stopping.
		if proc, ok := o.procMgr.Get(compName); ok &&
			(proc.State == process.Running || proc.State == process.Stopping) {
			continue
		}

		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := o.StartComponent(n); err != nil {
				errCh <- fmt.Errorf("component %q: %w", n, err)
			}
		}(compName)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	o.bus.Publish(event.Event{
		Name:    event.WorkspaceStarted,
		Payload: &event.WorkspacePayload{Workspace: name, Components: len(names)},
	})
	return nil
}

// StopWorkspace stops all running components in the given workspace.
// Components that are not Running are skipped. The method runs component
// stops concurrently and returns an aggregated error if any component
// failed to stop. An unknown workspace name is an error.
//
// The workspace.stopped event is published only when every component
// stop succeeded.
func (o *Orchestrator) StopWorkspace(name string) error {
	done, err := o.begin("")
	if err != nil {
		return err
	}
	defer done()
	names, ok := o.componentNamesForWorkspace(name)
	if !ok {
		return fmt.Errorf("workspace %q not found", name)
	}
	o.bus.Publish(event.Event{
		Name:    event.WorkspaceStopping,
		Payload: &event.WorkspacePayload{Workspace: name, Components: len(names)},
	})

	var wg sync.WaitGroup
	errCh := make(chan error, len(names))
	sem := make(chan struct{}, maxConcurrentActions)

	for _, compName := range names {
		// Only stop components that are actually running.
		if !o.needsCleanup(compName) {
			continue
		}

		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := o.StopComponent(n); err != nil {
				errCh <- fmt.Errorf("component %q: %w", n, err)
			}
		}(compName)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	o.bus.Publish(event.Event{
		Name:    event.WorkspaceStopped,
		Payload: &event.WorkspacePayload{Workspace: name, Components: len(names)},
	})
	return nil
}
