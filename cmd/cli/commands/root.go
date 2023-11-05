package commands

import (
	"io"
	"os"

	"github.com/AzraelSec/glock/cmd/cli/commands/status"
	switchcmd "github.com/AzraelSec/glock/cmd/cli/commands/switch_cmd"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const CONFIG_PATH_ENV = "GLOCK_CONFIG_PATH"
const VERSION = "0.0.1"
const CONFIG_FILE_NAME = "glock.yml"
const MAX_CONFIG_FILE_DEPTH = 20

var rootCmd = &cobra.Command{
	Use:          "glock",
	Short:        "Shooting flies with a bazooka \U0001f680",
	SilenceUsage: true,
	Version:      VERSION,
}

type RedWriter struct {
	output io.Writer
}

func (w RedWriter) Write(p []byte) (n int, err error) {
	return color.New(color.FgHiRed).Print(string(p))
}

func ExecuteRoot() {
	dm := dependency.NewDependencyManager(CONFIG_PATH_ENV, CONFIG_FILE_NAME, MAX_CONFIG_FILE_DEPTH)

	rootCmd.AddCommand(
		startFactory(dm),
		status.NewStatus(dm).Command(),
		updateFactory(dm),
		switchcmd.New(dm).Command(),
		resetFactory(dm),
	)

	rootCmd.SetErr(RedWriter{os.Stderr})

	rootCmd.Execute()
}
