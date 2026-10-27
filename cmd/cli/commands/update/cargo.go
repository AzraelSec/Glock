package update

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/internal/dir"
	"github.com/AzraelSec/glock/internal/shell"
)

var _ updater = cargoUpdater{}

type cargoUpdater struct{}

func (cargoUpdater) Tag() string {
	return "cargo"
}

func (cargoUpdater) Update(ctx context.Context, w io.Writer, path string) error {
	_, err := shell.NewSyncShell(ctx, shell.ShellOps{
		Cmd:      "cargo fetch",
		ExecPath: path,
	}).Start(w)
	return err
}

func (cargoUpdater) Infer(d dir.Directory) (bool, error) {
	files, err := d.Files()
	if err != nil {
		return false, err
	}

	ok := false
	for _, file := range files {
		if file != "Cargo.lock" && file != "Cargo.toml" {
			continue
		}
		ok = true
	}
	return ok, nil
}
