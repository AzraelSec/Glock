package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateV1RoundTrip(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "old.yml")
	raw := []byte(`root_path: project
root_main_stream: develop
tag_pattern: 'v{.Now}'
env_filenames: [.env]
repos:
  api:
    remote: git@github.com:org/api.git
    rel_path: backend
    updater: go
    exclude_tag: true
    on_start: ['echo start']
    on_stop: echo stop
`)
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "glock.yml")
	if err := WriteMigration(input, output, root); err != nil {
		t.Fatal(err)
	}
	project, err := LoadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	s := project.Config.Sources["api"]
	c := project.Config.Components["api"]
	if *s.Path != "backend" || *s.Head != "develop" || s.Updater != "go" || !s.ExcludeTag || c.Run != "echo start" || len(c.EnvFilenames) != 1 || c.On == nil {
		t.Fatalf("bad migration: %+v %+v", s, c)
	}
	if project.Config.Tasks[c.On.Shutdown[0]].Run != "echo stop" {
		t.Fatal("lost cleanup")
	}
	if err = WriteMigration(input, output, root); err == nil {
		t.Fatal("overwrote destination")
	}
	got, _ := os.ReadFile(input)
	if string(got) != string(raw) {
		t.Fatal("changed source")
	}
}

func TestMigrationBlockers(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"multiple", "on_start: [echo a, echo b]", "multiple commands"},
		{"shutdown", "on_stop: echo stop", "shutdown-only"},
		{"traversal", "rel_path: ../outside", "within the project"},
		{"absolute", "rel_path: /outside", "within the project"},
		{"root", "rel_path: .", "non-root"},
		{"unknown", "typo: true", "field typo not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte("repos:\n  api:\n    remote: git@github.com:org/api.git\n    " + tc.body + "\n")
			data, _, err := MigrateV1(raw, filepath.Join(t.TempDir(), "old.yml"), "")
			if err == nil || !strings.Contains(err.Error(), tc.want) || data != nil {
				t.Fatalf("%s: %v", data, err)
			}
		})
	}
}

func TestMigrationServicesAndCollisions(t *testing.T) {
	raw := []byte("repos:\n  service-1: {remote: 'git@github.com:org/api.git'}\nservices:\n  - {cmd: echo service, dispose: echo cleanup}\n")
	data, _, err := MigrateV1(raw, filepath.Join(t.TempDir(), "old.yml"), "")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfigV2(data)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := cfg.Components["service-1-2"]
	if !ok || c.Source != nil || c.On == nil {
		t.Fatalf("%s", data)
	}
}

func TestMigrationDestination(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "glock.yml")
	raw := []byte("repos: {}\n")
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{input, filepath.Join(dir, "wrong.yml"), filepath.Join(dir, "elsewhere", "glock.yml")} {
		if err := WriteMigration(input, out, ""); err == nil {
			t.Fatalf("accepted %s", out)
		}
	}
	got, _ := os.ReadFile(input)
	if string(got) != string(raw) {
		t.Fatal("changed original")
	}
}

func TestMigrateRelativeRootRequiresExplicitAbsoluteRoot(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "backup.yml")
	raw := []byte("root_path: workspace\nrepos:\n  api: {remote: 'git@github.com:org/api.git'}\n")
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "actual-root")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "glock.yml")
	for _, explicit := range []string{"", "relative"} {
		err := WriteMigration(input, output, explicit)
		if err == nil || !strings.Contains(err.Error(), "--v1-root") {
			t.Fatalf("root %q: %v", explicit, err)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("wrote ambiguous migration")
		}
	}
	if err := WriteMigration(input, output, root); err != nil {
		t.Fatal(err)
	}
	project, err := LoadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := RepositoryCatalog(project)
	if err != nil {
		t.Fatal(err)
	}
	if cm.Repos[0].GitConfig.Path != filepath.Join(root, "api") {
		t.Fatal(cm.Repos[0].GitConfig.Path)
	}
	got, _ := os.ReadFile(input)
	if string(got) != string(raw) {
		t.Fatal("source changed")
	}
}
