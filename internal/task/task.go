package task

import "github.com/AzraelSec/glock/internal/process"

// TaskState represents the lifecycle state of a task.
type TaskState string

const (
	Pending  TaskState = "Pending"
	Running  TaskState = "Running"
	Finished TaskState = "Finished"
	Failed   TaskState = "Failed"
)

// Task is a reusable one-shot command.
type Task struct {
	Name        string
	Description string
	Command     string   // shell command to execute (passed to "sh -c")
	ExecDir     string   // working directory; empty means inherit parent's cwd
	Env         []string // additional KEY=VAL entries merged with os.Environ()
	State       TaskState
	Output      process.Output // stdout/stderr lines from the current execution
}
