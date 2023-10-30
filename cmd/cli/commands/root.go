package commands

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/AzraelSec/glock/cmd/cli/commands/status"
	"github.com/AzraelSec/glock/cmd/cli/commands/switch_cmd"
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/pkg/external_git"
	"github.com/AzraelSec/glock/pkg/serializer"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const CONFIG_PATH_ENV = "GLOCK_CONFIG_PATH"
const VERSION = "0.0.1"
const CONFIG_FILE_NAME = "glock.yml"
const MAX_CONFIG_FILE_DEPTH = 20

var rootCmd = &cobra.Command{
	Use:     "glock",
	Short:   "Shooting flies with a bazooka \U0001f680",
	Version: VERSION,
}

func ExecuteRoot() {
	cfPath, err := getConfigFilePath()
	if err != nil {
		color.Red("Impossible to identify a valid %s config file", CONFIG_FILE_NAME)
		return
	}

	// TODO: Make this lazy so that it's possible to create commands
	// that don't require the configuration file (remote init)
	gm := external_git.NewExternalGit()
	cm, err := loadConfigManager(cfPath)
	if err != nil {
		color.Red("Impossible to find a valid %s nearby configuration file.\nDetails: %v", CONFIG_FILE_NAME, err)
		return
	}

	rootCmd.AddCommand(
		initFactory(cm, gm),
		startFactory(cm, gm),
		status.NewStatus(cm, gm).Command(),
		updateFactory(cm, gm),
		switchcmd.New(cm, gm).Command(),
		resetFactory(cm, gm),
	)

	rootCmd.Execute()
}

func getConfigFilePath() (string, error) {
	if v, exists := os.LookupEnv(CONFIG_PATH_ENV); exists {
		return v, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", errors.New("impossible to find the current directory")
	}

	configPath, err := findNearestConfigFile(wd, MAX_CONFIG_FILE_DEPTH)
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

func findNearestConfigFile(currentPath string, hop int8) (string, error) {
	_, err := os.Stat(path.Join(currentPath, CONFIG_FILE_NAME))
	if err == nil {
		return path.Join(currentPath, CONFIG_FILE_NAME), nil
	}

	if currentPath == "/" || hop == 0 {
		return "", fmt.Errorf("No %s file found", CONFIG_FILE_NAME)
	}

	parentDir := path.Dir(currentPath)
	return findNearestConfigFile(parentDir, hop-1)
}
