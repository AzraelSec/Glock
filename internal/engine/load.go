package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/orchestrator"
)

// Load is application bootstrap, outside the presentation layer. It creates
// and initializes the engine before exposing its event-only session.
func Load(dm *dependency.Manager) (*Session, error) {
	return LoadContext(context.Background(), dm)
}

// LoadContext owns interrupted initialization too, before any frontend exists.
func LoadContext(ctx context.Context, dm *dependency.Manager) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	project, err := dm.LoadProject()
	if err != nil {
		return nil, err
	}
	cfg := project.Config
	orch := orchestrator.NewOrchestrator(cfg, project.Root)
	initializationDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = orch.Stop()
		case <-initializationDone:
		}
	}()
	err = orch.Start()
	close(initializationDone)
	<-watcherDone
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("start orchestrator: %w", err), orch.Close())
	}
	return NewSession(orch), nil
}
