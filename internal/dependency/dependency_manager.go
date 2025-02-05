package dependency

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/external_git"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/serializer"
)

type DependencyManager struct {
	configPathEnv      string
	configFileName     string
	maxConfigFileDepth int8
}

func NewDependencyManager(configPathEnv, configFileName string, maxConfigFileDepth int8) *DependencyManager {
	return &DependencyManager{
		configPathEnv:      configPathEnv,
		configFileName:     configFileName,
		maxConfigFileDepth: maxConfigFileDepth,
	}
}

func (*DependencyManager) GetGit() (git.Git, error) {
	return external_git.NewExternalGit()
}

func (dm *DependencyManager) ConfigManagerFromFile(dir string) (*config.ConfigManager, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("impossible to identify a valid %s config file", dm.configPathEnv)
	}
	return loadConfigManager(path.Join(dir, dm.configFileName))
}

func (dm *DependencyManager) GetConfigManager() (*config.ConfigManager, error) {
	cfPath, err := dm.getConfigFilePath()
	if err != nil {
		return nil, fmt.Errorf("impossible to identify a valid %s config file", dm.configPathEnv)
	}

	cm, err := loadConfigManager(cfPath)
	if err != nil {
		return nil, fmt.Errorf("impossible to find a valid %s nearby configuration file.\nDetails: %v", dm.configFileName, err)
	}
	return cm, nil
}

func (dm *DependencyManager) getConfigFilePath() (string, error) {
	if v, exists := os.LookupEnv(dm.configPathEnv); exists {
		return v, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", errors.New("impossible to find the current directory")
	}

	configPath, err := dm.findNearestConfigFile(wd, dm.maxConfigFileDepth)
	if err != nil {
		return "", errors.New("no configuration file found")
	}
	return configPath, nil
}

func loadConfigManager(configPath string) (*config.ConfigManager, error) {
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	return config.NewConfigManager(configPath, rawConfig, serializer.NewYamlDecoder())
}

func (dm *DependencyManager) findNearestConfigFile(currentPath string, hop int8) (string, error) {
	_, err := os.Stat(path.Join(currentPath, dm.configFileName))
	if err == nil {
		return path.Join(currentPath, dm.configFileName), nil
	}

	if currentPath == "/" || hop == 0 {
		return "", fmt.Errorf("no %s file found", dm.configFileName)
	}

	parentDir := path.Dir(currentPath)
	return dm.findNearestConfigFile(parentDir, hop-1)
}
