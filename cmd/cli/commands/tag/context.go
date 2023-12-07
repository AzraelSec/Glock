package tag

import "time"

// todo: enlarge the available fields
type sharedContext struct {
	Now time.Time
}

func (s sharedContext) NewTagContext(branch string) tagContext {
	return tagContext{
		sharedContext: s,
		Branch:        branch,
	}
}

// todo: enlarge the available fields
type tagContext struct {
	sharedContext
	Branch string
}
