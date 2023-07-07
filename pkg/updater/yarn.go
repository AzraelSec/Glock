package updater

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/pkg/shell"
	"github.com/AzraelSec/glock/pkg/utils"
)

var _ Updater = yarnUpdater{}

type yarnUpdater struct{}

func (yarnUpdater) Tag() string {
	return "yarn"
}

func (yarnUpdater) Update(ctx context.Context, w io.Writer,  path string) error {
	_, err := shell.NewSyncShell(shell.ShellOps{
		Cmd:      "yarn",
		ExecPath: path,
		Ctx:      ctx,
	}).Start(w)
	return err
}

func (yarnUpdater) Infer(d utils.Directory) (bool, error) {
	files, err := d.Files()
	if err != nil {
		return false, err
	}

	ok := false
	for _, file := range files {
		if file != "yarn.lock" {
			continue
		}
		ok = true
	}
	return ok, nil
}

