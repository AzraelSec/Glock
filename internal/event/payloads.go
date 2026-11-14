package event

// ComponentStartedPayload is attached to component.started events.
type ComponentStartedPayload struct {
	PID int
}

// ComponentStoppedPayload is attached to component.stopped events.
// Signal is the terminating signal (e.g. 2 for SIGINT) when the process
// was killed by one, 0 for a normal exit.
type ComponentStoppedPayload struct {
	ExitCode int
	Signal   int
}

// ProcessFailedPayload is attached to process.failed events.
// Signal is the terminating signal when the process was killed by one,
// 0 for a non-zero exit code.
type ProcessFailedPayload struct {
	ExitCode int
	Signal   int
}

// ProcessOutputPayload is attached to process.stdout and process.stderr events.
type ProcessOutputPayload struct {
	Line string
}

// TaskStartedPayload is attached to task.started events.
type TaskStartedPayload struct {
	PID int
}

// TaskFinishedPayload is attached to task.finished events.
type TaskFinishedPayload struct {
	ExitCode int
}

// TaskFailedPayload is attached to task.failed events.
type TaskFailedPayload struct {
	ExitCode int
}

// WorkspacePayload is attached to workspace.starting, workspace.started,
// workspace.stopping, and workspace.stopped events.
type WorkspacePayload struct {
	Workspace  string
	Components int
}
