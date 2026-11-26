// Package engine assembles and hosts the frontend-neutral v2 runtime.
package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/orchestrator"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/runtime"
	"github.com/AzraelSec/glock/internal/workspace"
)

// Session owns one frontend connection and finalizes its attached engine.
// Frontend selections and read models stay separate from runtime state.
type outputStats struct{ dropped, truncated atomic.Uint64 }

type Session struct {
	stats          *outputStats
	stream         chan process.OutputLine
	streamCancel   context.CancelFunc
	orch           *orchestrator.Orchestrator
	ctx            context.Context
	cancel         context.CancelFunc
	commands       chan runtime.Command
	shutdown       chan runtime.Command
	results        chan runtime.CommandResult
	snapshots      chan *runtime.Snapshot
	events         chan runtime.Notification
	wg             sync.WaitGroup
	closeOnce      sync.Once
	closeErr       error
	outputMu       sync.Mutex
	output         runtime.OutputTarget
	outputRevision uint64
}

// NewSession attaches a frontend to an initialized engine.
func NewSession(orch *orchestrator.Orchestrator) *Session {
	return newSession(orch, nil, nil, nil)
}
func newSession(orch *orchestrator.Orchestrator, stats *outputStats, stream chan process.OutputLine, streamCancel context.CancelFunc) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{orch: orch, ctx: ctx, cancel: cancel, stats: stats, stream: stream, streamCancel: streamCancel,
		commands: make(chan runtime.Command, 64), shutdown: make(chan runtime.Command, 1),
		results: make(chan runtime.CommandResult, 64), snapshots: make(chan *runtime.Snapshot, 1),
		events: make(chan runtime.Notification),
	}
	initial := s.snapshot(1)
	// A dedicated shutdown lane can cancel initialization even when every
	// ordinary command worker is waiting on a lifecycle hook.
	for range 8 {
		s.wg.Add(1)
		go s.worker(s.commands)
	}
	s.wg.Add(1)
	go s.worker(s.shutdown)
	s.wg.Add(1)
	go s.publishSnapshots()
	s.wg.Add(1)
	go s.deliver(initial)
	return s
}

func (s *Session) Send(ctx context.Context, command runtime.Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.ctx.Err() != nil {
		return runtime.ErrDisconnected
	}
	queue := s.commands
	if command.Action == runtime.Shutdown {
		queue = s.shutdown
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return runtime.ErrDisconnected
	case queue <- command:
		return nil
	}
}
func (s *Session) Events() <-chan runtime.Notification { return s.events }

// Close disconnects transport, then completes engine cleanup. It never waits
// for a frontend to drain results, making unexpected frontend exits safe.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		if s.streamCancel != nil {
			s.streamCancel()
		}
		s.closeErr = s.orch.Close()
		s.wg.Wait()
		if s.stream != nil {
			close(s.stream)
		}
	})
	return s.closeErr
}

func (s *Session) worker(queue <-chan runtime.Command) {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case command := <-queue:
			if s.ctx.Err() != nil {
				return
			}
			err := s.execute(command)
			result := runtime.CommandResult{ID: command.ID, Action: command.Action, Target: command.Target}
			if err != nil {
				result.Error = err.Error()
			}
			select {
			case s.results <- result:
			case <-s.ctx.Done():
				return
			}
		}
	}
}

func (s *Session) execute(c runtime.Command) error {
	switch c.Action {
	case runtime.Initialize:
		return s.orch.Start()
	case runtime.StartComponent:
		return s.orch.StartComponent(c.Target)
	case runtime.StopComponent:
		return s.orch.StopComponent(c.Target)
	case runtime.RestartComponent:
		return s.orch.RestartComponent(c.Target)
	case runtime.StartWorkspace:
		return s.orch.StartWorkspace(c.Target)
	case runtime.StopWorkspace:
		return s.orch.StopWorkspace(c.Target)
	case runtime.RunTask:
		return s.orch.RunTask(c.Target)
	case runtime.Shutdown:
		return s.orch.Stop()
	case runtime.SelectOutput:
		if c.Output.Kind != "" && c.Output.Kind != runtime.ComponentOutput && c.Output.Kind != runtime.TaskOutput {
			return fmt.Errorf("unknown output kind %q", c.Output.Kind)
		}
		s.outputMu.Lock()
		if c.OutputRevision >= s.outputRevision {
			s.output = c.Output
			s.outputRevision = c.OutputRevision
		}
		s.outputMu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown command action %q", c.Action)
	}
}

func (s *Session) publishSnapshots() {
	defer s.wg.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	sequence := uint64(1)
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			sequence++
			snapshot := s.snapshot(sequence)
			// Only state may be replaced. Command results use another reliable lane.
			select {
			case <-s.snapshots:
			default:
			}
			select {
			case s.snapshots <- snapshot:
			case <-s.ctx.Done():
				return
			}
		}
	}
}

func (s *Session) deliver(initial *runtime.Snapshot) {
	defer s.wg.Done()
	defer close(s.events)
	select {
	case s.events <- runtime.Notification{Snapshot: initial}:
	case <-s.ctx.Done():
		return
	}
	for {
		select {
		case <-s.ctx.Done():
			return
		case result := <-s.results:
			select {
			case s.events <- runtime.Notification{Result: &result}:
			case <-s.ctx.Done():
				return
			}
		case snapshot := <-s.snapshots:
			select {
			case s.events <- runtime.Notification{Snapshot: snapshot}:
			default: // the next snapshot will repair a slow frontend's read model
			}
		}
	}
}

func (s *Session) snapshot(sequence uint64) *runtime.Snapshot {
	out := &runtime.Snapshot{Sequence: sequence, Workspaces: s.orch.Workspaces(), OutputLoss: s.OutputLoss()}
	for i := range out.Workspaces {
		out.Workspaces[i].Components = slices.Clone(out.Workspaces[i].Components)
	}
	slices.SortFunc(out.Workspaces[2:], func(a, b workspace.Workspace) int { return strings.Compare(a.Name, b.Name) })
	s.outputMu.Lock()
	target := s.output
	revision := s.outputRevision
	s.outputMu.Unlock()
	out.Output.Target = target
	out.Output.Revision = revision
	for _, entry := range s.orch.Components(runtime.WorkspaceAll) {
		definition := entry.Component
		definition.EnvFilenames = slices.Clone(definition.EnvFilenames)
		cp := runtime.ComponentSnapshot{Definition: definition}
		if p := entry.Process; p != nil {
			cp.Process = &runtime.ProcessSnapshot{State: p.State, PID: p.PID}
			if reader, ok := p.Output.(interface{ Truncated() bool }); ok {
				cp.Process.OutputTruncated = reader.Truncated()
			}
			if target.Kind == runtime.ComponentOutput && target.Name == definition.Name && p.Output != nil {
				copyRetainedOutput(&out.Output, p.Output)
			}
		}
		out.Components = append(out.Components, cp)
	}
	for _, t := range s.orch.Tasks() {
		taskSnapshot := runtime.TaskSnapshot{Name: t.Name, Description: t.Description, Command: t.Command, ExecDir: t.ExecDir, State: t.State}
		if reader, ok := t.Output.(interface{ Truncated() bool }); ok {
			taskSnapshot.OutputTruncated = reader.Truncated()
		}
		out.Tasks = append(out.Tasks, taskSnapshot)
		if target.Kind == runtime.TaskOutput && target.Name == t.Name && t.Output != nil {
			copyRetainedOutput(&out.Output, t.Output)
		}
	}
	slices.SortFunc(out.Tasks, func(a, b runtime.TaskSnapshot) int { return strings.Compare(a.Name, b.Name) })
	for _, e := range s.orch.RecentEvents(1000) {
		out.Events = append(out.Events, cloneEvent(e))
	}
	return out
}

func copyRetainedOutput(snapshot *runtime.OutputSnapshot, output process.Output) {
	if reader, ok := output.(interface {
		RetainedOutput() ([]process.OutputFragment, uint64, bool)
	}); ok {
		snapshot.Fragments, snapshot.Evicted, snapshot.Truncated = reader.RetainedOutput()
		snapshot.Lines = make([]string, len(snapshot.Fragments))
		for i, fragment := range snapshot.Fragments {
			snapshot.Lines[i] = fragment.Text
		}
		return
	}
	snapshot.Lines = output.Lines()
}

// Diagnostic payloads currently consist entirely of value fields. Copy each
// payload too so a frontend cannot mutate the engine's retained event history.
func cloneEvent(e event.Event) event.Event {
	switch p := e.Payload.(type) {
	case *event.ComponentStartedPayload:
		cp := *p
		e.Payload = &cp
	case *event.ComponentStoppedPayload:
		cp := *p
		e.Payload = &cp
	case *event.ProcessFailedPayload:
		cp := *p
		e.Payload = &cp
	case *event.ProcessOutputPayload:
		cp := *p
		e.Payload = &cp
	case *event.TaskStartedPayload:
		cp := *p
		e.Payload = &cp
	case *event.TaskFinishedPayload:
		cp := *p
		e.Payload = &cp
	case *event.TaskFailedPayload:
		cp := *p
		e.Payload = &cp
	case *event.WorkspacePayload:
		cp := *p
		e.Payload = &cp
	}
	return e
}

// OpenProject returns an uninitialized streaming session. The caller starts
// draining Output and Events before sending Initialize. Output uses a bounded
// queue and records dropped fragments instead of blocking producers.
func OpenProject(project *config.Project) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	orch, stats, lines := streamingProject(ctx, project, orchestrator.Options{})
	return newSession(orch, stats, lines, cancel)
}

func streamingProject(ctx context.Context, project *config.Project, opts orchestrator.Options) (*orchestrator.Orchestrator, *outputStats, chan process.OutputLine) {
	lines := make(chan process.OutputLine, 4096)
	stats := &outputStats{}
	sink := func(line process.OutputLine) {
		if line.Truncated {
			stats.truncated.Add(1)
		}
		select {
		case <-ctx.Done():
			stats.dropped.Add(1)
			return
		default:
		}
		select {
		case lines <- line:
		default:
			stats.dropped.Add(1)
		}
	}
	opts.Output = sink
	orch := orchestrator.NewOrchestratorWithOptions(project.Config, project.Root, opts)
	return orch, stats, lines
}
func (s *Session) Output() <-chan process.OutputLine { return s.stream }
func (s *Session) OutputLoss() runtime.OutputLoss {
	if s.stats == nil {
		return runtime.OutputLoss{}
	}
	return runtime.OutputLoss{DroppedChunks: s.stats.dropped.Load(), TruncatedDrains: s.stats.truncated.Load()}
}
