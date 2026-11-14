package event

import "time"

// Event is a runtime notification emitted by the Glock system.
type Event struct {
	// Name is the dot-separated event identifier, e.g. "component.started".
	// Use the constants in names.go instead of raw strings.
	Name string

	// Component is the name of the component that the event relates to
	// for component.* and process.* events, and the task name for
	// task.* events. It is empty for application-level and workspace
	// events (workspace events carry the workspace name in
	// WorkspacePayload), so component-scoped filters never receive
	// cross-namespace leakage.
	Component string

	// Timestamp is set by the bus when the event is published.
	Timestamp time.Time

	// Payload is a typed struct that carries event-specific data.
	// It is nil for events that carry no extra information.
	// Consumers should pattern-match on Name and type-assert Payload.
	Payload any
}
