package runner

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/pkg/git"
)

type RunnerWrapperFunc[T any, O any] func(rd config.LiveRepo, args O)

type RunnerFuncWrapperInfo[R any] struct {
	Context context.Context
	Output  io.Writer
	Git     git.Git
	Result  chan<- RunnerResult[R]
}

func WrapRunnerFunc[T any, O any](f RunnerFunc[T, O], i RunnerFuncWrapperInfo[T]) RunnerWrapperFunc[T, O] {
	return func(rd config.LiveRepo, args O) {
		// TODO: Try remove the result and make this closure to ship the inner
		// function result to the Result channel
		f(RunnerInfo[T, O]{
			Context:  i.Context,
			Output:   i.Output,
			Git:      i.Git,
			Result:   i.Result,
			Args:     args,
			RepoData: rd,
		})
	}
}
