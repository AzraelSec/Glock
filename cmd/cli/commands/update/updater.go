package update

import (
	"context"
	"errors"
	"io"

	"github.com/AzraelSec/glock/internal/dir"
)

const ignoreTag = "_ignore_"

var ErrNoUpdater = errors.New("no matching updater found")

var updaters = []updater{
	yarnUpdater{},
	npmBundler{},
	pnpmBundler{},
	bundlerUpdater{},
	gomodUpdater{},
}

type updater interface {
	Tag() string
	Infer(dir.Directory) (bool, error)
	Update(ctx context.Context, output io.Writer, path string) error
}

func inferUpdater(d dir.Directory) (updater, error) {
	for _, updater := range updaters {
		if ok, _ := updater.Infer(d); ok {
			return updater, nil
		}
	}
	return nil, ErrNoUpdater
}

func matchUpdaterByTag(tag string) (updater, error) {
	for _, updater := range updaters {
		if updater.Tag() == tag {
			return updater, nil
		}
	}
	return nil, ErrNoUpdater
}

func isIgnoreUpdaterTag(tag string) bool {
	return tag == ignoreTag
}
