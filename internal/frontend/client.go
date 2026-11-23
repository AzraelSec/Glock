// Package frontend maintains a frontend-owned read model from engine events.
// It never constructs or accesses engine managers or the orchestrator.
package frontend

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/runtime"
	"github.com/AzraelSec/glock/internal/task"
	"github.com/AzraelSec/glock/internal/workspace"
)

type Client struct {
	session  runtime.Session
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	nextID   atomic.Uint64
	mu       sync.RWMutex
	snapshot *runtime.Snapshot
	pending  map[uint64]chan error
	output   process.Output
}

// New receives the initial state before widgets are constructed. Afterwards
// a receiver continually consumes events, including while the UI is quitting.
func New(ctx context.Context, session runtime.Session) (*Client, error) {
	clientCtx, cancel := context.WithCancel(ctx)
	c := &Client{session: session, ctx: clientCtx, cancel: cancel, done: make(chan struct{}), pending: make(map[uint64]chan error)}
	select {
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	case n, ok := <-session.Events():
		if !ok || n.Snapshot == nil {
			cancel()
			return nil, runtime.ErrDisconnected
		}
		c.apply(n.Snapshot)
	}
	go c.receive()
	return c, nil
}

func (c *Client) Close() { c.cancel(); <-c.done }
func (c *Client) receive() {
	defer close(c.done)
	for {
		select {
		case <-c.ctx.Done():
			return
		case n, ok := <-c.session.Events():
			if !ok {
				return
			}
			if n.Snapshot != nil {
				c.apply(n.Snapshot)
			}
			if n.Result != nil {
				var err error
				if n.Result.Error != "" {
					err = errors.New(n.Result.Error)
				}
				c.mu.Lock()
				waiter := c.pending[n.Result.ID]
				delete(c.pending, n.Result.ID)
				c.mu.Unlock()
				if waiter != nil {
					waiter <- err
				}
			}
		}
	}
}

func (c *Client) apply(snapshot *runtime.Snapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshot != nil && snapshot.Sequence <= c.snapshot.Sequence {
		return
	}
	c.snapshot = snapshot
	buffer := process.NewMemoryOutput()
	for _, line := range snapshot.Output.Lines {
		buffer.Append(line)
	}
	c.output = buffer
}

// Execute communicates only through command/result events. Context cancellation
// abandons this wait, not engine ownership of an already-admitted operation.
func (c *Client) Execute(ctx context.Context, command runtime.Command) error {
	command.ID = c.nextID.Add(1)
	waiter := make(chan error, 1)
	c.mu.Lock()
	c.pending[command.ID] = waiter
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, command.ID); c.mu.Unlock() }()
	select {
	case <-c.done:
		return runtime.ErrDisconnected
	default:
	}
	if err := c.session.Send(ctx, command); err != nil {
		return err
	}
	select {
	case err := <-waiter:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		select {
		case err := <-waiter:
			return err
		default:
			return runtime.ErrDisconnected
		}
	}
}

func (c *Client) Providers() runtime.Providers {
	return runtime.Providers{Workspaces: c, Components: c, Events: c, Tasks: c,
		Controller: c, WsController: c, TaskController: c, Output: c}
}
func (c *Client) StartComponent(name string) error { return c.command(runtime.StartComponent, name) }
func (c *Client) StopComponent(name string) error  { return c.command(runtime.StopComponent, name) }
func (c *Client) RestartComponent(name string) error {
	return c.command(runtime.RestartComponent, name)
}
func (c *Client) StartWorkspace(name string) error { return c.command(runtime.StartWorkspace, name) }
func (c *Client) StopWorkspace(name string) error  { return c.command(runtime.StopWorkspace, name) }
func (c *Client) RunTask(name string) error        { return c.command(runtime.RunTask, name) }
func (c *Client) Shutdown() error                  { return c.command(runtime.Shutdown, "") }
func (c *Client) command(action runtime.Action, name string) error {
	return c.Execute(c.ctx, runtime.Command{Action: action, Target: name})
}
func (c *Client) SelectOutput(target runtime.OutputTarget, revision uint64) error {
	return c.Execute(c.ctx, runtime.Command{Action: runtime.SelectOutput, Output: target, OutputRevision: revision})
}

func (c *Client) Workspaces() []workspace.Workspace {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := slices.Clone(c.snapshot.Workspaces)
	for i := range out {
		out[i].Components = slices.Clone(out[i].Components)
	}
	return out
}
func (c *Client) Components(ws string) []runtime.ComponentEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make(map[string]bool)
	for _, w := range c.snapshot.Workspaces {
		if w.Name == ws {
			for _, name := range w.Components {
				names[name] = true
			}
		}
	}
	var out []runtime.ComponentEntry
	for _, entry := range c.snapshot.Components {
		if ws != "" && ws != runtime.WorkspaceAll && !names[entry.Definition.Name] {
			continue
		}
		definition := entry.Definition
		definition.EnvFilenames = slices.Clone(definition.EnvFilenames)
		cp := runtime.ComponentEntry{Component: definition}
		if p := entry.Process; p != nil {
			cp.Process = &process.Process{Component: definition.Name, State: p.State, PID: p.PID}
			if c.snapshot.Output.Target == (runtime.OutputTarget{Kind: runtime.ComponentOutput, Name: definition.Name}) {
				cp.Process.Output = c.output
			}
		}
		out = append(out, cp)
	}
	return out
}
func (c *Client) Tasks() []task.Task {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]task.Task, 0, len(c.snapshot.Tasks))
	for _, t := range c.snapshot.Tasks {
		cp := task.Task{Name: t.Name, Description: t.Description, Command: t.Command, ExecDir: t.ExecDir, State: t.State}
		if c.snapshot.Output.Target == (runtime.OutputTarget{Kind: runtime.TaskOutput, Name: t.Name}) {
			cp.Output = c.output
		}
		out = append(out, cp)
	}
	return out
}
func (c *Client) RecentEvents(limit int) []event.Event {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if limit <= 0 || limit > len(c.snapshot.Events) {
		limit = len(c.snapshot.Events)
	}
	return slices.Clone(c.snapshot.Events[:limit])
}
