package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/orchestrator"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/runtime"
)

// Run owns one batch execution. Output is available from initialization through
// cleanup. Wait succeeds on a clean interrupt and retains component and cleanup failures.
type Run struct {
	Output   <-chan process.OutputLine
	Failures <-chan error
	done     chan struct{}
	err      error
	stats    *outputStats
}

func (r *Run) Wait() error { <-r.done; return r.err }
func (r *Run) OutputLoss() runtime.OutputLoss {
	return runtime.OutputLoss{DroppedChunks: r.stats.dropped.Load(), TruncatedDrains: r.stats.truncated.Load()}
}

func StartRun(ctx context.Context, project *config.Project, names []string, failFast bool) *Run {
	// Shutdown hooks still produce output after ctx is canceled.
	ctx, cancel := context.WithCancel(ctx)
	var opts orchestrator.Options
	if failFast {
		opts.OnStartupFailure = cancel
	}
	orch, stats, lines := streamingProject(context.Background(), project, opts)
	names = slices.Clone(names)
	reports := make(chan error, len(names))
	r := &Run{Output: lines, Failures: reports, done: make(chan struct{}), stats: stats}
	go func() {
		r.err = supervise(ctx, cancel, orch, names, failFast, reports)
		close(reports)
		close(lines)
		close(r.done)
	}()
	return r
}

func supervise(ctx context.Context, cancel context.CancelFunc, orch *orchestrator.Orchestrator, names []string, failFast bool, reports chan<- error) (err error) {
	var workers sync.WaitGroup
	stopWatcher := context.AfterFunc(ctx, func() { _ = orch.Stop() })
	defer func() {
		cancel()
		stopWatcher()
		err = errors.Join(err, orch.Close())
	}()
	if ctx.Err() != nil {
		return nil
	}
	if err := orch.StartBatch(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, orchestrator.ErrShuttingDown) {
			return nil
		}
		return err
	}

	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(names))
	slots := make(chan struct{}, 8)
	for _, name := range names {
		workers.Go(func() {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			if ctx.Err() != nil {
				<-slots
				return
			}
			exit, startErr := orch.StartTrackedComponent(name)
			<-slots
			var failure error
			switch {
			case errors.Is(startErr, context.Canceled), errors.Is(startErr, orchestrator.ErrShuttingDown):
				return
			case startErr != nil:
				failure = fmt.Errorf("component %s: %w", name, startErr)
			case <-exit == process.Failed:
				failure = fmt.Errorf("component %s failed", name)
			default:
				return
			}
			// Always consume the launch's terminal outcome. Stop resolves live
			// launches as Stopped; cancellation must not erase a prior Failed result.
			if failFast {
				cancel()
			}
			reports <- failure
			if !failFast {
				if cleanupErr := orch.CleanupFailedComponent(name); cleanupErr != nil {
					failure = errors.Join(failure, fmt.Errorf("component %s cleanup: %w", name, cleanupErr))
				}
			}
			results <- result{name, failure}
		})
	}
	workers.Wait()
	close(results)
	failures := make(map[string]error)
	for outcome := range results {
		failures[outcome.name] = outcome.err
	}
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(failures)) {
		errs = append(errs, failures[name])
	}
	return errors.Join(errs...)
}
