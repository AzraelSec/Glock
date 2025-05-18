package init_cmd

import (
	"io"
	"net/http"
	"os"
	"path"

	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/spf13/cobra"
)

func setupConfig(url, targetDir string) (string, error) {
	// fixme: `glock.yml` should not be hard-coded here!
	finalPath := path.Join(targetDir, "glock.yml")
	if !dir.DirExists(targetDir) {
		if err := os.Mkdir(targetDir, os.ModePerm); err != nil {
			return "", err
		}
	}

	file, err := os.Create(finalPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", err
	}

	return finalPath, nil
}

func remoteCommand(dm *dependency.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "remote",
		Short: "Use a remote configuration (via URL) to clone the configured repos",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := dm.GetGit()
			if err != nil {
				return err
			}

			dirPath := "./"
			if len(args) == 2 {
				dirPath = args[1]
			}
			setupConfig(args[0], dirPath)

			cm, err := dm.ConfigManagerFromFile(dirPath)
			if err != nil {
				return err
			}

			initFn := func(cloneOps git.CloneOps) (struct{}, error) {
				return struct{}{}, g.Clone(cloneOps)
			}

			initArgs := make([]git.CloneOps, 0, len(cm.Repos))
			for _, repo := range cm.Repos {
				initArgs = append(initArgs, git.CloneOps{
					Remote: repo.GitConfig.Remote,
					Path:   repo.GitConfig.Path,
					Refs:   repo.GitConfig.Refs,
				})
			}

			results := runner.Run(initFn, initArgs)

			for i, res := range results {
				logger := log.NewRepoLogger(os.Stdout, cm.Repos[i].Name)
				if res.Error != nil {
					logger.Error(res.Error.Error())
				} else {
					logger.Success("successfully cloned")
				}
			}

			return nil
		},
	}
}
