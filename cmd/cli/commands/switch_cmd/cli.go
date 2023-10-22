package switchcmd

import (
	"errors"
	"fmt"

	"github.com/AzraelSec/glock/internal/runner"
	"github.com/AzraelSec/glock/pkg/git"
	"github.com/fatih/color"
)

type cli struct {
	*switchGit
}

func newCli(sg *switchGit) *cli {
	return &cli{switchGit: sg}
}
func (s *cli) printPlaneResults(results []runner.Result[struct{}]) {
	if len(results) != 0 {
		color.Green("Switches:")
	}

	errs := []struct {
		idx int
		err error
	}{}
	for i, rs := range results {
		if rs.Error != nil {
			if errors.Is(rs.Error, git.InvalidReferenceErr) {
				continue
			}

			errs = append(errs, struct {
				idx int
				err error
			}{
				idx: i,
				err: rs.Error,
			})
			continue
		}

		fmt.Printf("\t -> %s\n", s.repos[i].Name)
	}

	if len(errs) != 0 {
		color.Red("Errors:")
	}
	for _, err := range errs {
		fmt.Printf("\t -> %s {%s}\n", s.repos[err.idx].Name, err.err.Error())
	}
}

func (s *cli) run(target string, force bool) {
	results := s.performSwitch(target, force)
	printRichResults(s.repos, results)
}
