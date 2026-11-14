package dependency

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
)

type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (*Manager) GetGit() (git.Git, error) {
	return git.NewGitCLI()
}

func (dm *Manager) ConfigManagerFromFile(dir string) (*config.ConfigManager, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("impossible to identify a valid %s config file", config.ConfigPathEnv)
	}
	return loadConfigManager(path.Join(dir, config.ConfigFileName))
}

func (dm *Manager) GetConfigManager() (*config.ConfigManager, error) {
	cfPath, err := dm.GetConfigFilePath()
	if err != nil {
		return nil, fmt.Errorf("impossible to identify a valid %s config file", config.ConfigPathEnv)
	}

	cm, err := loadConfigManager(cfPath)
	if err != nil {
		return nil, fmt.Errorf("impossible to find a valid %s nearby configuration file.\nDetails: %v", config.ConfigFileName, err)
	}
	return cm, nil
}

func (dm *Manager) GetConfigFilePath() (string, error) {
	if v, exists := os.LookupEnv(config.ConfigPathEnv); exists {
		return v, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", errors.New("impossible to find the current directory")
	}

	configPath, err := dir.FindNearestFile(wd, config.MaxConfigFileDepth, config.ConfigFileName)
	if err != nil {
		return "", errors.New("no configuration file found")
	}
	return configPath, nil
}

func loadConfigManager(configPath string) (*config.ConfigManager, error) {
	project, err := config.LoadFile(configPath)
	if err != nil {
		return nil, err
	}
	return config.RepositoryCatalog(project)
}

func (dm *Manager) LoadProject() (*config.Project, error) {
	path, err := dm.GetConfigFilePath()
	if err != nil {
		return nil, err
	}
	return config.LoadFile(path)
}
