package update

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/shell"
)

var _ updater = gomodUpdater{}

type gomodUpdater struct{}

func (gomodUpdater) Tag() string {
	return "go mod"
}

func (gomodUpdater) Update(ctx context.Context, w io.Writer, path string) error {
	_, err := shell.NewSyncShell(ctx, shell.ShellOps{
		Cmd:      "go get",
		ExecPath: path,
	}).Start(w)
	return err
}

func (gomodUpdater) Infer(d dir.Directory) (bool, error) {
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
