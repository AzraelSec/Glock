package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/task"
)

func waitLifecycle(t *testing.T, condition func() bool) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for !condition() {
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatal("condition did not become true")
		}
	}
}
func readMarker(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func receiveAction(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not settle")
		return nil
	}
}

func TestApplicationInitializationOrderingAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			second := `test "$(cat sequence)" = first && printf second >> sequence`
			if fail {
				second = "exit 42"
			}
			cfg := &config.ConfigV2{Tasks: map[string]config.Task{
				"first": {Run: "printf first > sequence"}, "second": {Run: second},
				"cleanup": {Run: "printf cleanup >> sequence"},
			}, On: &config.On{Startup: []string{"first", "second"}, Shutdown: []string{"cleanup"}}}
			o := NewOrchestrator(cfg, dir)
			defer o.Close()
			sub := o.bus.Subscribe(nil)
			defer sub.Cancel()
			err := o.Start()
			if fail {
				if err == nil {
					t.Fatal("startup failure was ignored")
				}
				if got := readMarker(t, dir, "sequence"); got != "firstcleanup" {
					t.Fatalf("rollback order: %q", got)
				}
				if err := o.Close(); err != nil {
					t.Fatal(err)
				}
				for e := range sub.Events() {
					if e.Name == event.ApplicationStarted {
						t.Fatal("failed initialization announced success")
					}
				}
				if got := readMarker(t, dir, "sequence"); got != "firstcleanup" {
					t.Fatalf("rollback ran twice: %q", got)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if got := readMarker(t, dir, "sequence"); got != "firstsecond" {
					t.Fatalf("startup returned before hooks: %q", got)
				}
				if err := o.Stop(); err != nil {
					t.Fatal(err)
				}
				if got := readMarker(t, dir, "sequence"); got != "firstsecondcleanup" {
					t.Fatalf("shutdown order: %q", got)
				}
			}
		})
	}
}

func TestRestartReloadsEnvironmentAndUsesHooks(t *testing.T) {
	dir := t.TempDir()
	writeEnv := func(value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("GLOCK_RESTART_TEST="+value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeEnv("one")
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{
		"up":   {Run: `printf 'up:%s\n' "$GLOCK_RESTART_TEST" >> sequence`},
		"down": {Run: `printf 'down:%s\n' "$GLOCK_RESTART_TEST" >> sequence`},
	}, Components: map[string]config.Component{"svc": {Run: "sleep 60", EnvFilenames: []string{".env"}, On: &config.On{Startup: []string{"up"}, Shutdown: []string{"down"}}}}}
	o := NewOrchestrator(cfg, dir)
	defer o.Close()
	if err := o.RestartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	writeEnv("two")
	if err := o.RestartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	if err := o.Stop(); err != nil {
		t.Fatal(err)
	}
	if got := readMarker(t, dir, "sequence"); got != "up:one\ndown:one\nup:two\ndown:two\n" {
		t.Fatalf("lifecycle/environment order: %q", got)
	}
}

func TestCleanupAfterExitContinuesFailuresAndRunsOnce(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{
		"fail": {Run: "exit 31"}, "cleanup": {Run: "printf cleaned >> sequence"},
	}, Components: map[string]config.Component{"svc": {Run: "exit 2", On: &config.On{Shutdown: []string{"fail", "cleanup"}}}}}
	o := NewOrchestrator(cfg, dir)
	defer o.Close()
	if err := o.StartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	waitLifecycle(t, func() bool { p, ok := o.procMgr.Get("svc"); return ok && p.State == process.Failed })
	if err := o.StopComponent("svc"); err == nil || !strings.Contains(err.Error(), "31") {
		t.Fatalf("cleanup error missing: %v", err)
	}
	if got := readMarker(t, dir, "sequence"); got != "cleaned" {
		t.Fatalf("cleanup skipped: %q", got)
	}
	if err := o.Stop(); err != nil {
		t.Fatal(err)
	}
	if got := readMarker(t, dir, "sequence"); got != "cleaned" {
		t.Fatalf("cleanup ran twice: %q", got)
	}
}

func TestShutdownCancelsStartupAndOrdinaryTasks(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{
		"setup":    {Run: "touch setup-entered; sleep 60"},
		"ordinary": {Run: "sleep 60"}, "cleanup": {Run: "printf cleaned > sequence"},
	}, Components: map[string]config.Component{"svc": {Run: "touch main-started; sleep 60", On: &config.On{Startup: []string{"setup"}, Shutdown: []string{"cleanup"}}}}}
	o := NewOrchestrator(cfg, dir)
	defer o.Close()
	if err := o.RunTask("ordinary"); err != nil {
		t.Fatal(err)
	}
	started := make(chan error, 1)
	go func() { started <- o.StartComponent("svc") }()
	waitLifecycle(t, func() bool { _, err := os.Stat(filepath.Join(dir, "setup-entered")); return err == nil })
	if err := o.StartComponent("svc"); err == nil {
		t.Fatal("duplicate setup was admitted")
	}
	if err := o.StopComponent("svc"); err == nil {
		t.Fatal("overlapping stop was admitted")
	}
	if err := o.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := receiveAction(t, started); !errors.Is(err, context.Canceled) {
		t.Fatalf("startup was not canceled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main-started")); !os.IsNotExist(err) {
		t.Fatal("main process started after cancellation")
	}
	if got := readMarker(t, dir, "sequence"); got != "cleaned" {
		t.Fatalf("rollback missing: %q", got)
	}
	ordinary, _ := o.taskMgr.Get("ordinary")
	if ordinary.State != task.Failed {
		t.Fatalf("ordinary task remains %s", ordinary.State)
	}
	for _, err := range []error{o.StartComponent("svc"), o.RestartComponent("svc"), o.RunTask("ordinary"), o.StartWorkspace("[All]"), o.Start()} {
		if !errors.Is(err, ErrShuttingDown) {
			t.Fatalf("new work was accepted: %v", err)
		}
	}
}

func TestHookDeadlineTerminatesBeforeRollback(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{
		"hang": {Run: "sleep 60"}, "cleanup": {Run: "touch cleaned"},
	}, Components: map[string]config.Component{"svc": {Run: "sleep 60", On: &config.On{Startup: []string{"hang"}, Shutdown: []string{"cleanup"}}}}}
	o := NewOrchestrator(cfg, dir)
	defer o.Close()
	o.hookTimeout = 50 * time.Millisecond
	if err := o.StartComponent("svc"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline missing: %v", err)
	}
	tsk, _ := o.taskMgr.Get("svc/hang")
	if tsk.State != task.Failed {
		t.Fatalf("hung task remains %s", tsk.State)
	}
	if _, err := os.Stat(filepath.Join(dir, "cleaned")); err != nil {
		t.Fatal("rollback did not complete", err)
	}
}

func TestConcurrentShutdownSharesErrorsAndCompletesAllHooks(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{
		"fail": {Run: "exit 19"}, "cleanup": {Run: "printf component >> sequence"},
		"application": {Run: "printf application >> sequence"},
	}, Components: map[string]config.Component{"svc": {Run: "sleep 60", On: &config.On{Shutdown: []string{"fail", "cleanup"}}}}, On: &config.On{Shutdown: []string{"application"}}}
	o := NewOrchestrator(cfg, dir)
	defer o.Close()
	if err := o.StartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 4)
	for range 4 {
		go func() { results <- o.Stop() }()
	}
	var first error
	for range 4 {
		err := receiveAction(t, results)
		if err == nil {
			t.Fatal("shutdown swallowed cleanup error")
		}
		if first == nil {
			first = err
		}
		if err != first {
			t.Fatal("callers received different results")
		}
	}
	if got := readMarker(t, dir, "sequence"); got != "componentapplication" {
		t.Fatalf("cleanup ordering/repetition: %q", got)
	}
}

func TestHostEnvironmentOverridesFilesForHooksAndProcess(t *testing.T) {
	t.Setenv("GLOCK_HOST_TEST", "host")
	dir := t.TempDir()
	for name, value := range map[string]string{".env": "first", ".env.local": "last"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("GLOCK_HOST_TEST=file\nGLOCK_FILES_TEST="+value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{"env": {Run: `printf '%s/%s' "$GLOCK_HOST_TEST" "$GLOCK_FILES_TEST" > hook.env`}}, Components: map[string]config.Component{"svc": {Run: `printf '%s/%s' "$GLOCK_HOST_TEST" "$GLOCK_FILES_TEST" > process.env`, EnvFilenames: []string{".env", ".env.local"}, On: &config.On{Startup: []string{"env"}}}}}
	o := NewOrchestrator(cfg, dir)
	defer o.Close()
	if err := o.StartComponent("svc"); err != nil {
		t.Fatal(err)
	}
	waitLifecycle(t, func() bool { p, ok := o.procMgr.Get("svc"); return ok && p.State == process.Stopped })
	for _, name := range []string{"hook.env", "process.env"} {
		if got := readMarker(t, dir, name); got != "host/last" {
			t.Fatalf("%s: %q", name, got)
		}
	}
}

func TestShutdownRetainsRollbackErrorsFromCancelledStartup(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{
		"setup": {Run: "touch entered; sleep 60"}, "cleanup": {Run: "exit 37"},
	}, Components: map[string]config.Component{"svc": {Run: "sleep 60", On: &config.On{Startup: []string{"setup"}, Shutdown: []string{"cleanup"}}}}}
	o := NewOrchestratorWithOptions(cfg, dir, Options{StopTimeout: 30 * time.Millisecond})
	defer o.Close()
	result := make(chan error, 1)
	go func() { result <- o.StartComponent("svc") }()
	waitLifecycle(t, func() bool { _, err := os.Stat(filepath.Join(dir, "entered")); return err == nil })
	if err := o.Stop(); err == nil || !strings.Contains(err.Error(), "37") {
		t.Fatalf("rollback error missing from shutdown: %v", err)
	}
	if err := receiveAction(t, result); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "37") {
		t.Fatalf("startup/rollback errors lost: %v", err)
	}
}

func TestPairedApplicationRollbackAndShutdown(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			dir := t.TempDir()
			second := "echo two >> order"
			if fail {
				second = "exit 3"
			}
			cfg := &config.ConfigV2{Tasks: map[string]config.Task{"one": {Run: "echo one >> order"}, "two": {Run: second}, "undo-one": {Run: "echo undo-one >> order"}, "undo-two": {Run: "echo undo-two >> order"}}, On: &config.On{Steps: []config.HookStep{{Startup: "one", Cleanup: "undo-one"}, {Startup: "two", Cleanup: "undo-two"}}}}
			o := NewOrchestrator(cfg, dir)
			err := o.Start()
			if (err != nil) != fail {
				t.Fatalf("start: %v", err)
			}
			_ = o.Stop()
			_ = o.Close()
			data, err := os.ReadFile(filepath.Join(dir, "order"))
			if err != nil {
				t.Fatal(err)
			}
			want := "one\ntwo\nundo-two\nundo-one\n"
			if fail {
				want = "one\nundo-one\n"
			}
			if string(data) != want {
				t.Fatalf("got %q want %q", data, want)
			}
		})
	}
}

func TestPairedComponentRollback(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{"up": {Run: "echo up >> order"}, "fail": {Run: "exit 2"}, "down": {Run: "echo down >> order"}, "never": {Run: "echo never >> order"}}, Components: map[string]config.Component{"svc": {Run: "echo bad >> order", On: &config.On{Steps: []config.HookStep{{Startup: "up", Cleanup: "down"}, {Startup: "fail", Cleanup: "never"}}}}}}
	o := NewOrchestrator(cfg, dir)
	defer o.Close()
	if err := o.StartComponent("svc"); err == nil {
		t.Fatal("expected startup failure")
	}
	data, err := os.ReadFile(filepath.Join(dir, "order"))
	if err != nil || string(data) != "up\ndown\n" {
		t.Fatalf("rollback %q %v", data, err)
	}
}

func TestPairedStepsCancelAndCleanupOnce(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{
		"one": {Run: "touch first"}, "undo": {Run: "echo undo >> order"}, "slow": {Run: "trap 'exit 130' INT; touch entered; while :; do sleep 0.01; done"}, "never": {Run: "echo never >> order"},
	}, On: &config.On{Steps: []config.HookStep{{Startup: "one", Cleanup: "undo"}, {Startup: "slow", Cleanup: "never"}}}}
	o := NewOrchestrator(cfg, dir)
	started := make(chan error, 1)
	go func() { started <- o.Start() }()
	waitLifecycle(t, func() bool { _, err := os.Stat(filepath.Join(dir, "entered")); return err == nil })
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() { _ = o.Stop() })
	}
	wg.Wait()
	if err := <-started; err == nil {
		t.Fatal("cancelled startup unexpectedly succeeded")
	}
	_ = o.Close()
	data, err := os.ReadFile(filepath.Join(dir, "order"))
	if err != nil || string(data) != "undo\n" {
		t.Fatalf("cleanup %q %v", data, err)
	}
}
