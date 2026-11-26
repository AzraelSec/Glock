package config_cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
)

func TestValidateCommand(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"valid", "components: {local: {run: 'touch SHOULD_NOT_EXIST'}}\n", ""},
		{"explicit empty mapping", "{}\n", ""},
		{"blank", "", "no YAML document found"},
		{"whitespace only", " \n  \n", "no YAML document found"},
		{"comment only", "# Project configuration\n  # Add components here\n", "no YAML document found"},
		{"v1", "repos: {}\n", "config.repos: field not allowed"},
		{"schema", "components: {local: {run: 4}}\n", "glock.custom.yml:1:27"},
		{"references", "components: {local: {run: echo, source: missing}}\n", "missing"},
		{"version field", "version: 2\ncomponents: {}\n", "config.version: field not allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "glock.custom.yml")
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

func TestValidatePrintsNativeDiagnosticsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.yml")
	if err := os.WriteFile(path, []byte("tasks: {build: {run: 4}}\ncomponents: {api: {run: false}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.ConfigPathEnv, path)
	cmd := NewCommand(dependency.NewManager())
	cmd.SetArgs([]string{"validate"})
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	cmd.SilenceUsage = true
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected validation failure")
	}
	for _, field := range []string{"config.tasks.build.run", "config.components.api.run"} {
		if strings.Count(stderr.String(), "\n"+field+": conflicting values") != 1 {
			t.Fatalf("unexpected stderr: %s", stderr.String())
		}
	}
}
