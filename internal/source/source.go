package source

import (
	"errors"
	"fmt"
	"os"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/git"
)

var errNotImplemented = errors.New("not yet implemented")

// Source is a unified interface over the `sources` entries of glock.yml.
// Each concrete implementation knows how to update its materialized
// working directory; the mechanism is type-specific and opaque to the
// caller, so the same operation can be run uniformly over every source.
type Source interface {
	// Pull updates the source's working directory to the latest content.
	// It returns nil on success or an error whose concrete type is
	// implementation-specific and not meaningful to interface users.
	Pull() error
}

// NewSource builds a source for its configured local directory.
func NewSource(cfg config.Source, projectDir string, g git.Git) (Source, error) {
	if cfg.Path == nil || *cfg.Path == "" {
		return nil, errors.New("source path is required")
	}
	dir := cfg.LocalDir(projectDir)

	switch cfg.Type {
	case config.SourceTypeGitHub:
		if cfg.URL == nil || cfg.Head == nil {
			return nil, fmt.Errorf("source type %q requires url and head", cfg.Type)
		}
		return githubSource{
			g: g,
			repo: git.Repo{
				Remote: git.RemoteGitUrl(*cfg.URL),
				Path:   dir,
				Refs:   git.BranchName(*cfg.Head),
			},
		}, nil
	case config.SourceTypeFilesystem:
		return filesystemSource{dir: dir}, nil
	case config.SourceTypeS3:
		if cfg.URL == nil {
			return nil, fmt.Errorf("source type %q requires url", cfg.Type)
		}
		return s3Source{dir: dir, url: *cfg.URL}, nil
	default:
		return nil, fmt.Errorf("unsupported source type %q", cfg.Type)
	}
}

// filesystemSource uses the configured existing directory directly.
type filesystemSource struct{ dir string }

func (s filesystemSource) Pull() error {
	info, err := os.Stat(s.dir)
	if err != nil {
		return fmt.Errorf("filesystem source %q: %w", s.dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("filesystem source %q is not a directory", s.dir)
	}
	return nil
}
