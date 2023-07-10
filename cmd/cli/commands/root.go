package commands

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/pkg/external_git"
	"github.com/AzraelSec/glock/pkg/serializer"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const CONFIG_PATH_ENV = "GLOCK_CONFIG_PATH"
const VERSION = "0.1"
const CONFIG_FILE_NAME = "glock.yml"
const MAX_CONFIG_FILE_DEPTH = 20

var rootCmd = &cobra.Command{
	Use:     "glock",
	Short:   "Shooting flies with a bazooka \U0001f680",
	Version: VERSION,
}

func ExecuteRoot() {
	// TODO: Make this lazy so that it's possible to create commands
	// that don't require the configuration file (remote init)
	gm := external_git.NewExternalGit()
	cm, err := loadConfigManager()

	if err != nil {
		color.Red("Impossible to find a valid %s nearby configuration file", CONFIG_FILE_NAME)
		return
	}

	rootCmd.AddCommand(
		debugFactory(cm),
		initFactory(cm, gm),
		startFactory(cm, gm),
		statusFactory(cm, gm),
		updateFactory(cm, gm),
		switchFactory(cm, gm),
		resetFactory(cm, gm),
	)

	rootCmd.Execute()
}

func loadConfigManager() (*config.ConfigManager, error) {
	configPath := ""
	wd, err := os.Getwd()
	if err == nil {
		configPath, _ = findNearestConfigFile(wd, MAX_CONFIG_FILE_DEPTH)
	}

	if v, exists := os.LookupEnv(CONFIG_PATH_ENV); exists {
		configPath = v
	}

	if configPath == "" {
		return nil, errors.New("no configuration file found")
	}

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