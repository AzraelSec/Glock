package source

import "github.com/AzraelSec/glock/internal/git"

// githubSource materializes a git repository checked out into its working
// directory. Pull forwards to the git abstraction, which already errors
// when the directory does not exist yet.
type githubSource struct {
	g    git.Git
	repo git.Repo
}

// ensure to implement the Source interface
var _ Source = githubSource{}

func (s githubSource) Pull() error {
	return s.g.Pull(s.repo, false)
}
