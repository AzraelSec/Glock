package runner

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/pkg/git"
)

type RunnerWrapperFunc[ArgsT any] func(rd config.LiveRepo, args ArgsT)

type RunnerFuncWrapperInfo[ResultT any] struct {
	Context context.Context
	Output  io.Writer
	Git     git.Git
	Result  chan<- RunnerResult[ResultT]
}

func WrapRunnerFunc[ResultT any, ArgsT any](f RunnerFunc[ResultT, ArgsT], i RunnerFuncWrapperInfo[ResultT]) RunnerWrapperFunc[ArgsT] {
	return func(rd config.LiveRepo, args ArgsT) {
		// TODO: Try remove the result and make this closure to ship the inner
		// function result to the Result channel
		f(RunnerInfo[ResultT, ArgsT]{
			Context:  i.Context,
			Output:   i.Output,
			Git:      i.Git,
			Result:   i.Result,
			Args:     args,
			RepoData: rd,
		})
	}
}
