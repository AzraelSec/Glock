package engine

import (
	"context"
	"testing"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/runtime"
)

func TestDisconnectUnblocksOutputDuringInitialization(t *testing.T) {
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{"chatty": {Run: "i=0; while [ $i -lt 10000 ]; do echo line; i=$((i+1)); done"}}, On: &config.On{Startup: []string{"chatty"}}}
	s := OpenProject(&config.Project{Config: cfg, Root: t.TempDir()})
	defer s.Close()
	nextNotification(t, s)
	if err := s.Send(context.Background(), runtime.Command{ID: 1, Action: runtime.Initialize}); err != nil {
		t.Fatal(err)
	}
	waitEngine(t, func() bool { return len(s.stream) == cap(s.stream) })
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("blocked output prevented disconnect cleanup")
	}
	// Closing retains already queued lines for readers, then signals EOF.
	for range s.Output() {
	}
}

func TestSlowOutputConsumerReportsDroppedChunks(t *testing.T) {
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{"chatty": {Run: "i=0; while [ $i -lt 30000 ]; do echo line; i=$((i+1)); done"}}, On: &config.On{Startup: []string{"chatty"}}}
	s := OpenProject(&config.Project{Config: cfg, Root: t.TempDir()})
	defer s.Close()
	nextNotification(t, s)
	if err := s.Send(context.Background(), runtime.Command{ID: 1, Action: runtime.Initialize}); err != nil {
		t.Fatal(err)
	}
	waitEngine(t, func() bool { return s.OutputLoss().DroppedChunks > 0 })
	if got := s.snapshot(2).OutputLoss.DroppedChunks; got == 0 {
		t.Fatal("snapshot omitted output loss")
	}
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("slow consumer blocked shutdown")
	}
}

func TestInheritedPipeReportsTruncation(t *testing.T) {
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{"linger": {Run: "sleep 4 & echo done"}}, On: &config.On{Startup: []string{"linger"}}}
	s := OpenProject(&config.Project{Config: cfg, Root: t.TempDir()})
	defer s.Close()
	nextNotification(t, s)
	if err := s.Send(context.Background(), runtime.Command{ID: 1, Action: runtime.Initialize}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(6 * time.Second)
	for {
		select {
		case n := <-s.Events():
			if n.Result != nil {
				if n.Result.Error != "" {
					t.Fatal(n.Result.Error)
				}
				if s.OutputLoss().TruncatedDrains == 0 {
					t.Fatal("missing truncation warning")
				}
				return
			}
		case <-deadline:
			t.Fatal("pipe drain did not finish")
		}
	}
}
