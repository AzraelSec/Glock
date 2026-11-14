// Package process defines the runtime instance of a Component.
//
// A Process is created when a Component is started and tracks the OS
// process lifecycle: PID, state transitions, and an Output buffer.
//
// A Process owns its state machine. State transitions are driven by
// three sources: the OS process exit code, UI commands from the TUI,
// and events from the main event pipeline.
//
// When a Process receives a stop request it enters the Stopping state
// and sends a graceful signal (SIGINT or SIGTERM) to the process group.
// If the process does not exit within the configured timeout, the signal
// is escalated to SIGKILL (see the overmind process manager for a
// reference implementation of this escalation pattern). No user
// interactions are accepted while a Process is in the Stopping state —
// the caller must wait until the Process reaches Stopped or Failed.
//
// Lifecycle (no explicit Starting state — Sleeping transitions directly
// to Running once the OS process is confirmed alive):
//
//	Sleeping -> Running -> Stopping -> Stopped
//	Running may transition to Failed
//	Stopping may transition to Failed
//	Sleeping, Stopped, and Failed can transition to Running (restart)
//
// State semantics:
//
//   - Sleeping: the Process has never been started.
//   - Running: the OS process is alive and the PID is known.
//   - Stopping: a stop signal has been sent; interactions are locked
//     until the OS process exits.
//   - Stopped: the OS process exited cleanly (zero exit code). The
//     Process has been executed at least once.
//   - Failed: the OS process exited with a non-zero code.
package process
