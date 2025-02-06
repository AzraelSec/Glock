package updater

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/shell"
)

var _ Updater = bundlerUpdater{}

type bundlerUpdater struct{}

func (bundlerUpdater) Tag() string {
	return "bundler"
}

func (bundlerUpdater) Update(ctx context.Context, w io.Writer, path string) error {
	_, err := shell.NewSyncShell(ctx, shell.ShellOps{
		Cmd:      "bundle install",
		ExecPath: path,
	}).Start(w)
	return err
}

func (bundlerUpdater) Infer(d dir.Directory) (bool, error) {
	files, err := d.Files()
	if err != nil {
		return false, err
	}

	ok := false
	for _, file := range files {
		if file != "Gemfile.lock" && file != "Gemfile" {
			continue
		}
		ok = true
	}
	return ok, nil
}
