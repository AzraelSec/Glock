package frontend_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/engine"
	"github.com/AzraelSec/glock/internal/frontend"
	"github.com/AzraelSec/glock/internal/orchestrator"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/runtime"
	"github.com/AzraelSec/glock/internal/task"
)

func waitFrontend(t *testing.T, condition func() bool) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for !condition() {
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatal("frontend state did not converge")
		}
	}
}

func TestEventClientLifecycleOutputAndShutdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "input"), []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.ConfigV2{Components: map[string]config.Component{"svc": {Run: "cat input; sleep 60", On: &config.On{Shutdown: []string{"cleanup"}}}}, Workspaces: map[string][]string{"dev": {"svc"}}, Tasks: map[string]config.Task{"cleanup": {Run: "touch cleaned"}, "hello": {Run: "echo task-output"}}}
	o := orchestrator.NewOrchestratorWithOptions(cfg, dir, orchestrator.Options{StopTimeout: 30 * time.Millisecond})
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	s := engine.NewSession(o)
	defer s.Close()
	c, err := frontend.New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if len(c.Workspaces()) != 3 || len(c.Components("dev")) != 1 || c.Components("dev")[0].Process != nil {
		t.Fatal("initial snapshot was not available before rendering")
	}
	if err := c.StartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	if err := c.SelectOutput(runtime.OutputTarget{Kind: runtime.ComponentOutput, Name: "svc"}, 1); err != nil {
		t.Fatal(err)
	}
	waitFrontend(t, func() bool {
		p := c.Components("dev")[0].Process
		return p != nil && p.State == process.Running && p.Output != nil && strings.Join(p.Output.Lines(), "\n") == "first"
	})
	if err := os.WriteFile(filepath.Join(dir, "input"), []byte("second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.RestartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	waitFrontend(t, func() bool {
		p := c.Components("dev")[0].Process
		return p != nil && p.Output != nil && strings.Join(p.Output.Lines(), "\n") == "second"
	})
	if err := c.RunTask("hello"); err != nil {
		t.Fatal(err)
	}
	if err := c.SelectOutput(runtime.OutputTarget{Kind: runtime.TaskOutput, Name: "hello"}, 2); err != nil {
		t.Fatal(err)
	}
	waitFrontend(t, func() bool {
		for _, tsk := range c.Tasks() {
			if tsk.Name == "hello" {
				return tsk.State == task.Finished && tsk.Output != nil && strings.Join(tsk.Output.Lines(), "\n") == "task-output"
			}
		}
		return false
	})
	if c.Components("dev")[0].Process.Output != nil {
		t.Fatal("previous output target remained attached")
	}
	if err := c.RunTask("missing"); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("command error not delivered: %v", err)
	}
	if err := c.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cleaned")); err != nil {
		t.Fatal("shutdown returned before cleanup", err)
	}
	if err := c.StartComponent("svc"); err == nil {
		t.Fatal("post-shutdown command succeeded")
	}
}

func TestDisconnectUnblocksPendingCommand(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{"setup": {Run: "touch entered; sleep 60"}}, Components: map[string]config.Component{"svc": {Run: "sleep 60", On: &config.On{Startup: []string{"setup"}}}}}
	o := orchestrator.NewOrchestratorWithOptions(cfg, dir, orchestrator.Options{StopTimeout: 30 * time.Millisecond})
	s := engine.NewSession(o)
	defer s.Close()
	c, err := frontend.New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	result := make(chan error, 1)
	go func() { result <- c.StartComponent("svc") }()
	waitFrontend(t, func() bool { _, err := os.Stat(filepath.Join(dir, "entered")); return err == nil })
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, runtime.ErrDisconnected) {
			t.Fatalf("expected disconnection: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending command did not unblock")
	}
}

func TestInitialSnapshotWaitHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := frontend.New(ctx, &silentSession{events: make(chan runtime.Notification)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
}

type silentSession struct{ events chan runtime.Notification }

func (s *silentSession) Send(context.Context, runtime.Command) error { return nil }
func (s *silentSession) Events() <-chan runtime.Notification         { return s.events }
