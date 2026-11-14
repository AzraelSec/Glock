package config

import (
	"errors"
	"fmt"
	"sort"

	"github.com/AzraelSec/glock/internal/git"
)

var ErrRepoNotFound = errors.New("repo not found")

// ConfigManager is the Git command catalog; runtime definitions stay in Project.
type ConfigManager struct {
	Repos      []LiveRepo
	ConfigPath string
	TagPattern string
}
type RepoOptions struct {
	Updater    string
	ExcludeTag bool
}
type LiveRepo struct {
	Name      string
	GitConfig git.Repo
	Config    RepoOptions
}

func RepositoryCatalog(project *Project) (*ConfigManager, error) {
	cm := &ConfigManager{ConfigPath: project.Path, TagPattern: project.Config.TagPattern}
	names := make([]string, 0, len(project.Config.Sources))
	for name, src := range project.Config.Sources {
		if src.Type == SourceTypeGitHub {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		src := project.Config.Sources[name]
		remote, err := git.NewRemoteGitUrl(*src.URL)
		if err != nil {
			return nil, fmt.Errorf("sources.%s.url: %w", name, err)
		}
		dir := src.LocalDir(project.Root)
		cm.Repos = append(cm.Repos, LiveRepo{Name: name, GitConfig: git.Repo{Remote: remote, Refs: git.BranchName(*src.Head), Path: dir}, Config: RepoOptions{Updater: src.Updater, ExcludeTag: src.ExcludeTag}})
	}
	if len(cm.Repos) == 0 {
		return nil, errors.New("configuration contains no Git sources applicable to this command")
	}
	return cm, nil
}
