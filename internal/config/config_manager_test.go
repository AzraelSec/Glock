package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryCatalog(t *testing.T) {
	raw := []byte(`tag_pattern: 'v1'
sources:
  z:
    type: github
    url: git@github.com:org/z.git
    head: develop
    path: custom
    updater: _ignore_
    exclude_tag: true
  a:
    type: github
    url: git@github.com:org/a.git
    head: main
    path: a
  asset:
    type: http
    path: assets
    url: https://example.com/archive
components:
  first: {source: z, run: echo first}
  second: {source: z, run: echo second}
`)
	cfg, err := ParseConfigV2(raw)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cm, err := RepositoryCatalog(&Project{Config: cfg, Root: root, Path: filepath.Join(root, "glock.yml")})
	if err != nil {
		t.Fatal(err)
	}
	if len(cm.Repos) != 2 || cm.Repos[0].Name != "a" || cm.Repos[1].Name != "z" {
		t.Fatalf("catalog: %+v", cm.Repos)
	}
	if cm.Repos[0].GitConfig.Path != filepath.Join(root, "a") || cm.Repos[1].GitConfig.Path != filepath.Join(root, "custom") {
		t.Fatal(cm.Repos)
	}
	if cm.Repos[1].GitConfig.Refs != "develop" || cm.Repos[1].Config.Updater != "_ignore_" || !cm.Repos[1].Config.ExcludeTag || cm.TagPattern != "v1" {
		t.Fatal(cm)
	}
}

func TestLoadFileRejectsV1(t *testing.T) {
	p := filepath.Join(t.TempDir(), "glock.yml")
	raw := []byte("repos: {}\n")
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(p)
	if err == nil || !strings.Contains(err.Error(), "config.repos: field not allowed") {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != string(raw) {
		t.Fatal("input changed")
	}
}

func TestNoApplicableGitSources(t *testing.T) {
	cfg, err := ParseConfigV2([]byte("components: {local: {run: echo hello}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = RepositoryCatalog(&Project{Config: cfg, Root: t.TempDir()}); err == nil {
		t.Fatal("expected no Git sources error")
	}
}

func TestRemoteSourcePathAllowsSymlinkOutsideProject(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	raw := []byte("sources: {repo: {type: github, path: linked/repo, url: 'git@github.com:org/repo.git', head: main}}\n")
	cfg, err := ParseConfigV2(raw)
	if err != nil {
		t.Fatal(err)
	}
	cm, err := RepositoryCatalog(&Project{Config: cfg, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := cm.Repos[0].GitConfig.Path; got != filepath.Join(root, "linked", "repo") {
		t.Fatal(got)
	}
	if err := os.Mkdir(filepath.Join(external, "repo"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(cm.Repos[0].GitConfig.Path)
	if err != nil || got != filepath.Join(external, "repo") {
		t.Fatalf("%s: %v", got, err)
	}
}
