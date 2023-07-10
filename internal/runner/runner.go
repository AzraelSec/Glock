package runner

import (
	"sync"
)

type Result[T any] struct {
	Res   T
	Error error
}

// Run executes f for each element in args, returning the results in the same order as args.
// Note: it will return an error as soon as any of the executions returns an error.
func Run[ArgsT any, ResT any](f func(ArgsT) (ResT, error), args []ArgsT) []Result[ResT] {
	var wg sync.WaitGroup
  largs := len(args)
	wg.Add(largs)

	results := make([]Result[ResT], largs)
	for i, arg := range args {
		go func(i int, arg ArgsT) {
			res, err := f(arg)
			results[i] = Result[ResT]{Res: res, Error: err}
			wg.Done()
		}(i, arg)
	}

	wg.Wait()
	return results
}
