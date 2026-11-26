package engine

import (
	"context"
	"errors"
	"fmt"
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

func StartRun(ctx context.Context, project *config.Project, names []string) *Run {
	// Shutdown hooks still produce output after ctx is canceled.
	orch, stats, lines := streamingProject(context.Background(), project)
	names = slices.Clone(names)
	slices.Sort(names)
	reports := make(chan error, len(names))
	r := &Run{Output: lines, Failures: reports, done: make(chan struct{}), stats: stats}
	go func() {
		r.err = supervise(ctx, orch, names, reports)
		close(reports)
		close(lines)
		close(r.done)
	}()
	return r
}

func supervise(ctx context.Context, orch *orchestrator.Orchestrator, names []string, reports chan<- error) (err error) {
	var workers sync.WaitGroup
	// Cancellation requests shutdown immediately, including during startup.
	// Closing the orchestrator below is the single place that awaits cleanup.
	stopWatcher := context.AfterFunc(ctx, func() { _ = orch.Stop() })
	defer func() {
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

	failures := make([]error, len(names))
	slots := make(chan struct{}, 8)
	for i, name := range names {
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
			// Wait for the terminal outcome even during shutdown to retain
			// failures that were recorded before cancellation.
			case <-exit == process.Failed:
				failure = fmt.Errorf("component %s failed", name)
			default:
				return
			}
			reports <- failure
			if cleanupErr := orch.CleanupFailedComponent(name); cleanupErr != nil {
				failure = errors.Join(failure, fmt.Errorf("component %s cleanup: %w", name, cleanupErr))
			}
			failures[i] = failure
		})
	}
	workers.Wait()
	return errors.Join(failures...)
}
