package config

// Schema for glock.yml (v2 architecture).

// A relative output directory inside the project folder. Absolute
// paths and parent-directory traversal are rejected lexically. Symlinks may
// resolve outside the project root; this is not a containment boundary.
#RemoteDir: string & != "" & !~"^/" & !~"(^|/)\\.\\.(/|$)"

// A reusable one-shot command. Used in `on.startup`,
// `on.shutdown`, `components.*.on.*`, and top-level `tasks`.
#Task: {
	description?: string
	run:          string & != ""
}

// A Source acquires and materializes a working directory.
// `type` discriminates which fields apply.
#Source: #SourceGitHub | #SourceHTTP | #SourceS3 | #SourceFilesystem

#SourceGitHub: {
	updater?: string
	exclude_tag?: bool
	type: "github"
	path: #RemoteDir & !~"^(\\./)*\\.?$"
	url:  string & != "" // git URL
	head: string & != "" // required: default branch
}

#SourceHTTP: {
	type: "http"
	path: #RemoteDir
	url:  string & != ""

	// no `head`: http has no branch concept
}

#SourceS3: {
	type: "s3"
	path: #RemoteDir
	url:  string & != ""

	// no `head`: s3 has no branch concept
}

#SourceFilesystem: {
	type: "filesystem"
	path: string & != "" // existing local directory

	// no `head`: filesystem has no branch concept
}

// A Component is a declarative unit; it is not a running Process.
#Component: {
	source?: string & != "" // optional reference to a key in `sources`
	// optional .env file names to load before execution, resolved
	// relative to the component's Execution Environment
	env_filenames?: [...string]
	run:            string & != ""
	on?: {
		startup?: [...string]  // references to top-level task keys
		shutdown?: [...string] // references to top-level task keys
	}
}

// A Workspaces is a named collection of component references.
#Workspaces: [...string]

#On: {
	startup?: [...string]  // references to top-level task keys
	shutdown?: [...string] // references to top-level task keys
}

// Root document.
#Config: {
	// Optional explicit version. When present it must be 2 or "2".
	version?: 2 | "2"
	tag_pattern?: string

	sources?: {
		[string]: #Source
	}
	components?: {
		[string]: #Component
	}
	workspaces?: {
		[string]: #Workspaces
	}
	on?: #On
	tasks?: {
		[string]: #Task
	}
}

// `#Config` is the exported schema used to validate glock.yml.
config: #Config
