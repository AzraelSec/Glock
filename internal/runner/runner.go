package runner

import (
	"context"
	"io"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/pkg/git"
)

type RunnerResult[T any] struct {
	Error  error
	Result T
}

func NewRunnerResult[T any](err error, result T) RunnerResult[T] {
	return RunnerResult[T]{
		Error:  err,
		Result: result,
	}
}

type RunnerInfo[ResultT any, ArgsT any] struct {
	Context  context.Context
	Output   io.Writer
	Git      git.Git
	RepoData config.LiveRepo
	// TODO: remove this field that is not useful at all!
	Result   chan<- RunnerResult[ResultT]
	Args     ArgsT
}

type RunnerFunc[T any, O any] func(RunnerInfo[T, O])
