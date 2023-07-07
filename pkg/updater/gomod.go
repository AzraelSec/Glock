package updater

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/pkg/shell"
	"github.com/AzraelSec/glock/pkg/utils"
)

var _ Updater = npmBundler{}

type gomodUpdater struct{}

func (gomodUpdater) Tag() string {
	return "go mod"
}

func (gomodUpdater) Update(ctx context.Context, w io.Writer, path string) error {
	_, err := shell.NewSyncShell(shell.ShellOps{
		Cmd:      "go mod tidy",
		ExecPath: path,
		Ctx:      ctx,
	}).Start(w)
	return err
}

func (gomodUpdater) Infer(d utils.Directory) (bool, error) {
	files, err := d.Files()
	if err != nil {
		return false, err
	}

	ok := false
	for _, file := range files {
		if file != "go.mod" {
			continue
		}
		ok = true
	}
	return ok, nil
}
