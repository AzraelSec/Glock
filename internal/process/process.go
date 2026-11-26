package process

// ProcessState represents the lifecycle state of a Process.
type ProcessState string

const (
	// Sleeping means the Process has never been started.
	Sleeping ProcessState = "Sleeping"
	// Running means the OS process is alive and the PID is known.
	Running ProcessState = "Running"
	// Stopping means a stop signal has been sent to the OS process.
	// No interactions are accepted until the Process reaches Stopped
	// or Failed.
	Stopping ProcessState = "Stopping"
	// Stopped means the OS process exited cleanly (zero exit code).
	// The Process has been executed at least once.
	Stopped ProcessState = "Stopped"
	// Failed means the OS process exited with a non-zero code.
	Failed ProcessState = "Failed"
)

// Process is a runtime instance of a Component.
type Process struct {
	exit chan ProcessState
	// Component is the name of the owning Component.
	Component string
	// State is the current lifecycle state.
	State ProcessState
	// PID is the OS process ID; 0 when the Process is not running.
	PID int
	// Output is the buffer that captures stdout/stderr from the
	// current execution. It is cleared when the Process is started
	// again.
	Output Output
	// Env is the resolved KEY=VAL environment entries that the
	// process was started with. It is a snapshot from the last start.
	Env []string
}
