package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/runtime"
	"github.com/AzraelSec/glock/internal/task"
)

func testConfig() *config.ConfigV2 {
	return &config.ConfigV2{
		Sources: map[string]config.Source{
			"backend": {
				Type: "github",
				URL:  new("git@github.com:AzraelSec/backend.git"),
				Head: new("main"),
				Path: new("backend"),
			},
		},
		Components: map[string]config.Component{
			"api": {
				Source:       new("backend"),
				Run:          "echo api",
				EnvFilenames: []string{".env"},
			},
			"worker": {
				Source: new("backend"),
				Run:    "echo worker",
			},
			"proxy": {
				Run: "echo proxy",
			},
		},
		Workspaces: map[string][]string{
			"backend": {"api", "worker"},
		},
		On: &config.On{
			Startup:  []string{"startup-task"},
			Shutdown: []string{"shutdown-task"},
		},
		Tasks: map[string]config.Task{
			"lint":          {Description: "lint", Run: "echo lint"},
			"startup-task":  {Description: "startup task", Run: "echo startup"},
			"shutdown-task": {Description: "shutdown task", Run: "echo shutdown"},
		},
	}
}

func TestNewOrchestrator_BuildsRegistries(t *testing.T) {
	cfg := testConfig()
	o := NewOrchestrator(cfg, "/tmp/glock")
	defer o.Close()

	if len(o.components) != 3 {
		t.Fatalf("wanted 3 components, got %d", len(o.components))
	}
	if len(o.workspaces) != 1 {
		t.Fatalf("wanted 1 workspace, got %d", len(o.workspaces))
	}
	if len(o.tasks) != 3 {
		t.Fatalf("wanted 3 tasks, got %d", len(o.tasks))
	}

	api := o.components["api"]
	if api.Source != "backend" {
		t.Errorf("api source: wanted backend, got %s", api.Source)
	}
	if api.Workspace != "backend" {
		t.Errorf("api workspace: wanted backend, got %s", api.Workspace)
	}

	proxy := o.components["proxy"]
	if proxy.Source != "" {
		t.Errorf("proxy source: wanted empty, got %s", proxy.Source)
	}
	if proxy.Workspace != "" {
		t.Errorf("proxy workspace: wanted empty, got %s", proxy.Workspace)
	}
}

func TestProviders(t *testing.T) {
	cfg := testConfig()
	o := NewOrchestrator(cfg, "/tmp/glock")
	defer o.Close()

	p := o.Providers()
	if p.Workspaces == nil || p.Components == nil || p.Events == nil || p.Tasks == nil {
		t.Fatal("all provider fields must be non-nil")
	}

	// Verify each provider is the orchestrator itself (same pointer).
	if p.Workspaces != o {
		t.Error("Workspaces provider should be the orchestrator")
	}
	if p.Components != o {
		t.Error("Components provider should be the orchestrator")
	}
	if p.Events != o {
		t.Error("Events provider should be the orchestrator")
	}
	if p.Tasks != o {
		t.Error("Tasks provider should be the orchestrator")
	}
}

func TestWorkspaces(t *testing.T) {
	cfg := testConfig()
	o := NewOrchestrator(cfg, "/tmp/glock")
	defer o.Close()

	wss := o.Workspaces()
	if len(wss) != 3 { // [All], [Unassigned], backend
		t.Fatalf("wanted 3 workspaces, got %d", len(wss))
	}

	found := make(map[string][]string)
	for _, ws := range wss {
		found[ws.Name] = ws.Components
	}

	if len(found[runtime.WorkspaceAll]) != 3 {
		t.Errorf("[All] workspace wanted 3 components, got %d", len(found[runtime.WorkspaceAll]))
	}
	if len(found[runtime.WorkspaceUnassigned]) != 1 {
		t.Errorf("[Unassigned] workspace wanted 1 component, got %d", len(found[runtime.WorkspaceUnassigned]))
	}
	if len(found["backend"]) != 2 {
		t.Errorf("backend workspace wanted 2 components, got %d", len(found["backend"]))
	}
}

func TestComponents_All(t *testing.T) {
	cfg := testConfig()
	o := NewOrchestrator(cfg, "/tmp/glock")
	defer o.Close()

	entries := o.Components(runtime.WorkspaceAll)
	if len(entries) != 3 {
		t.Fatalf("wanted 3 entries, got %d", len(entries))
	}

	// Unstarted components should have nil Process.
	for _, e := range entries {
		if e.Component.Name == "" {
			t.Error("component name should not be empty")
		}
		if e.Process != nil {
			t.Errorf("component %s should have nil process before start", e.Component.Name)
		}
	}
}

func TestComponents_Filtered(t *testing.T) {
	cfg := testConfig()
	o := NewOrchestrator(cfg, "/tmp/glock")
	defer o.Close()

	entries := o.Components("backend")
	if len(entries) != 2 {
		t.Fatalf("wanted 2 entries, got %d", len(entries))
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Component.Name
	}
	if names[0] != "api" || names[1] != "worker" {
		t.Errorf("unexpected components: %v", names)
	}
}

func TestComponents_Unassigned(t *testing.T) {
	cfg := testConfig()
	o := NewOrchestrator(cfg, "/tmp/glock")
	defer o.Close()

	entries := o.Components(runtime.WorkspaceUnassigned)
	if len(entries) != 1 {
		t.Fatalf("wanted 1 unassigned entry, got %d", len(entries))
	}
	if entries[0].Component.Name != "proxy" {
		t.Errorf("wanted proxy, got %s", entries[0].Component.Name)
	}
}

func TestRecentEvents(t *testing.T) {
	cfg := testConfig()
	o := NewOrchestrator(cfg, "/tmp/glock")
	defer o.Close()

	// Publish a few events.
	o.bus.Publish(event.Event{Name: event.ApplicationStarted})
	o.bus.Publish(event.Event{Name: event.ComponentStarted, Component: "api"})
	o.bus.Publish(event.Event{Name: event.TaskFinished, Component: "x"})

	// Give the collector a moment to receive them.
	time.Sleep(50 * time.Millisecond)

	all := o.RecentEvents(0)
	if len(all) != 3 {
		t.Fatalf("wanted 3 events, got %d", len(all))
	}

	// Most recent first.
	if all[0].Name != event.TaskFinished {
		t.Errorf("most recent event: wanted %s, got %s", event.TaskFinished, all[0].Name)
	}

	limited := o.RecentEvents(2)
	if len(limited) != 2 {
		t.Fatalf("wanted 2 events, got %d", len(limited))
	}
}

func TestStartComponent_ResolvesEnv(t *testing.T) {
	tmp := t.TempDir()

	// Write a .env file.
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("FOO=bar\nBAZ=qux\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"svc": {
				Run:          "echo svc",
				EnvFilenames: []string{".env"},
			},
		},
	}

	o := NewOrchestrator(cfg, tmp)
	defer o.Close()

	if err := o.StartComponent("svc"); err != nil {
		t.Fatalf("start component: %v", err)
	}

	// Verify the process was created.
	proc, ok := o.procMgr.Get("svc")
	if !ok {
		t.Fatal("process not found after start")
	}
	if proc.State != process.Running && proc.State != process.Stopped {
		t.Errorf("unexpected state: %s", proc.State)
	}
}

func TestStopComponent(t *testing.T) {
	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"svc": {Run: "sleep 10"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	if err := o.StartComponent("svc"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Give the process time to start.
	time.Sleep(100 * time.Millisecond)

	if err := o.StopComponent("svc"); err != nil {
		t.Fatalf("stop: %v", err)
	}

	proc, ok := o.procMgr.Get("svc")
	if !ok {
		t.Fatal("process not found after stop")
	}
	if proc.State != process.Stopped && proc.State != process.Failed {
		t.Errorf("wanted Stopped or Failed after stop, got %s", proc.State)
	}
}

func TestRestartComponent(t *testing.T) {
	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"svc": {Run: "echo hello"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	if err := o.StartComponent("svc"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wait for the quick command to finish.
	time.Sleep(200 * time.Millisecond)

	if err := o.RestartComponent("svc"); err != nil {
		t.Fatalf("restart: %v", err)
	}

	proc, ok := o.procMgr.Get("svc")
	if !ok {
		t.Fatal("process not found after restart")
	}
	// After restart of a quick command it may already be Stopped.
	if proc.State != process.Running && proc.State != process.Stopped {
		t.Errorf("unexpected state after restart: %s", proc.State)
	}
}

func TestRunTask(t *testing.T) {
	cfg := &config.ConfigV2{
		Tasks: map[string]config.Task{
			"hello-task": {Description: "hello-task", Run: "echo hello"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	if err := o.RunTask("hello-task"); err != nil {
		t.Fatalf("run task: %v", err)
	}

	// Wait for the quick command.
	time.Sleep(200 * time.Millisecond)

	tsk, ok := o.taskMgr.Get("hello-task")
	if !ok {
		t.Fatal("task not found")
	}
	if tsk.State != task.Finished {
		t.Errorf("wanted Finished, got %s", tsk.State)
	}
}

func TestRunTask_UsesExecDir(t *testing.T) {
	tmp := t.TempDir()

	cfg := &config.ConfigV2{
		Tasks: map[string]config.Task{
			"pwd": {Description: "print working dir", Run: "pwd"},
		},
	}

	o := NewOrchestrator(cfg, tmp)
	defer o.Close()

	if err := o.RunTask("pwd"); err != nil {
		t.Fatalf("run task: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	tsk, ok := o.taskMgr.Get("pwd")
	if !ok {
		t.Fatal("task not found")
	}
	if tsk.State != task.Finished {
		t.Fatalf("wanted Finished, got %s", tsk.State)
	}

	lines := tsk.Output.Lines()
	if len(lines) == 0 {
		t.Fatal("expected output lines")
	}
	if !strings.Contains(lines[0], tmp) {
		t.Errorf("expected output to contain %q, got %q", tmp, lines[0])
	}
}

func TestStart_StartupTasks(t *testing.T) {
	cfg := &config.ConfigV2{
		Tasks: map[string]config.Task{
			"startup": {Description: "startup", Run: "echo startup"},
		},
		On: &config.On{
			Startup: []string{"startup"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	if err := o.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	tsk, ok := o.taskMgr.Get("startup")
	if !ok {
		t.Fatal("startup task not found")
	}
	if tsk.State != task.Finished {
		t.Errorf("wanted Finished, got %s", tsk.State)
	}

	// Verify application.started was captured.
	events := o.RecentEvents(10)
	found := false
	for _, e := range events {
		if e.Name == event.ApplicationStarted {
			found = true
			break
		}
	}
	if !found {
		t.Error("application.started event not found")
	}
}

func TestStop_StopsProcesses(t *testing.T) {
	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"svc": {Run: "sleep 10"},
		},
		Tasks: map[string]config.Task{
			"shutdown": {Description: "shutdown", Run: "echo shutdown"},
		},
		On: &config.On{
			Shutdown: []string{"shutdown"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	if err := o.StartComponent("svc"); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if err := o.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Give the event collector time to receive the events.
	time.Sleep(50 * time.Millisecond)

	proc, ok := o.procMgr.Get("svc")
	if !ok {
		t.Fatal("process not found")
	}
	if proc.State != process.Stopped && proc.State != process.Failed {
		t.Errorf("wanted Stopped or Failed, got %s", proc.State)
	}

	// Verify application.stopping and application.stopped events.
	events := o.RecentEvents(10)
	var stopping, stopped bool
	for _, e := range events {
		if e.Name == event.ApplicationStopping {
			stopping = true
		}
		if e.Name == event.ApplicationStopped {
			stopped = true
		}
	}
	if !stopping {
		t.Error("application.stopping event not found")
	}
	if !stopped {
		t.Error("application.stopped event not found")
	}
}

func TestLoadEnv(t *testing.T) {
	tmp := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("KEY=value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".env.local"), []byte("LOCAL=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := loadEnv(tmp, []string{".env", ".env.local"})
	if len(env) != 2 {
		t.Fatalf("wanted 2 env vars, got %d", len(env))
	}

	m := make(map[string]string, len(env))
	for _, e := range env {
		m[e] = ""
	}
	if _, ok := m["KEY=value"]; !ok {
		t.Error("KEY=value not found")
	}
	if _, ok := m["LOCAL=true"]; !ok {
		t.Error("LOCAL=true not found")
	}

	// Empty filenames returns nil.
	empty := loadEnv(tmp, nil)
	if empty != nil {
		t.Errorf("wanted nil, got %v", empty)
	}
}

func TestLoadEnv_LaterFileWins(t *testing.T) {
	tmp := t.TempDir()

	// Base file defines VAR=from_base and BASE=only_here
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("VAR=from_base\nBASE=only_here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Override file defines VAR=from_override (same key, different value)
	if err := os.WriteFile(filepath.Join(tmp, ".env.local"), []byte("VAR=from_override\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := loadEnv(tmp, []string{".env", ".env.local"})

	// Must have exactly 2 unique keys (no duplicates)
	if len(env) != 2 {
		t.Fatalf("wanted 2 env vars, got %d: %v", len(env), env)
	}

	m := make(map[string]string, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}

	// Later file wins for the overlapping key
	if m["VAR"] != "from_override" {
		t.Errorf("VAR: wanted 'from_override', got %q", m["VAR"])
	}
	// Key from base file only is preserved
	if m["BASE"] != "only_here" {
		t.Errorf("BASE: wanted 'only_here', got %q", m["BASE"])
	}
}

func TestTasks(t *testing.T) {
	cfg := &config.ConfigV2{
		Tasks: map[string]config.Task{
			"lint":    {Description: "lint", Run: "echo lint"},
			"unnamed": {Run: "echo unnamed"}, // no description
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	tasks := o.Tasks()
	if len(tasks) != 2 {
		t.Fatalf("wanted 2 tasks, got %d", len(tasks))
	}

	names := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		names[t.Name] = true
	}
	if !names["lint"] {
		t.Error("missing task 'lint'")
	}
	if !names["unnamed"] {
		t.Error("missing task 'unnamed'")
	}
}

func TestStartComponent_RunsStartupTasks(t *testing.T) {
	tmp := t.TempDir()

	cfg := &config.ConfigV2{
		Tasks: map[string]config.Task{
			"touch": {Description: "touch", Run: "touch marker"},
		},
		Components: map[string]config.Component{
			"svc": {
				Run: "sleep 10",
				On: &config.On{
					Startup: []string{"touch"},
				},
			},
		},
	}

	o := NewOrchestrator(cfg, tmp)
	defer o.Close()

	if err := o.StartComponent("svc"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// The startup task should have created a file in the project dir.
	marker := filepath.Join(tmp, "marker")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Errorf("startup task did not create marker file in %s", tmp)
	}

	// The main process should also be running.
	proc, ok := o.procMgr.Get("svc")
	if !ok {
		t.Fatal("process not found")
	}
	if proc.State != process.Running && proc.State != process.Stopping {
		t.Errorf("wanted Running or Stopping, got %s", proc.State)
	}
}

func TestStartComponent_StartupTaskFails(t *testing.T) {
	cfg := &config.ConfigV2{
		Tasks: map[string]config.Task{
			"fail": {Description: "fail", Run: "exit 42"},
		},
		Components: map[string]config.Component{
			"svc": {
				Run: "sleep 10",
				On: &config.On{
					Startup: []string{"fail"},
				},
			},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	if err := o.StartComponent("svc"); err == nil {
		t.Fatal("expected StartComponent to fail because startup task failed")
	}

	// The main process should NOT have been started.
	_, ok := o.procMgr.Get("svc")
	if ok {
		t.Error("main process should not have been started")
	}
}

func TestStopComponent_RunsShutdownTasks(t *testing.T) {
	tmp := t.TempDir()

	cfg := &config.ConfigV2{
		Tasks: map[string]config.Task{
			"cleanup": {Description: "cleanup", Run: "touch cleanup-marker"},
		},
		Components: map[string]config.Component{
			"svc": {
				Run: "sleep 10",
				On: &config.On{
					Shutdown: []string{"cleanup"},
				},
			},
		},
	}

	o := NewOrchestrator(cfg, tmp)
	defer o.Close()

	if err := o.StartComponent("svc"); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if err := o.StopComponent("svc"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// The shutdown task should have created a file in the project dir.
	marker := filepath.Join(tmp, "cleanup-marker")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		t.Errorf("shutdown task did not create marker file in %s", tmp)
	}
}

func TestStopComponent_NotRunning(t *testing.T) {
	cfg := &config.ConfigV2{
		Tasks: map[string]config.Task{
			"cleanup": {Description: "cleanup", Run: "touch cleanup-marker"},
		},
		Components: map[string]config.Component{
			"svc": {
				Run: "sleep 10",
				On: &config.On{
					Shutdown: []string{"cleanup"},
				},
			},
		},
	}

	tmp := t.TempDir()
	o := NewOrchestrator(cfg, tmp)
	defer o.Close()

	// The component was never started — Stop should return an error.
	if err := o.StopComponent("svc"); err == nil {
		t.Fatal("expected StopComponent to fail because component is not running")
	}

	// Shutdown tasks should NOT have run.
	marker := filepath.Join(tmp, "cleanup-marker")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("shutdown task should not have run for a non-running component")
	}
}

func TestStartWorkspace_StartsAll(t *testing.T) {
	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"svc1": {Run: "sleep 10"},
			"svc2": {Run: "sleep 10"},
		},
		Workspaces: map[string][]string{
			"backend": {"svc1", "svc2"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	if err := o.StartWorkspace("backend"); err != nil {
		t.Fatalf("start workspace: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	for _, name := range []string{"svc1", "svc2"} {
		proc, ok := o.procMgr.Get(name)
		if !ok {
			t.Fatalf("process %s not found", name)
		}
		if proc.State != process.Running {
			t.Errorf("%s: wanted Running, got %s", name, proc.State)
		}
	}

	// Give the event collector time to receive the events.
	time.Sleep(50 * time.Millisecond)

	// Verify workspace events were emitted.
	events := o.RecentEvents(10)
	var starting, started bool
	for _, e := range events {
		if e.Name == event.WorkspaceStarting {
			starting = true
		}
		if e.Name == event.WorkspaceStarted {
			started = true
		}
	}
	if !starting {
		t.Error("workspace.starting event not found")
	}
	if !started {
		t.Error("workspace.started event not found")
	}
}

func TestStartComponent_StartupTaskInheritsEnv(t *testing.T) {
	tmp := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("FROM_ENV=loaded\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ConfigV2{
		Tasks: map[string]config.Task{
			"show": {Description: "show env", Run: "echo $FROM_ENV > env-marker"},
		},
		Components: map[string]config.Component{
			"svc": {
				Run:          "sleep 10",
				EnvFilenames: []string{".env"},
				On: &config.On{
					Startup: []string{"show"},
				},
			},
		},
	}

	o := NewOrchestrator(cfg, tmp)
	defer o.Close()

	if err := o.StartComponent("svc"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wait for the quick startup task to finish.
	time.Sleep(200 * time.Millisecond)

	marker := filepath.Join(tmp, "env-marker")
	b, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("startup task did not write marker: %v", err)
	}
	if got := strings.TrimSpace(string(b)); got != "loaded" {
		t.Errorf("startup task env: wanted 'loaded', got %q", got)
	}
}

func TestStartWorkspace_SkipsRunning(t *testing.T) {
	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"svc1": {Run: "sleep 10"},
			"svc2": {Run: "sleep 10"},
		},
		Workspaces: map[string][]string{
			"backend": {"svc1", "svc2"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	// Start only svc1 first.
	if err := o.StartComponent("svc1"); err != nil {
		t.Fatalf("start svc1: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Now start the whole workspace — svc1 should be skipped, svc2 started.
	if err := o.StartWorkspace("backend"); err != nil {
		t.Fatalf("start workspace: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	for _, name := range []string{"svc1", "svc2"} {
		proc, ok := o.procMgr.Get(name)
		if !ok {
			t.Fatalf("process %s not found", name)
		}
		if proc.State != process.Running {
			t.Errorf("%s: wanted Running, got %s", name, proc.State)
		}
	}
}

func TestStartWorkspace_RestartsFailed(t *testing.T) {
	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"svc": {Run: "sleep 10"},
		},
		Workspaces: map[string][]string{
			"backend": {"svc"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	if err := o.StartComponent("svc"); err != nil {
		t.Fatalf("start svc: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Kill the process; an intentional kill transitions to Stopped.
	if err := o.procMgr.Kill("svc"); err != nil {
		t.Fatalf("kill svc: %v", err)
	}

	proc, _ := o.procMgr.Get("svc")
	if proc.State != process.Stopped {
		t.Fatalf("wanted Stopped before restart, got %s", proc.State)
	}

	// StartWorkspace should restart the stopped component.
	if err := o.StartWorkspace("backend"); err != nil {
		t.Fatalf("start workspace: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	proc, _ = o.procMgr.Get("svc")
	if proc.State != process.Running {
		t.Errorf("wanted Running after restart, got %s", proc.State)
	}
}

func TestStopWorkspace_StopsAll(t *testing.T) {
	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"svc1": {Run: "sleep 10"},
			"svc2": {Run: "sleep 10"},
		},
		Workspaces: map[string][]string{
			"backend": {"svc1", "svc2"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	for _, name := range []string{"svc1", "svc2"} {
		if err := o.StartComponent(name); err != nil {
			t.Fatalf("start %s: %v", name, err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	if err := o.StopWorkspace("backend"); err != nil {
		t.Fatalf("stop workspace: %v", err)
	}

	for _, name := range []string{"svc1", "svc2"} {
		proc, ok := o.procMgr.Get(name)
		if !ok {
			t.Fatalf("process %s not found", name)
		}
		if proc.State != process.Stopped && proc.State != process.Failed {
			t.Errorf("%s: wanted Stopped or Failed, got %s", name, proc.State)
		}
	}

	// Give the event collector time to receive the events.
	time.Sleep(50 * time.Millisecond)

	// Verify workspace events were emitted.
	events := o.RecentEvents(10)
	var stopping, stopped bool
	for _, e := range events {
		if e.Name == event.WorkspaceStopping {
			stopping = true
		}
		if e.Name == event.WorkspaceStopped {
			stopped = true
		}
	}
	if !stopping {
		t.Error("workspace.stopping event not found")
	}
	if !stopped {
		t.Error("workspace.stopped event not found")
	}
}

func TestStopWorkspace_SkipsNotRunning(t *testing.T) {
	cfg := &config.ConfigV2{
		Components: map[string]config.Component{
			"svc1": {Run: "sleep 10"},
			"svc2": {Run: "sleep 10"},
		},
		Workspaces: map[string][]string{
			"backend": {"svc1", "svc2"},
		},
	}

	o := NewOrchestrator(cfg, t.TempDir())
	defer o.Close()

	// Start only svc1.
	if err := o.StartComponent("svc1"); err != nil {
		t.Fatalf("start svc1: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if err := o.StopWorkspace("backend"); err != nil {
		t.Fatalf("stop workspace: %v", err)
	}

	// svc1 should be stopped.
	proc1, _ := o.procMgr.Get("svc1")
	if proc1.State != process.Stopped && proc1.State != process.Failed {
		t.Errorf("svc1: wanted Stopped or Failed, got %s", proc1.State)
	}

	// svc2 was never started — should remain Sleeping (not tracked).
	_, ok := o.procMgr.Get("svc2")
	if ok {
		t.Error("svc2 should not have a process entry because it was never started")
	}
}
