package config_cmd

import (
	"bytes"
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCommand(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"valid", "version: 2\ncomponents: {local: {run: 'touch SHOULD_NOT_EXIST'}}\n", ""},
		{"v1", "repos: {}\n", "config migrate"},
		{"schema", "version: 2\ncomponents: {local: {run: 4}}\n", "glock.yml:2"},
		{"references", "version: 2\ncomponents: {local: {run: echo, source: missing}}\n", "missing"},
		{"conflict", "version: 1\ncomponents: {}\n", "conflicts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "glock.yml")
			if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(config.ConfigPathEnv, path)
			cmd := NewCommand(dependency.NewManager())
			cmd.SetArgs([]string{"validate"})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			err := cmd.Execute()
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 1 {
				t.Fatal("validation caused side effects")
			}
		})
	}
}

func TestMigrateCommand(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "old.yml")
	raw := []byte("repos: {}\n")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.ConfigPathEnv, path)
	cmd := NewCommand(dependency.NewManager())
	cmd.SetArgs([]string{"migrate", "--output", filepath.Join(root, "glock.yml")})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFile(filepath.Join(root, "glock.yml")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(raw) {
		t.Fatal("source modified")
	}
}

func TestMigrateCommandRequiresExplicitV1Root(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "old.yml")
	root := filepath.Join(dir, "actual-root")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("root_path: relative\nrepos: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.ConfigPathEnv, backup)
	output := filepath.Join(root, "glock.yml")
	run := func(args ...string) error {
		cmd := NewCommand(dependency.NewManager())
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run("migrate", "--output", output); err == nil || !strings.Contains(err.Error(), "--v1-root") {
		t.Fatalf("missing root accepted: %v", err)
	}
	if err := run("migrate", "--output", output, "--v1-root", root); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFile(output); err != nil {
		t.Fatal(err)
	}
}
