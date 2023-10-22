package start

import (
	"slices"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func StartFactory(cm *config.ConfigManager) *cobra.Command {
	var filteredRepos *[]string
	var useCli *bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the development stack 🚀",
		Run: func(cmd *cobra.Command, args []string) {
			repos := []config.LiveRepo{}
			if filteredRepos == nil || len(*filteredRepos) == 0 {
				repos = cm.Repos
			} else {
				for _, repo := range cm.Repos {
					if slices.Contains(*filteredRepos, repo.Name) {
						repos = append(repos, repo)
					}
				}
			}

			if len(repos) == 0 {
				color.Red("You cannot start your stack with no repos selected")
				return
			}

			executableRepo := []config.LiveRepo{}
			disposableRepo := []config.LiveRepo{}
			for _, repo := range repos {
				if len(repo.Config.OnStart) != 0 {
					executableRepo = append(executableRepo, repo)
				}
				if repo.Config.OnStop != "" {
					disposableRepo = append(disposableRepo, repo)
				}
			}

			executableService := []config.Services{}
			disposableService := []config.Services{}

			for _, srv := range cm.Services {
				if srv.Cmd != "" {
					executableService = append(executableService, srv)
				}
				if srv.Dispose != "" {
					disposableService = append(disposableService, srv)
				}
			}

			if *useCli {
				executeCLI(cm, executableRepo, disposableRepo, executableService, disposableService)
			} else {
				executeTUI(cm, executableRepo, disposableRepo, executableService, disposableService)
			}
		},
	}

	filteredRepos = cmd.Flags().StringArrayP("repos", "r", nil, "list of repository you want to start")
	useCli = cmd.Flags().Bool("cli", false, "use cli instead of tui")

	return cmd
}
