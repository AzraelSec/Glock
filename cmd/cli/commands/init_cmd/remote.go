package init_cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/git"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/runner"
	"github.com/spf13/cobra"
)

func setupConfig(url, targetDir string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download config: HTTP %s", resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// Validate before creating files or attempting acquisition.
	finalPath := path.Join(targetDir, "glock.yml")
	cfg, err := config.ParseConfigV2(raw)
	if err != nil {
		return "", err
	}
	if _, err = config.RepositoryCatalog(&config.Project{Config: cfg, Root: targetDir, Path: finalPath}); err != nil {
		return "", err
	}
	if !dir.DirExists(targetDir) {
		if err = os.MkdirAll(targetDir, 0755); err != nil {
			return "", err
		}
	}
	file, err := os.OpenFile(finalPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(raw)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(finalPath)
		return "", writeErr
	}
	if closeErr != nil {
		_ = os.Remove(finalPath)
		return "", closeErr
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
			if _, err := setupConfig(args[0], dirPath); err != nil {
				return err
			}

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

			var failures []error
			for i, res := range results {
				logger := log.NewRepoLogger(os.Stdout, cm.Repos[i].Name)
				if res.Error != nil {
					logger.Error("%s", res.Error)
					failures = append(failures, fmt.Errorf("%s: %w", cm.Repos[i].Name, res.Error))
				} else {
					logger.Success("successfully cloned")
				}
			}

			if len(failures) > 0 {
				return fmt.Errorf("%w\nConfiguration retained at %s; after fixing the errors, run glock init local from its directory, or set GLOCK_CONFIG_PATH to that file", errors.Join(failures...), cm.ConfigPath)
			}
			return nil
		},
	}
}
