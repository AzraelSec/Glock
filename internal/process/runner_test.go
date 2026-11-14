package process

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestOutputDrainsBeforeCompletion(t *testing.T) {
	output := NewMemoryOutput()
	runner, err := NewOSProcess(`i=0; while [ "$i" -lt 2000 ]; do echo "out:$i"; echo "err:$i" >&2; i=$((i + 1)); done`, "", nil, output)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.WaitChan():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not finish")
	}
	lines := output.Lines()
	if runner.ExitCode() != 0 || len(lines) != 4000 {
		t.Fatalf("output truncated: exit=%d lines=%d", runner.ExitCode(), len(lines))
	}
	lastOut, lastErr := false, false
	for _, line := range lines {
		if strings.HasPrefix(line, "[glock]") {
			t.Fatalf("pipe read failed: %s", line)
		}
		if line == "out:1999" {
			lastOut = true
		}
		if line == "err:1999" {
			lastErr = true
		}
	}
	if !lastOut || !lastErr {
		t.Fatal("trailing stdout/stderr missing")
	}
}

func TestMergeEnvironmentUniqueAndHostWins(t *testing.T) {
	t.Setenv("GLOCK_MERGE_HOST", "")
	merged := MergeEnvironment([]string{"GLOCK_MERGE_HOST=file", "GLOCK_MERGE_FILE=first", "GLOCK_MERGE_FILE=last"})
	seen := make(map[string]bool)
	for _, entry := range merged {
		key, value, _ := strings.Cut(entry, "=")
		if seen[key] {
			t.Fatalf("duplicate key %s", key)
		}
		seen[key] = true
		if key == "GLOCK_MERGE_HOST" && value != "" {
			t.Fatal("empty host value lost")
		}
		if key == "GLOCK_MERGE_FILE" && value != "last" {
			t.Fatal("later file value lost")
		}
	}
	if !seen["GLOCK_MERGE_HOST"] || !seen["GLOCK_MERGE_FILE"] {
		t.Fatal("environment keys lost")
	}
	if os.Getenv("GLOCK_MERGE_HOST") != "" {
		t.Fatal("host environment mutated")
	}
}
