package commands

import (
	"io"
	"os"

	"github.com/AzraelSec/glock/cmd/cli/commands/init_cmd"
	"github.com/AzraelSec/glock/cmd/cli/commands/status"
	switchcmd "github.com/AzraelSec/glock/cmd/cli/commands/switch_cmd"
	"github.com/AzraelSec/glock/cmd/cli/commands/tag"
	"github.com/AzraelSec/glock/cmd/cli/commands/update"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	CONFIG_PATH_ENV       = "GLOCK_CONFIG_PATH"
	VERSION               = "0.0.1"
	CONFIG_FILE_NAME      = "glock.yml"
	MAX_CONFIG_FILE_DEPTH = 20
)

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
	dm := dependency.NewManager(CONFIG_PATH_ENV, CONFIG_FILE_NAME, MAX_CONFIG_FILE_DEPTH)

	rootCmd.SetErr(RedWriter{os.Stderr})
	rootCmd.AddCommand(
		startFactory(dm),
		init_cmd.New(dm).Command(),
		status.NewStatus(dm).Command(),
		update.NewUpdate(dm).Command(),
		switchcmd.New(dm).Command(),
		resetFactory(dm),
		tag.New(dm).Command(),
	)
	rootCmd.Execute()
}
