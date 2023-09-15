package updater

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/pkg/dir"
	"github.com/AzraelSec/glock/pkg/shell"
)

var _ Updater = pnpmBundler{}

type pnpmBundler struct{}

func (pnpmBundler) Tag() string {
	return "pnpm"
}

func (pnpmBundler) Update(ctx context.Context, w io.Writer, path string) error {
	_, err := shell.NewSyncShell(shell.ShellOps{
		Cmd:      "pnpm install",
		ExecPath: path,
		Ctx:      ctx,
	}).Start(w)
	return err
}

func (pnpmBundler) Infer(d dir.Directory) (bool, error) {
	files, err := d.Files()
	if err != nil {
		return false, err
	}

	ok := false
	for _, file := range files {
		if file != "pnpm-lock.yaml" {
			continue
		}
		ok = true
	}
	return ok, nil
}
