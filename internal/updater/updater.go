package updater

import (
	"context"
	"errors"
	"io"

	"github.com/AzraelSec/glock/internal/dir"
)

const ignoreTag = "_ignore_"

var ErrNoUpdater = errors.New("no matching updater found")

var updaters = []Updater{
	yarnUpdater{},
	npmBundler{},
	pnpmBundler{},
	bundlerUpdater{},
	gomodUpdater{},
}

type Updater interface {
	Tag() string
	Infer(dir.Directory) (bool, error)
	Update(ctx context.Context, output io.Writer, path string) error
}

func Infer(d dir.Directory) (Updater, error) {
	for _, updater := range updaters {
		if ok, _ := updater.Infer(d); ok {
			return updater, nil
		}
	}
	return nil, ErrNoUpdater
}

func MatchByTag(tag string) (Updater, error) {
	for _, updater := range updaters {
		if updater.Tag() == tag {
			return updater, nil
		}
	}
	return nil, ErrNoUpdater
}

func IsIgnoreTag(tag string) bool {
	return tag == ignoreTag
}
