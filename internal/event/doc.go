// Package event defines runtime notifications emitted by the Glock system.
//
// Events are categorized as:
//
//   - Application Events: lifecycle of the whole application, for example
//     application.started, application.stopping, application.stopped.
//   - Component Events: lifecycle of individual Components, for example
//     component.starting, component.started, component.stopping,
//     component.stopped.
//   - Process Events: process.failed, process.stdout, process.stderr.
//   - Task Events: task.started, task.finished, task.failed.
//   - Workspace Events: workspace.starting, workspace.started,
//     workspace.stopping, workspace.stopped.
//   - Source Events: source.sync.started, source.sync.finished,
//     source.sync.failed.
//
// All event names are defined as const strings in names.go. Managers and
// consumers must use these constants instead of raw strings so that the
// compiler can catch typos and renaming is mechanical.
//
// The Event struct carries a typed Payload (any) that is pattern-matched
// by consumers. Each event category that needs structured data defines a
// payload struct in payloads.go. For example component.started carries a
// ComponentStartedPayload with the PID field.
//
// The bus is the central event pipeline. It is asynchronous: a dispatcher
// goroutine reads from a buffered inbound channel and fans events out to
// matching subscribers. Each subscriber gets its own buffered channel. If a
// subscriber's buffer is full the event is dropped for that subscriber — a
// slow consumer must never block others.
//
// Segregated interfaces are provided so that components only need the
// surface area they actually use:
//
//   - Publisher — push-only interface for managers.
//   - Bus — full bidirectional interface (publish + subscribe) for the
//     concrete implementation and the orchestrator.
//   - Subscription — handle returned from Subscribe; gives a receive-only
//     channel and a Cancel() method.
//
// The TUI and other consumers subscribe to these events through the bus;
// managers communicate exclusively through the event pipeline.
package event
