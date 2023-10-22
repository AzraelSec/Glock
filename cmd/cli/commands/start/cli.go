package start

import (
	"context"
	"os"
	"os/signal"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/fatih/color"
)

func executeCLI(cm *config.ConfigManager, repos []config.LiveRepo) {
	onStartCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	mlt := newMultiplexer(onStartCtx, os.Stdout, cm, repos)
	mlt.start()

	color.Green("Execution completed")
}
