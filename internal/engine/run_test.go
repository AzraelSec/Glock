package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AzraelSec/glock/internal/config"
)

func TestRunOwnsCleanupWithoutFrontend(t *testing.T) {
	dir := t.TempDir()
	project := &config.Project{Root: dir, Config: &config.ConfigV2{
		Components: map[string]config.Component{
			"one":     {Run: "true", On: &config.On{Shutdown: []string{"check-sibling"}}},
			"bad":     {Run: "exit 7", On: &config.On{Shutdown: []string{"release"}}},
			"sibling": {Run: "while [ ! -f released ]; do sleep 0.01; done; touch finished"},
		},
		Tasks: map[string]config.Task{"release": {Run: "touch released"}, "check-sibling": {Run: "test -f finished && touch cleaned"}},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := StartRun(ctx, project, []string{"one", "bad", "sibling"})
	if err := run.Wait(); err == nil || !strings.Contains(err.Error(), "component bad failed") {
		t.Fatalf("run returned %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cleaned")); err != nil {
		t.Fatal("successful component cleanup did not follow sibling completion", err)
	}
}

func TestInterruptRetainsFailuresDuringComponentCleanup(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"first":  {Run: "exit 1", On: &config.On{Shutdown: []string{"hold"}}},
			"second": {Run: "while [ ! -f holding ]; do sleep .01; done; exit 9", On: &config.On{Shutdown: []string{"record"}}},
		},
		Tasks: map[string]config.Task{
			"hold":   {Run: "echo first >> cleaned; touch holding; while [ ! -f release ]; do sleep .01; done"},
			"record": {Run: "echo second >> cleaned"},
			"finish": {Run: "test -f release && test $(wc -l < cleaned) -eq 2 && touch finished"},
		},
		On: &config.On{Shutdown: []string{"finish"}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	run := StartRun(ctx, &config.Project{Root: dir, Config: cfg}, []string{"first", "second"})
	release := filepath.Join(dir, "release")
	t.Cleanup(func() { cancel(); _ = os.WriteFile(release, nil, 0600); run.Wait() })
	for _, name := range []string{"first", "second"} {
		select {
		case err := <-run.Failures:
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("expected %s failure, got %v", name, err)
			}
		case <-ctx.Done():
			t.Fatal("failure reporting blocked during rollback")
		}
	}
	cancel()
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	err := run.Wait()
	if err == nil || !strings.Contains(err.Error(), "first failed") || !strings.Contains(err.Error(), "second failed") {
		t.Fatalf("lost failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "finished")); err != nil {
		t.Fatal("application cleanup ran out of order", err)
	}
}

func TestInterruptDuringApplicationRollbackPreservesStartupFailure(t *testing.T) {
	dir := t.TempDir()
	project := &config.Project{Root: dir, Config: &config.ConfigV2{
		On: &config.On{Startup: []string{"fail"}, Shutdown: []string{"rollback"}},
		Tasks: map[string]config.Task{
			"fail":     {Run: "exit 7"},
			"rollback": {Run: "touch rolling; while [ ! -f release ]; do sleep .01; done; echo cleanup >> log"},
		},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	run := StartRun(ctx, project, nil)
	release := filepath.Join(dir, "release")
	t.Cleanup(func() { cancel(); _ = os.WriteFile(release, nil, 0600); run.Wait() })
	waitEngine(t, func() bool { _, err := os.Stat(filepath.Join(dir, "rolling")); return err == nil })
	cancel()
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run.Wait(); err == nil || !strings.Contains(err.Error(), "exited with code 7") {
		t.Fatalf("lost application startup failure: %v", err)
	}
	if log, err := os.ReadFile(filepath.Join(dir, "log")); err != nil || string(log) != "cleanup\n" {
		t.Fatalf("application rollback did not run exactly once: %q, %v", log, err)
	}
}
