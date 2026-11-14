// Package component defines a logical service or asset within a Glock project.
//
// A Component has a unique name that acts as its identifier. It may reference
// zero or one Source; when a Source is configured, the Component's main
// Process and its inline Tasks run inside that Source's path directory. A
// Component may also omit a Source entirely, relying on executables or files
// already present on the host machine; in that case its Execution Environment
// defaults to the Project Folder. This is useful for host tools, local
// proxies, or system services.
//
// A Component has a required run command — the main command whose Process
// tracks the Component lifecycle. It may also declare on.startup and
// on.shutdown Tasks that run around the main Process. Updater hints belong
// to Git sources and are consumed by the update command.
//
// A Component is a pure definition — it carries no runtime state. When
// started, a Component creates a Process (see package process) that executes
// inside the Component's Execution Environment and owns the lifecycle state,
// PID, and output buffer.
package component
