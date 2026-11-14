package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AzraelSec/glock/internal/config"
)

func TestStartSelection(t *testing.T) {
	source := "repo"
	cfg := &config.ConfigV2{Sources: map[string]config.Source{"repo": {Type: config.SourceTypeGitHub}}, Components: map[string]config.Component{"a": {Source: &source}, "b": {Source: &source}, "service": {}}}
	names, err := selectedComponents(cfg, []string{"repo"})
	if err != nil || strings.Join(names, ",") != "a,b" {
		t.Fatalf("%v %v", names, err)
	}
	names, err = selectedComponents(cfg, nil)
	if err != nil || len(names) != 3 {
		t.Fatalf("%v %v", names, err)
	}
	if _, err = selectedComponents(cfg, []string{"missing"}); err == nil {
		t.Fatal("unknown source accepted")
	}
}

func TestStartStreamsHooksAndAllLines(t *testing.T) {
	cfg, err := config.ParseConfigV2([]byte(`version: 2
components:
  api:
    run: 'i=0; while [ "$i" -lt 12000 ]; do echo "line-$i"; i=$((i+1)); done; echo error >&2'
    on: {startup: [setup], shutdown: [cleanup]}
tasks:
  setup: {run: echo setup}
  cleanup: {run: echo cleanup}
  app: {run: echo application}
on: {startup: [app]}
`))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	if err = runStart(ctx, &config.Project{Config: cfg, Root: t.TempDir()}, []string{"api"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 12003 {
		t.Fatalf("lost output: %d lines", len(lines))
	}
	for i := 0; i < 12000; i++ {
		if lines[i+2] != fmt.Sprintf("[api] line-%d", i) {
			t.Fatalf("line %d: %q", i, lines[i+2])
		}
	}
	if !strings.Contains(lines[0], "application") || !strings.Contains(lines[1], "setup") || !strings.Contains(lines[len(lines)-1], "cleanup") {
		t.Fatal("hook order lost")
	}
	if stderr.String() != "[api] error\n" {
		t.Fatalf("stderr: %q", stderr.String())
	}
}

func TestStartFailureAndCleanup(t *testing.T) {
	for _, run := range []string{"exit 4", "echo done"} {
		t.Run(run, func(t *testing.T) {
			root := t.TempDir()
			cfg := &config.ConfigV2{Components: map[string]config.Component{"api": {Run: run, On: &config.On{Shutdown: []string{"cleanup"}}}}, Tasks: map[string]config.Task{"cleanup": {Run: "touch cleaned"}}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var stdout, stderr bytes.Buffer
			err := runStart(ctx, &config.Project{Config: cfg, Root: root}, []string{"api"}, &stdout, &stderr)
			if (err != nil) != (run == "exit 4") {
				t.Fatalf("run %s error: %v", run, err)
			}
			if _, err = os.Stat(filepath.Join(root, "cleaned")); err != nil {
				t.Fatal("cleanup missing", err)
			}
		})
	}
}

func TestStartCancellationDuringInitialization(t *testing.T) {
	root := t.TempDir()
	cfg := &config.ConfigV2{Tasks: map[string]config.Task{"setup": {Run: "trap 'exit 130' INT; touch entered; while :; do sleep 0.01; done"}, "cleanup": {Run: "touch cleaned"}}, On: &config.On{Startup: []string{"setup"}, Shutdown: []string{"cleanup"}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	var stdout, stderr bytes.Buffer
	go func() { done <- runStart(ctx, &config.Project{Config: cfg, Root: root}, nil, &stdout, &stderr) }()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "entered")); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("setup did not start")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown blocked")
	}
	if _, err := os.Stat(filepath.Join(root, "cleaned")); err != nil {
		t.Fatal("cleanup missing", err)
	}
}

func TestStartPropagatesShutdownError(t *testing.T) {
	cfg := &config.ConfigV2{Components: map[string]config.Component{"api": {Run: "true", On: &config.On{Shutdown: []string{"fail"}}}}, Tasks: map[string]config.Task{"fail": {Run: "exit 7"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	if err := runStart(ctx, &config.Project{Config: cfg, Root: t.TempDir()}, []string{"api"}, &stdout, &stderr); err == nil {
		t.Fatal("cleanup failure discarded")
	}
}

func TestStartFilesystemSourceRunsInExistingDirectory(t *testing.T) {
	projectRoot := t.TempDir()
	external := t.TempDir()
	sourceName := "local"
	cfg := &config.ConfigV2{Sources: map[string]config.Source{sourceName: {Type: config.SourceTypeFilesystem, Path: &external}}, Components: map[string]config.Component{"tool": {Source: &sourceName, Run: "touch component-ran", On: &config.On{Startup: []string{"setup"}, Shutdown: []string{"cleanup"}}}}, Tasks: map[string]config.Task{"setup": {Run: "touch setup-ran"}, "cleanup": {Run: "touch cleanup-ran"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out, errOut bytes.Buffer
	if err := runStart(ctx, &config.Project{Config: cfg, Root: projectRoot}, []string{"tool"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"component-ran", "setup-ran", "cleanup-ran"} {
		if _, err := os.Stat(filepath.Join(external, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(projectRoot, name)); !os.IsNotExist(err) {
			t.Fatalf("%s unexpectedly in project root", name)
		}
	}
}
