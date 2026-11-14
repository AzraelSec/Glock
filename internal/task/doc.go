// Package task defines a reusable one-shot command within a Glock project.
//
// A Task is defined by a run command and an optional description. It is used
// in on.startup, on.shutdown, the top-level tasks list, and Component on
// blocks. A Task executes in the Execution Environment determined by its
// location: Tasks inside a Component's on block run in the Component's
// Execution Environment (the Source path directory, or the Project Folder if
// no Source is configured); Tasks inside the top-level on block run in the
// Project Folder.
//
// When a Task is executed it produces an Output Buffer that captures stdout
// and stderr chunks. The buffer is associated with a single execution and is
// cleared when the Task is queued again.
//
// Lifecycle:
//
//	Pending -> Running -> Finished
//	Running may transition to Failed
//	A Task in Finished or Failed can be queued again (-> Pending)
package task
