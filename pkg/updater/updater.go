package updater

import (
	"context"
	"errors"
	"io"

	"github.com/AzraelSec/glock/pkg/utils"
)

const IGNORE_TAG = "ignore"
var NoUpdaterErr = errors.New("no matching updater found")

var updaters = []Updater{
	yarnUpdater{},
	npmBundler{},
	bundlerUpdater{},
	gomodUpdater{},
}

type Updater interface {
	Tag() string
	Infer(utils.Directory) (bool, error)
	Update(ctx context.Context, output io.Writer, path string) error
}

func Infer(d utils.Directory) (Updater, error) {
	for _, updater := range updaters {
		if ok, _ := updater.Infer(d); ok {
			return updater, nil
		}
	}
	return nil, NoUpdaterErr
}

func MatchByTag(tag string) (Updater, error) {
	for _, updater := range updaters {
		if tag == IGNORE_TAG {
			continue
		}
		if updater.Tag() == tag {
			return updater, nil
		}
	}
	return nil, NoUpdaterErr
}
