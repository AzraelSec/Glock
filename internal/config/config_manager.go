package config

import (
	"errors"
	"path"

	"github.com/AzraelSec/glock/pkg/git"
	"github.com/AzraelSec/glock/pkg/serializer"
)

var RepoNotFoundErr = errors.New("repo not found")

type ConfigManager struct {
	Repos        []LiveRepo
	ConfigPath   string
	Services     []Services
	EnvFilenames []string
}

type LiveRepo struct {
	Name      string
	GitConfig git.Repo
	Config    ConfigRepo
}

func NewConfigManager(configPath string, src []byte, d serializer.Serializer) (*ConfigManager, error) {
	// TODO: introduce JSON-schema validation in here
	dataSource := new(Config)
	if err := d.Unmarshal(src, dataSource); err != nil {
		return nil, err
	}

	if err := dataSource.Validate(); err != nil {
		return nil, err
	}

	repos := []LiveRepo{}
	for configKey, repoData := range dataSource.Repos {

		prefixPath := "./"
		if configPath != "" {
			prefixPath = path.Dir(configPath)
		}
		if dataSource.RootPath != "" {
			prefixPath = dataSource.RootPath
		}

		res, err := hydrateRepoConfig(configKey, prefixPath, dataSource.RootRefs, *repoData)
		if err != nil {
			continue
		}

		repos = append(repos, res)
	}

	envFilenames := []string{}
	if len(dataSource.EnvFilenames) != 0 {
		envFilenames = dataSource.EnvFilenames
	}

	return &ConfigManager{
		ConfigPath:   configPath,
		Repos:        repos,
		Services:     dataSource.Services,
		EnvFilenames: envFilenames,
	}, dataSource.Validate()
}

func hydrateRepoConfig(name string, prefixPath string, rootRefs string, repoData ConfigRepo) (LiveRepo, error) {
	repoPath := prefixPath
	if repoData.Path != "" {
		repoPath = path.Join(repoPath, repoData.Path)
	} else {
		repoPath = path.Join(repoPath, name)
	}

	remote, err := git.NewRemoteGitUrl(repoData.Remote)
	if err != nil {
		return LiveRepo{}, err
	}

	refs := "main"
	if rootRefs != "" {
		refs = rootRefs
	}
	if repoData.Refs != "" {
		refs = repoData.Refs
	}

	return LiveRepo{
		Name: name,
		GitConfig: git.Repo{
			Path:   repoPath,
			Remote: remote,
			Refs:   git.BranchName(refs),
		},
		Config: repoData,
	}, nil
}
