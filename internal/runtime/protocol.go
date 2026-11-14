package runtime

import (
	"context"
	"errors"

	"github.com/AzraelSec/glock/internal/component"
	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/task"
	"github.com/AzraelSec/glock/internal/workspace"
)

// Action identifies frontend commands; these events are separate from the
// lossy diagnostic event bus. Request IDs correlate reliable completions.
type Action string

const (
	Initialize       Action = "application.initialize"
	StartComponent   Action = "component.start"
	StopComponent    Action = "component.stop"
	RestartComponent Action = "component.restart"
	StartWorkspace   Action = "workspace.start"
	StopWorkspace    Action = "workspace.stop"
	RunTask          Action = "task.run"
	SelectOutput     Action = "output.select"
	Shutdown         Action = "application.shutdown"
)

type OutputKind string

const (
	ComponentOutput OutputKind = "component"
	TaskOutput      OutputKind = "task"
)

type OutputTarget struct {
	Kind OutputKind
	Name string
}
type Command struct {
	ID             uint64
	Action         Action
	Target         string
	Output         OutputTarget
	OutputRevision uint64
}
type CommandResult struct {
	ID     uint64
	Action Action
	Target string
	Error  string
}

var ErrDisconnected = errors.New("engine session disconnected")

// Session is the entire frontend/engine boundary. Send reports admission,
// not execution success. Accepted commands receive a result unless disconnected.
type Session interface {
	Send(context.Context, Command) error
	Events() <-chan Notification
}

// Notification contains either a replaceable snapshot or a reliable result.
// Snapshots are detached values: no manager, output-buffer, or OS references.
type Notification struct {
	Snapshot *Snapshot
	Result   *CommandResult
}
type ProcessSnapshot struct {
	State process.ProcessState
	PID   int
	Env   []string
}
type ComponentSnapshot struct {
	Definition component.Component
	Process    *ProcessSnapshot
}
type TaskSnapshot struct {
	Name, Description, Command, ExecDir string
	Env                                 []string
	State                               task.TaskState
}
type OutputSnapshot struct {
	Revision uint64
	Target   OutputTarget
	Lines    []string
}
type Snapshot struct {
	Sequence   uint64
	Workspaces []workspace.Workspace
	Components []ComponentSnapshot
	Tasks      []TaskSnapshot
	Events     []event.Event
	Output     OutputSnapshot
}

// OutputController changes which bounded output buffer the engine transfers.
type OutputController interface {
	SelectOutput(OutputTarget, uint64) error
}

// StreamingSession optionally provides lossless output. Consumers must drain it
// concurrently with Events; disconnect cancels blocked producers.
type StreamingSession interface {
	Session
	Output() <-chan process.OutputLine
}
