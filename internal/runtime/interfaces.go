package runtime

import (
	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/task"
	"github.com/AzraelSec/glock/internal/workspace"
)

// WorkspaceAll is the virtual workspace that shows all components.
const WorkspaceAll = "[All]"

// WorkspaceUnassigned is the virtual workspace that shows only unassigned components.
const WorkspaceUnassigned = "[Unassigned]"

// WorkspaceProvider returns workspace definitions.
type WorkspaceProvider interface {
	Workspaces() []workspace.Workspace
}

// ComponentProvider returns components, optionally filtered by workspace.
// Each entry pairs a Component definition with its runtime Process so the
// caller can display state, PID, and output without coupling the Component
// struct to mutable runtime data.
type ComponentProvider interface {
	Components(workspace string) []ComponentEntry
}

// EventProvider returns recent runtime events.
type EventProvider interface {
	RecentEvents(limit int) []event.Event
}

// TaskProvider returns top-level project tasks.
type TaskProvider interface {
	Tasks() []task.Task
}

// ComponentController dispatches component lifecycle commands.
type ComponentController interface {
	StartComponent(name string) error
	StopComponent(name string) error
	RestartComponent(name string) error
}

// WorkspaceController dispatches workspace-level lifecycle commands.
type WorkspaceController interface {
	StartWorkspace(name string) error
	StopWorkspace(name string) error
}

// TaskController dispatches task execution commands.
type TaskController interface {
	RunTask(name string) error
}

// Providers bundles widget reads and command adapters. Frontends receive
// a local read model populated by session events, never live engine providers.
type Providers struct {
	Workspaces     WorkspaceProvider
	Components     ComponentProvider
	Events         EventProvider
	Tasks          TaskProvider
	Controller     ComponentController
	WsController   WorkspaceController
	TaskController TaskController
	Output         OutputController
}
