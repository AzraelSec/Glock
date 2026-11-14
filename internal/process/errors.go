package process

import "errors"

var (
	// ErrStopping is returned when an operation is requested on a
	// process that is already in the Stopping state.
	ErrStopping = errors.New("process is stopping")

	// ErrNotFound is returned when a named process is not tracked by
	// the manager.
	ErrNotFound = errors.New("process not found")

	// ErrNotRunning is returned when Stop or Kill is called on a
	// process whose current state is not Running.
	ErrNotRunning = errors.New("process is not running")

	// ErrAlreadyRunning is returned when Start is called for a
	// component that already has a Running process.
	ErrAlreadyRunning = errors.New("process is already running")

	// ErrStarting is returned when Start is called concurrently for a
	// component whose start is already in flight.
	ErrStarting = errors.New("process start already in flight")
)
