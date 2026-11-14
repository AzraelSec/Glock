package component

// Component is a logical service or asset.
//
// A Component is a pure definition — it carries no runtime state. When
// a Component is started, a process.Process is created to track the
// OS process lifecycle, state, PID, and output buffer.
type Component struct {
	// Name is the unique identifier of the Component.
	Name string
	// Source is the name of the configured Source (empty when no
	// Source is set, meaning the Component uses a host executable).
	Source string
	// Workspace is the name of the Workspace this Component belongs
	// to (empty when the Component is unassigned).
	Workspace string
	// Run is the main command whose process tracks the Component
	// lifecycle.
	Run string
	// EnvFilenames is the list of .env file names that will be loaded
	// when the Component process starts. Files are processed in order;
	// when the same key appears in multiple files, the value from the
	// LAST file wins (later overrides earlier).
	EnvFilenames []string
	// ExecDir is the working directory used when starting the Component
	// process.
	ExecDir string
}
