package task

import (
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/AzraelSec/glock/internal/event"
	"github.com/AzraelSec/glock/internal/process"
)

type mockPublisher struct {
	mu     sync.Mutex
	events []event.Event
}

func (m *mockPublisher) Publish(e event.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
}

func TestRunSucceeds(t *testing.T) {
	pub := &mockPublisher{}
	tm := NewTaskManager(pub)

	task := &Task{
		Name:    "success",
		Command: "echo hello && exit 0",
		State:   Pending,
		Output:  process.NewMemoryOutput(),
	}

	if _, err := tm.Run(task); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got, _ := tm.Get("success"); got.State != Running {
		t.Fatalf("expected state Running, got %s", got.State)
	}

	// Wait for the monitor to finish.
	time.Sleep(200 * time.Millisecond)

	if got, _ := tm.Get("success"); got.State != Finished {
		t.Fatalf("expected state Finished, got %s", got.State)
	}
	if task.Output.Len() == 0 {
		t.Fatal("expected output lines")
	}
	if !hasEvent(pub.snapshot(), event.TaskFinished) {
		t.Fatal("expected task.finished event")
	}
}

func TestRunFails(t *testing.T) {
	pub := &mockPublisher{}
	tm := NewTaskManager(pub)

	task := &Task{
		Name:    "fail",
		Command: "exit 42",
		State:   Pending,
		Output:  process.NewMemoryOutput(),
	}

	if _, err := tm.Run(task); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if got, _ := tm.Get("fail"); got.State != Failed {
		t.Fatalf("expected state Failed, got %s", got.State)
	}
	if !hasEvent(pub.snapshot(), event.TaskFailed) {
		t.Fatal("expected task.failed event")
	}
}

func TestRunDuplicate(t *testing.T) {
	pub := &mockPublisher{}
	tm := NewTaskManager(pub)

	task := &Task{
		Name:    "dup",
		Command: "sleep 5",
		State:   Pending,
		Output:  process.NewMemoryOutput(),
	}

	if _, err := tm.Run(task); err != nil {
		t.Fatalf("first Run failed: %v", err)
	}

	task2 := &Task{
		Name:    "dup",
		Command: "echo other",
		State:   Pending,
		Output:  process.NewMemoryOutput(),
	}

	if _, err := tm.Run(task2); err == nil {
		t.Fatal("second Run should have failed")
	}

	// Clean up the background sleep process.
	tm.mu.RLock()
	runner := tm.runners["dup"]
	tm.mu.RUnlock()
	if runner != nil {
		_ = runner.SendSignal(syscall.SIGKILL)
	}
}

func TestReRunFinishedTask(t *testing.T) {
	pub := &mockPublisher{}
	tm := NewTaskManager(pub)

	task := &Task{
		Name:    "rerun",
		Command: "echo first && exit 0",
		State:   Pending,
		Output:  process.NewMemoryOutput(),
	}

	if _, err := tm.Run(task); err != nil {
		t.Fatalf("first Run failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if got, _ := tm.Get("rerun"); got.State != Finished {
		t.Fatalf("expected state Finished after first run, got %s", got.State)
	}

	firstLen := task.Output.Len()

	task.Command = "echo second && echo third && exit 0"
	if _, err := tm.Run(task); err != nil {
		t.Fatalf("second Run failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if got, _ := tm.Get("rerun"); got.State != Finished {
		t.Fatalf("expected state Finished after second run, got %s", got.State)
	}
	if task.Output.Len() <= firstLen {
		t.Fatalf("expected output to be replaced, got %d lines", task.Output.Len())
	}
}

func TestGetAndAll(t *testing.T) {
	tm := NewTaskManager(nil)

	task := &Task{
		Name:    "getme",
		Command: "true",
		State:   Pending,
		Output:  process.NewMemoryOutput(),
	}

	if _, err := tm.Run(task); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	got, ok := tm.Get("getme")
	if !ok || got.Name != "getme" {
		t.Fatal("expected to find task getme")
	}

	all := tm.All()
	if len(all) != 1 || all[0].Name != "getme" {
		t.Fatalf("expected All to return one task, got %d", len(all))
	}
}

func TestRunWithNoOutput(t *testing.T) {
	pub := &mockPublisher{}
	tm := NewTaskManager(pub)

	task := &Task{
		Name:    "nooutput",
		Command: "true",
		State:   Pending,
		// Output is nil — Run should create one.
	}

	if _, err := tm.Run(task); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	got, _ := tm.Get("nooutput")
	if got.State != Finished {
		t.Fatalf("expected state Finished, got %s", got.State)
	}
	if got.Output == nil {
		t.Fatal("expected Output to be auto-created")
	}
}

func TestRunCompletionChannel(t *testing.T) {
	tm := NewTaskManager(nil)

	completion, err := tm.Run(&Task{
		Name:    "waited",
		Command: "exit 3",
		State:   Pending,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	select {
	case res := <-completion:
		if res.State != Failed {
			t.Errorf("expected Failed, got %s", res.State)
		}
		if res.ExitCode != 3 {
			t.Errorf("expected exit code 3, got %d", res.ExitCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for completion result")
	}

	// The channel must be closed after delivering the result.
	if _, ok := <-completion; ok {
		t.Fatal("expected completion channel to be closed")
	}
}

func hasEvent(events []event.Event, name string) bool {
	for _, e := range events {
		if e.Name == name {
			return true
		}
	}
	return false
}

func (m *mockPublisher) snapshot() []event.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.events)
}
