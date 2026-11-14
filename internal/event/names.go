package event

// ------------------------------------------------------------------
// Application Events
// ------------------------------------------------------------------

const (
	ApplicationStarted  = "application.started"
	ApplicationStopping = "application.stopping"
	ApplicationStopped  = "application.stopped"
)

// ------------------------------------------------------------------
// Component Events
// ------------------------------------------------------------------

const (
	ComponentStarting = "component.starting"
	ComponentStarted  = "component.started"
	ComponentStopping = "component.stopping"
	ComponentStopped  = "component.stopped"
)

// ------------------------------------------------------------------
// Process Events
// ------------------------------------------------------------------

const (
	ProcessFailed = "process.failed"
	ProcessStdout = "process.stdout"
	ProcessStderr = "process.stderr"
)

// ------------------------------------------------------------------
// Task Events
// ------------------------------------------------------------------

const (
	TaskStarted  = "task.started"
	TaskFinished = "task.finished"
	TaskFailed   = "task.failed"
)

// ------------------------------------------------------------------
// Workspace Events
// ------------------------------------------------------------------

const (
	WorkspaceStarting = "workspace.starting"
	WorkspaceStarted  = "workspace.started"
	WorkspaceStopping = "workspace.stopping"
	WorkspaceStopped  = "workspace.stopped"
)

// ------------------------------------------------------------------
// Source Events
// ------------------------------------------------------------------

const (
	SourceSyncStarted  = "source.sync.started"
	SourceSyncFinished = "source.sync.finished"
	SourceSyncFailed   = "source.sync.failed"
)
