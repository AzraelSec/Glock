package orchestrator

import (
	"slices"

	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/runtime"
	"github.com/AzraelSec/glock/internal/task"
	"github.com/AzraelSec/glock/internal/workspace"
)

// Workspaces implements runtime.WorkspaceProvider.
func (o *Orchestrator) Workspaces() []workspace.Workspace {
	out := make([]workspace.Workspace, 0, len(o.workspaces)+2)
	out = append(out, workspace.Workspace{
		Name:       runtime.WorkspaceAll,
		Components: o.allComponentNames(),
	})
	out = append(out, workspace.Workspace{
		Name:       runtime.WorkspaceUnassigned,
		Components: o.unassignedComponentNames(),
	})
	for _, ws := range o.workspaces {
		out = append(out, ws)
	}
	return out
}

// Components implements runtime.ComponentProvider.
func (o *Orchestrator) Components(ws string) []runtime.ComponentEntry {
	names, _ := o.componentNamesForWorkspace(ws)
	entries := make([]runtime.ComponentEntry, 0, len(names))
	for _, name := range names {
		comp, ok := o.components[name]
		if !ok {
			// Phantom reference (workspace lists a component that does
			// not exist). Skip it rather than rendering a zero-value row.
			continue
		}
		proc, _ := o.procMgr.Get(name)
		entries = append(entries, runtime.ComponentEntry{
			Component: comp,
			Process:   proc,
		})
	}
	return entries
}

// RecentEvents implements runtime.EventProvider. The ring-buffer copy
// is taken under the lock; the timestamp sort runs outside the lock so
// the UI thread never blocks the collector (and vice versa).
func (o *Orchestrator) RecentEvents(limit int) []event.Event {
	o.eventsMu.Lock()
	out := o.events.snapshotNewest(limit)
	o.eventsMu.Unlock()

	// The async bus stamps timestamps before enqueueing, so arrival
	// order is not guaranteed to match timestamp order under concurrent
	// publishers. Sort explicitly newest-first.
	slices.SortFunc(out, func(a, b event.Event) int {
		return b.Timestamp.Compare(a.Timestamp)
	})
	return out
}

// Tasks implements runtime.TaskProvider.
func (o *Orchestrator) Tasks() []task.Task {
	out := make([]task.Task, 0, len(o.tasks))
	for _, t := range o.tasks {
		// Update state from task manager if the task has been run.
		if rt, ok := o.taskMgr.Get(t.Name); ok {
			out = append(out, *rt)
		} else {
			out = append(out, t)
		}
	}
	return out
}

func (o *Orchestrator) allComponentNames() []string {
	out := make([]string, 0, len(o.components))
	for name := range o.components {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func (o *Orchestrator) unassignedComponentNames() []string {
	out := make([]string, 0)
	for name, comp := range o.components {
		if comp.Workspace == "" {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// componentNamesForWorkspace resolves a workspace name to its component
// names. The boolean reports whether the workspace exists; the virtual
// workspaces ([All], "", [Unassigned]) always exist.
func (o *Orchestrator) componentNamesForWorkspace(ws string) ([]string, bool) {
	switch ws {
	case runtime.WorkspaceAll, "":
		return o.allComponentNames(), true
	case runtime.WorkspaceUnassigned:
		return o.unassignedComponentNames(), true
	default:
		if w, ok := o.workspaces[ws]; ok {
			return w.Components, true
		}
		return nil, false
	}
}
