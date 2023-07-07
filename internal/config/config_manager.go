package config

import (
	"errors"
	"path"

	"github.com/AzraelSec/glock/pkg/serializer"
	"github.com/AzraelSec/glock/pkg/git"
)

type ConfigManager struct {
	*Config
	configPath string
}

type LiveRepo struct {
	Name      string
	GitConfig git.Repo
	Config    Repo
}

var RepoNotFoundErr = errors.New("repo not found")

func (cm *ConfigManager) GetRepos() []LiveRepo {
	repos := []LiveRepo{}
	for configKey, repoData := range cm.Repos {
		repoPath := "./"
		if cm.configPath != "" {
			repoPath = path.Dir(cm.configPath)
		}
		if cm.Config.RootPath != "" {
			repoPath = cm.Config.RootPath
		}
		if repoData.Path != "" {
			repoPath = path.Join(repoPath, repoData.Path)
		} else {
			repoPath = path.Join(repoPath, configKey)
		}

		remote, err := git.NewRemoteGitUrl(repoData.Remote)
		if err != nil {
			continue
		}

		refs := "main"
		if cm.Config.RootRefs != "" {
			refs = cm.Config.RootRefs
		}
		if repoData.Refs != "" {
			refs = repoData.Refs
		}

		res := LiveRepo{
			Name: configKey,
			GitConfig: git.Repo{
				Path:   repoPath,
				Remote: remote,
				Refs:   git.BranchName(refs),
			},
			Config: *repoData,
		}

		repos = append(repos, res)
	}
	return repos
}

func NewConfigManager(configPath string, src []byte, d serializer.Serializer) (*ConfigManager, error) {
	// TODO: introduce JSON-schema validation in here
	dataSource := new(Config)
	if err := d.Unmarshal(src, dataSource); err != nil {
		return nil, err
	}
	return &ConfigManager{dataSource, configPath}, dataSource.Validate()
}
