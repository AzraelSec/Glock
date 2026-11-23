package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/orchestrator"
	"github.com/AzraelSec/glock/internal/runtime"
)

func nextNotification(t *testing.T, s runtime.Session) runtime.Notification {
	t.Helper()
	select {
	case n, ok := <-s.Events():
		if !ok {
			t.Fatal("session disconnected")
		}
		return n
	case <-time.After(3 * time.Second):
		t.Fatal("notification timed out")
		return runtime.Notification{}
	}
}
func waitEngine(t *testing.T, condition func() bool) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for !condition() {
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatal("condition timed out")
		}
	}
}

func TestSnapshotHasNoEngineAliases(t *testing.T) {
	cfg := &config.ConfigV2{Components: map[string]config.Component{"svc": {Run: "sleep 60", EnvFilenames: []string{".env"}}}, Workspaces: map[string][]string{"dev": {"svc"}}}
	o := orchestrator.NewOrchestrator(cfg, t.TempDir())
	if err := o.StartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	s := NewSession(o)
	defer s.Close()
	n := nextNotification(t, s)
	if n.Snapshot == nil {
		t.Fatal("first event was not a snapshot")
	}
	n.Snapshot.Workspaces[2].Components[0] = "mutated"
	n.Snapshot.Components[0].Definition.EnvFilenames[0] = "mutated"
	n.Snapshot.Components[0].Process.PID = -99
	if got := o.Workspaces()[2].Components[0]; got != "svc" {
		t.Fatalf("workspace aliased: %q", got)
	}
	if got := o.Components("dev")[0]; got.Component.EnvFilenames[0] != ".env" || got.Process.PID <= 0 {
		t.Fatalf("component aliased: %+v", got)
	}
}

func TestSlowFrontendRetainsResultsAndCoalescesState(t *testing.T) {
	o := orchestrator.NewOrchestrator(&config.ConfigV2{}, t.TempDir())
	s := NewSession(o)
	defer s.Close()
	nextNotification(t, s)
	for id := uint64(1); id <= 40; id++ {
		if err := s.Send(context.Background(), runtime.Command{ID: id, Action: runtime.RunTask, Target: fmt.Sprintf("missing-%d", id)}); err != nil {
			t.Fatal(err)
		}
	}
	// Delay consumption until completions have backed up. This is a state
	// predicate, not a sleep that assumes the scheduler has run.
	waitEngine(t, func() bool { return len(s.results) >= 10 })
	if len(s.snapshots) > 1 {
		t.Fatal("state backlog is unbounded")
	}
	seen := make(map[uint64]bool)
	for len(seen) < 40 {
		n := nextNotification(t, s)
		if n.Result == nil {
			continue
		}
		r := n.Result
		if seen[r.ID] || r.ID == 0 || r.ID > 40 {
			t.Fatalf("unexpected duplicate result: %+v", r)
		}
		if r.Error == "" || r.Target != fmt.Sprintf("missing-%d", r.ID) {
			t.Fatalf("uncorrelated result: %+v", r)
		}
		seen[r.ID] = true
	}
	for {
		n := nextNotification(t, s)
		if n.Snapshot != nil && n.Snapshot.Sequence > 1 {
			break
		}
	}
}

func TestShutdownLaneCancelsSaturatedWorkers(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Components: make(map[string]config.Component), Tasks: map[string]config.Task{"setup": {Run: "touch entered; sleep 60"}}}
	for i := range 8 {
		cfg.Components[fmt.Sprintf("svc-%d", i)] = config.Component{Run: "touch main-started", On: &config.On{Startup: []string{"setup"}}}
	}
	o := orchestrator.NewOrchestratorWithOptions(cfg, dir, orchestrator.Options{StopTimeout: 30 * time.Millisecond})
	s := NewSession(o)
	defer s.Close()
	nextNotification(t, s)
	for i := range 8 {
		if err := s.Send(context.Background(), runtime.Command{ID: uint64(i + 1), Action: runtime.StartComponent, Target: fmt.Sprintf("svc-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	waitEngine(t, func() bool { _, err := os.Stat(filepath.Join(dir, "entered")); return err == nil })
	if err := s.Send(context.Background(), runtime.Command{ID: 99, Action: runtime.Shutdown}); err != nil {
		t.Fatal(err)
	}
	results := make(map[uint64]runtime.CommandResult)
	for len(results) < 9 {
		n := nextNotification(t, s)
		if n.Result != nil {
			results[n.Result.ID] = *n.Result
		}
	}
	if results[99].Error != "" {
		t.Fatal(results[99].Error)
	}
	if _, err := os.Stat(filepath.Join(dir, "main-started")); !os.IsNotExist(err) {
		t.Fatal("main process launched during shutdown")
	}
	if err := o.RunTask("setup"); !errors.Is(err, orchestrator.ErrShuttingDown) {
		t.Fatalf("work still admitted: %v", err)
	}
}

func TestDisconnectWithoutConsumerCompletesCleanup(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{"cleanup": {Run: "touch cleaned"}}, Components: map[string]config.Component{"svc": {Run: "sleep 60", On: &config.On{Shutdown: []string{"cleanup"}}}}}
	o := orchestrator.NewOrchestratorWithOptions(cfg, dir, orchestrator.Options{StopTimeout: 30 * time.Millisecond})
	if err := o.StartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	s := NewSession(o)
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect blocked on frontend delivery")
	}
	if _, err := os.Stat(filepath.Join(dir, "cleaned")); err != nil {
		t.Fatal("cleanup missing", err)
	}
	if err := s.Send(context.Background(), runtime.Command{Action: runtime.RunTask}); !errors.Is(err, runtime.ErrDisconnected) {
		t.Fatalf("disconnected command admitted: %v", err)
	}
	if _, ok := <-s.Events(); ok {
		t.Fatal("notification channel remains open")
	}
}

func TestOutputSelectionRevisionRejectsStaleCommands(t *testing.T) {
	o := orchestrator.NewOrchestrator(&config.ConfigV2{}, t.TempDir())
	s := NewSession(o)
	defer s.Close()
	newest := runtime.OutputTarget{Kind: runtime.TaskOutput, Name: "new"}
	if err := s.execute(runtime.Command{Action: runtime.SelectOutput, Output: newest, OutputRevision: 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.execute(runtime.Command{Action: runtime.SelectOutput, Output: runtime.OutputTarget{Kind: runtime.ComponentOutput, Name: "old"}, OutputRevision: 1}); err != nil {
		t.Fatal(err)
	}
	snapshot := s.snapshot(2)
	if snapshot.Output.Target != newest || snapshot.Output.Revision != 2 {
		t.Fatalf("stale selection replaced newest: %+v", snapshot.Output)
	}
}

func TestLoadFailedInitializationCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glock.yml")
	raw := `version: 2
tasks:
  fail:
    run: exit 5
  cleanup:
    run: touch cleaned
on:
  startup: [fail]
  shutdown: [cleanup]
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.ConfigPathEnv, path)
	if _, err := Load(dependency.NewManager()); err == nil {
		t.Fatal("startup failure accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "cleaned")); err != nil {
		t.Fatal("failed startup did not clean up", err)
	}
}

func TestLoadContextCancellationCleansUpBeforeFrontendExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "glock.yml")
	raw := `version: 2
tasks:
  setup:
    run: "trap 'exit 130' INT; touch entered; while :; do sleep 0.01; done"
  cleanup:
    run: touch cleaned
on:
  startup: [setup]
  shutdown: [cleanup]
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.ConfigPathEnv, path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := LoadContext(ctx, dependency.NewManager()); result <- err }()
	waitEngine(t, func() bool { _, err := os.Stat(filepath.Join(dir, "entered")); return err == nil })
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("initialization cancellation missing: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("initialization did not cancel")
	}
	if _, err := os.Stat(filepath.Join(dir, "cleaned")); err != nil {
		t.Fatal("interrupted initialization did not clean up", err)
	}
}

func TestSnapshotOmitsEnvironmentValues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PRIVATE_SENTINEL=supersecretvalue\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.ConfigV2{Components: map[string]config.Component{"svc": {Run: "sleep 10", EnvFilenames: []string{".env"}}}}
	o := orchestrator.NewOrchestrator(cfg, dir)
	if err := o.StartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	s := NewSession(o)
	defer s.Close()
	data, err := json.Marshal(s.snapshot(1))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("supersecretvalue")) || bytes.Contains(data, []byte("PRIVATE_SENTINEL")) {
		t.Fatalf("environment leaked: %s", data)
	}
}
