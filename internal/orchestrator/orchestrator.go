package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/AzraelSec/glock/internal/component"
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/runtime"
	"github.com/AzraelSec/glock/internal/task"
	"github.com/AzraelSec/glock/internal/workspace"
)

const maxRecentEvents = 1000

// taskWaitTimeout bounds how long lifecycle waits (component startup
// tasks, application shutdown tasks) block on a single task. Without a
// deadline a hung task would freeze StartComponent or application quit
// forever.
const taskWaitTimeout = 5 * time.Minute

// eventRing is a fixed-capacity ring buffer of events. It has O(1)
// insertion with zero steady-state allocation, replacing the previous
// slice-shift approach which caused periodic realloc-copy spikes and
// retained stale payload references in shifted-off slots.
type eventRing struct {
	buf   []event.Event
	head  int // index of the oldest event
	count int
}

func newEventRing(capacity int) *eventRing {
	return &eventRing{buf: make([]event.Event, capacity)}
}

func (r *eventRing) add(e event.Event) {
	if r.count < len(r.buf) {
		r.buf[(r.head+r.count)%len(r.buf)] = e
		r.count++
		return
	}
	r.buf[r.head] = e
	r.head = (r.head + 1) % len(r.buf)
}

// snapshotNewest returns up to limit events, newest first.
func (r *eventRing) snapshotNewest(limit int) []event.Event {
	if limit <= 0 || limit > r.count {
		limit = r.count
	}
	out := make([]event.Event, 0, limit)
	for i := 0; i < limit; i++ {
		idx := (r.head + r.count - 1 - i + len(r.buf)) % len(r.buf)
		out = append(out, r.buf[idx])
	}
	return out
}

// Orchestrator is the concrete assembly point that bridges parsed
// configuration and the runtime managers. It is the single owner of the
// event bus, the Process and Task managers, and the in-memory registries
// built from configuration.
type Orchestrator struct {
	cfg     *config.ConfigV2
	rootDir string

	bus     event.Bus
	procMgr *process.ProcessManager
	taskMgr *task.TaskManager

	components map[string]component.Component
	workspaces map[string]workspace.Workspace
	tasks      []task.Task

	events   *eventRing
	eventsMu sync.Mutex

	eventsSub     event.Subscription
	collectorDone chan struct{}

	stopOnce         sync.Once
	closeOnce        sync.Once
	stopErr          error
	lifecycleMu      sync.Mutex
	launchMu         sync.RWMutex
	onStartupFailure func()
	stopping         bool
	busy             map[string]bool
	cleanup          map[string]cleanupState
	appSteps         []string
	actions          sync.WaitGroup
	workCtx          context.Context
	cancelWork       context.CancelFunc
	hookTimeout      time.Duration
	appStarting      bool
	appStarted       bool
	appCleanup       bool
	cleanupErrors    []error
}

type cleanupState struct {
	env   []string
	steps []string
}

// Options allow embedders and tests to override lifecycle deadlines without
// adding configuration fields. Nonpositive durations use production defaults.
type Options struct {
	HookTimeout, StopTimeout time.Duration
	Output                   process.OutputSink
	// OnStartupFailure requests fail-fast shutdown as soon as a hook fails.
	// With this callback, shutdown signals running processes before joining
	// in-flight actions. The callback must not wait for shutdown.
	OnStartupFailure func()
}

// NewOrchestrator creates an engine with the default lifecycle deadlines.
func NewOrchestrator(cfg *config.ConfigV2, rootDir string) *Orchestrator {
	return NewOrchestratorWithOptions(cfg, rootDir, Options{})
}

// NewOrchestratorWithOptions creates a runtime from parsed configuration.
// It instantiates the event bus, ProcessManager, and TaskManager, builds
// static registries from cfg, and subscribes to the bus for the event
// ring buffer.
func NewOrchestratorWithOptions(cfg *config.ConfigV2, rootDir string, opts Options) *Orchestrator {
	if opts.HookTimeout <= 0 {
		opts.HookTimeout = taskWaitTimeout
	}
	if opts.StopTimeout <= 0 {
		opts.StopTimeout = process.DefaultStopTimeout
	}
	workCtx, cancelWork := context.WithCancel(context.Background())
	bus := event.NewBus(0)
	procMgr := process.NewProcessManager(bus, opts.StopTimeout, opts.Output)
	taskMgr := task.NewTaskManagerWithTimeout(bus, opts.StopTimeout, opts.Output)

	o := &Orchestrator{
		cfg:              cfg,
		onStartupFailure: opts.OnStartupFailure,
		rootDir:          rootDir,
		bus:              bus,
		procMgr:          procMgr,
		taskMgr:          taskMgr,
		components:       make(map[string]component.Component),
		workspaces:       make(map[string]workspace.Workspace),
		events:           newEventRing(maxRecentEvents),
		workCtx:          workCtx,
		cancelWork:       cancelWork,
		hookTimeout:      opts.HookTimeout,
		busy:             make(map[string]bool),
		cleanup:          make(map[string]cleanupState),
		appCleanup:       true,
	}

	// Build component registry from config.
	for name, c := range cfg.Components {
		comp := component.Component{
			Name:         name,
			Run:          c.Run,
			EnvFilenames: c.EnvFilenames,
			ExecDir:      c.ExecutionDir(cfg.Sources, rootDir),
		}
		if c.Source != nil {
			comp.Source = *c.Source
		}
		o.components[name] = comp
	}

	// Assign workspace names to components.
	for wsName, compNames := range cfg.Workspaces {
		for _, compName := range compNames {
			if comp, ok := o.components[compName]; ok {
				comp.Workspace = wsName
				o.components[compName] = comp
			}
		}
	}

	// Build workspace registry.
	for name, comps := range cfg.Workspaces {
		o.workspaces[name] = workspace.Workspace{
			Name:       name,
			Components: comps,
		}
	}

	// Build task registry from config.
	for name, t := range cfg.Tasks {
		o.tasks = append(o.tasks, task.Task{
			Name:        name,
			Description: t.Description,
			Command:     t.Run,
			ExecDir:     o.rootDir,
			State:       task.Pending,
			Output:      process.NewRingOutput(0),
		})
	}

	// Subscribe to all events for the ring buffer.
	o.eventsSub = bus.Subscribe(nil)
	o.collectorDone = make(chan struct{})
	go o.eventCollector()

	return o
}

// Providers returns the runtime.Providers bundle backed by this
// Orchestrator.
func (o *Orchestrator) Providers() runtime.Providers {
	return runtime.Providers{
		Workspaces:     o,
		Components:     o,
		Events:         o,
		Tasks:          o,
		Controller:     o,
		WsController:   o,
		TaskController: o,
	}
}

// ErrShuttingDown rejects work after shutdown admission closes.
var ErrShuttingDown = errors.New("application is shutting down")

// begin reserves the complete lifecycle operation, including hooks, before any
// side effects. WaitGroup admission and shutdown are serialized by one lock.
func (o *Orchestrator) begin(name string) (func(), error) {
	o.lifecycleMu.Lock()
	defer o.lifecycleMu.Unlock()
	if o.stopping {
		return nil, ErrShuttingDown
	}
	if name != "" {
		if _, ok := o.components[name]; !ok {
			return nil, fmt.Errorf("component %q not found", name)
		}
		if o.busy[name] {
			return nil, fmt.Errorf("component %q lifecycle action already in flight", name)
		}
		o.busy[name] = true
	}
	o.actions.Add(1)
	return func() {
		o.lifecycleMu.Lock()
		delete(o.busy, name)
		o.lifecycleMu.Unlock()
		o.actions.Done()
	}, nil
}

// Start awaits application initialization before announcing success.
func (o *Orchestrator) Start() error {
	return o.start(true)
}

// StartBatch leaves application rollback to Stop, which reports cleanup errors
// separately from the startup result.
func (o *Orchestrator) StartBatch() error {
	return o.start(false)
}

func (o *Orchestrator) start(rollback bool) error {
	done, err := o.begin("")
	if err != nil {
		return err
	}
	defer done()
	o.lifecycleMu.Lock()
	if o.appStarted {
		o.lifecycleMu.Unlock()
		return nil
	}
	if o.appStarting {
		o.lifecycleMu.Unlock()
		return process.ErrStarting
	}
	o.appStarting = true
	o.appCleanup = true
	o.lifecycleMu.Unlock()
	defer func() { o.lifecycleMu.Lock(); o.appStarting = false; o.lifecycleMu.Unlock() }()
	if o.cfg.On != nil {
		if err := o.startHooks(o.workCtx, o.cfg.On, "", o.rootDir, nil, func(key string) {
			o.lifecycleMu.Lock()
			o.appSteps = append(o.appSteps, key)
			o.lifecycleMu.Unlock()
		}); err != nil {
			if !rollback {
				return err
			}
			o.lifecycleMu.Lock()
			steps := append([]string(nil), o.appSteps...)
			o.appSteps = nil
			o.lifecycleMu.Unlock()
			cleanupErr := o.cleanupHooks(o.cfg.On, steps, "", o.rootDir, nil)
			o.lifecycleMu.Lock()
			o.appCleanup = false
			if o.stopping && cleanupErr != nil {
				o.cleanupErrors = append(o.cleanupErrors, cleanupErr)
			}
			o.lifecycleMu.Unlock()
			return errors.Join(err, cleanupErr)
		}
	}
	if err := o.workCtx.Err(); err != nil {
		return err
	}
	o.lifecycleMu.Lock()
	o.appStarted = true
	o.lifecycleMu.Unlock()
	o.bus.Publish(event.Event{Name: event.ApplicationStarted})
	return nil
}

// Stop closes admission, settles ordinary work, and completes all cleanup.
// Every caller observes the same completion and aggregated error.
func (o *Orchestrator) Stop() error {
	o.stopOnce.Do(func() {
		o.lifecycleMu.Lock()
		o.stopping = true
		o.cancelWork()
		o.lifecycleMu.Unlock()
		o.bus.Publish(event.Event{Name: event.ApplicationStopping})
		var earlyStops sync.WaitGroup
		var stopErrors chan error
		if o.onStartupFailure != nil {
			// Wait for fork/exec already in progress. Startup hooks can keep
			// draining while the running process groups receive SIGINT.
			o.launchMu.Lock()
			running := o.procMgr.All()
			o.launchMu.Unlock()
			stopErrors = make(chan error, len(running))
			for _, proc := range running {
				if proc.State != process.Running {
					continue
				}
				earlyStops.Go(func() {
					if err := o.procMgr.Stop(proc.Component); err != nil && !errors.Is(err, process.ErrNotRunning) {
						stopErrors <- err
					}
				})
			}
		}
		o.actions.Wait()
		o.taskMgr.CancelAllAndWait()
		earlyStops.Wait()
		var wg sync.WaitGroup
		sem := make(chan struct{}, maxConcurrentActions)
		for _, name := range o.allComponentNames() {
			if !o.needsCleanup(name) {
				continue
			}
			wg.Go(func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				_ = o.stopComponent(name) // errors are retained by the cleanup path
			})
		}
		wg.Wait()
		o.lifecycleMu.Lock()
		errs := append([]error(nil), o.cleanupErrors...)
		o.lifecycleMu.Unlock()
		if stopErrors != nil {
			close(stopErrors)
			for err := range stopErrors {
				errs = append(errs, err)
			}
		}
		if o.cfg.On != nil && o.appCleanup {
			errs = append(errs, o.cleanupHooks(o.cfg.On, o.appSteps, "", o.rootDir, nil))
		}
		o.procMgr.WaitOutput()
		o.stopErr = errors.Join(errs...)
		o.bus.Publish(event.Event{Name: event.ApplicationStopped})
	})
	return o.stopErr
}

// Close is an emergency safety net as well as event-bus finalization.
func (o *Orchestrator) Close() error {
	err := o.Stop()
	o.closeOnce.Do(func() {
		o.bus.Close()
		<-o.collectorDone
	})
	return err
}

// startHooks records compensation only after its corresponding startup succeeds.
func (o *Orchestrator) startHooks(ctx context.Context, on *config.On, namespace, dir string, env []string, record func(string)) error {
	if on.Steps == nil {
		return o.runHooks(ctx, on.Startup, namespace, dir, env, false)
	}
	for _, step := range on.Steps {
		if err := o.runHooks(ctx, []string{step.Startup}, namespace, dir, env, false); err != nil {
			return err
		}
		if step.Cleanup != "" {
			record(step.Cleanup)
		}
	}
	return nil
}
func (o *Orchestrator) cleanupHooks(on *config.On, steps []string, namespace, dir string, env []string) error {
	if on.Steps == nil {
		return o.runHooks(context.Background(), on.Shutdown, namespace, dir, env, true)
	}
	keys := append([]string(nil), steps...)
	slices.Reverse(keys)
	return o.runHooks(context.Background(), keys, namespace, dir, env, true)
}

// runHooks awaits each execution. Cleanup continues after failures; startup
// aborts on the first failure. Deadlines cancel and reap the actual task.
func (o *Orchestrator) runHooks(ctx context.Context, keys []string, namespace, dir string, env []string, cleanup bool) error {
	var errs []error
	for _, key := range keys {
		t := o.cfg.Tasks[key]
		name := key
		if namespace != "" {
			name = namespace + "/" + key
		}
		hookCtx, cancel := context.WithTimeout(ctx, o.hookTimeout)
		var onFailure func(error)
		if !cleanup && o.onStartupFailure != nil {
			onFailure = func(err error) {
				if !errors.Is(err, context.Canceled) {
					o.onStartupFailure()
				}
			}
		}
		completion, err := o.taskMgr.RunContextWithFailure(hookCtx, &task.Task{
			Name: name, Description: t.Description, Command: t.Run, ExecDir: dir,
			Env: env, State: task.Pending, Output: process.NewRingOutput(0),
		}, onFailure)
		if err == nil {
			err = (<-completion).Err
		}
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("hook %q: %w", name, err))
			if !cleanup {
				break
			}
		}
	}
	return errors.Join(errs...)
}

// eventCollector drains the subscription channel and appends events to
// the ring buffer.
func (o *Orchestrator) eventCollector() {
	defer close(o.collectorDone)
	for e := range o.eventsSub.Events() {
		o.eventsMu.Lock()
		o.events.add(e)
		o.eventsMu.Unlock()
	}
}
