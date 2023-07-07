package updater

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/shell"
)

var _ Updater = npmBundler{}

type npmBundler struct{}

func (npmBundler) Tag() string {
	return "npm"
}

func (npmBundler) Update(ctx context.Context, w io.Writer, path string) error {
	_, err := shell.NewSyncShell(shell.ShellOps{
		Cmd:      "npm i",
		ExecPath: path,
		Ctx:      ctx,
	}).Start(w)
	return err
}

func (npmBundler) Infer(d dir.Directory) (bool, error) {
	files, err := d.Files()
	if err != nil {
		return false, err
	}

	ok := false
	for _, file := range files {
		if file != "package-lock.json" {
			continue
		}
		ok = true
	}
	return ok, nil
}
