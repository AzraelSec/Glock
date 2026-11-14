package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
)

func TestParseConfigV2_ValidExample(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "glock.example.yml"))
	if err != nil {
		t.Fatalf("read example: %v", err)
	}

	cfg, err := ParseConfigV2(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(cfg.Sources) != 4 {
		t.Errorf("wanted 4 sources, got %d", len(cfg.Sources))
	}
	if len(cfg.Components) != 4 {
		t.Errorf("wanted 4 components, got %d", len(cfg.Components))
	}
	if len(cfg.Workspaces) != 2 {
		t.Errorf("wanted 2 workspaces, got %d", len(cfg.Workspaces))
	}
	if cfg.On == nil || len(cfg.On.Startup) != 1 || len(cfg.On.Shutdown) != 1 {
		t.Errorf("wanted 1 startup and 1 shutdown task reference")
	}
	if len(cfg.Tasks) != 5 {
		t.Errorf("wanted 5 top-level tasks, got %d", len(cfg.Tasks))
	}

	backend, ok := cfg.Sources["backend"]
	if !ok {
		t.Fatalf("missing backend source")
	}
	if backend.Type != "github" {
		t.Errorf("wanted type github, got %s", backend.Type)
	}
	if backend.URL == nil || !strings.Contains(*backend.URL, "AzraelSec/backend") {
		t.Errorf("unexpected backend url: %v", backend.URL)
	}

	api, ok := cfg.Components["api"]
	if !ok {
		t.Fatalf("missing api component")
	}
	if api.Source == nil || *api.Source != "backend" {
		t.Errorf("unexpected api source: %v", api.Source)
	}
	if !strings.Contains(api.Run, "go run") {
		t.Errorf("unexpected api run: %v", api.Run)
	}
}

func TestParseConfigV2_SourcelessComponent(t *testing.T) {
	const src = `
sources:
  backend:
    type: github
    path: backend
    url: git@github.com:AzraelSec/backend.git
    head: main

components:
  api:
    source: backend
    run: go run main.go

  local-proxy:
    run: caddy reverse-proxy --from :8080 --to :3000
`

	cfg, err := ParseConfigV2([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	proxy, ok := cfg.Components["local-proxy"]
	if !ok {
		t.Fatalf("missing local-proxy component")
	}
	if proxy.Source != nil {
		t.Errorf("wanted no source, got %v", *proxy.Source)
	}
	if proxy.Run != "caddy reverse-proxy --from :8080 --to :3000" {
		t.Errorf("unexpected run: %v", proxy.Run)
	}
}

func TestParseConfigV2_MissingRequiredField(t *testing.T) {
	const src = `
components:
  api:
    source: backend
    # run is missing
`

	_, err := ParseConfigV2([]byte(src))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestParseConfigV2_UnknownFieldsRejected(t *testing.T) {
	cases := map[string]string{
		"top level": `
unknown_top_level: true
components:
  api:
    run: go run main.go
`,
		"source field": `
sources:
  backend:
    type: github
    path: backend
    url: git@github.com:AzraelSec/backend.git
    head: main
    unknown_source_field: rejected
`,
		"component field": `
components:
  api:
    run: go run main.go
    unknown_component_field: rejected
`,
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseConfigV2([]byte(src))
			if err == nil {
				t.Fatal("expected validation error for unknown field")
			}
			// Closed-struct violations report "field not allowed";
			// source violations surface as a failed type disjunction.
			// Either way, the unknown field must be rejected.
			msg := err.Error()
			if !strings.Contains(msg, "field not allowed") && !strings.Contains(msg, "disjunction") {
				t.Errorf("unexpected error shape: %v", err)
			}
		})
	}
}

func TestHydration_ExecutionDir(t *testing.T) {
	const src = `
sources:
  backend:
    type: github
    path: backend
    url: git@github.com:AzraelSec/backend.git
    head: main

components:
  api:
    source: backend
    run: go run main.go

  local-proxy:
    run: caddy reverse-proxy --from :8080 --to :3000

tasks:
  infra-up:
    description: "run docker compose"
    run: docker-compose up -d
`

	cfg, err := ParseConfigV2([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	projectDir := "/home/user/glock"

	api := cfg.Components["api"]
	apiDir := api.ExecutionDir(cfg.Sources, projectDir)
	wantAPI := filepath.Join(projectDir, "backend")
	if apiDir != wantAPI {
		t.Errorf("api execution dir: wanted %s, got %s", wantAPI, apiDir)
	}

	proxy := cfg.Components["local-proxy"]
	proxyDir := proxy.ExecutionDir(cfg.Sources, projectDir)
	if proxyDir != projectDir {
		t.Errorf("proxy execution dir: wanted %s, got %s", projectDir, proxyDir)
	}

	startup := cfg.Tasks["infra-up"]
	topLevelDir := TaskExecutionDir(startup, nil, cfg.Sources, projectDir)
	if topLevelDir != projectDir {
		t.Errorf("top-level task execution dir: wanted %s, got %s", projectDir, topLevelDir)
	}

	componentTaskDir := TaskExecutionDir(startup, &api, cfg.Sources, projectDir)
	if componentTaskDir != wantAPI {
		t.Errorf("component task execution dir: wanted %s, got %s", wantAPI, componentTaskDir)
	}
}

func TestParseConfigV2_EnvFilenames(t *testing.T) {
	const src = `
components:
  api:
    env_filenames:
      - .env
      - .env.local
    run: go run main.go

  worker:
    run: go run cmd/worker.go
`

	cfg, err := ParseConfigV2([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	api, ok := cfg.Components["api"]
	if !ok {
		t.Fatal("missing api component")
	}
	if len(api.EnvFilenames) != 2 || api.EnvFilenames[0] != ".env" || api.EnvFilenames[1] != ".env.local" {
		t.Errorf("api env_filenames: wanted [.env .env.local], got %v", api.EnvFilenames)
	}

	worker, ok := cfg.Components["worker"]
	if !ok {
		t.Fatal("missing worker component")
	}
	if worker.EnvFilenames != nil {
		t.Errorf("worker env_filenames: wanted nil, got %v", worker.EnvFilenames)
	}
}

func TestParseConfigV2_UnknownTaskReference(t *testing.T) {
	const src = `
tasks:
  known:
    description: "a known task"
    run: echo known

on:
  startup:
    - unknown-task
`

	_, err := ParseConfigV2([]byte(src))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "references unknown task") {
		t.Errorf("expected unknown task error, got: %v", err)
	}
}

func TestParseConfigV2_ComponentOnReferences(t *testing.T) {
	const src = `
tasks:
  build:
    description: "build assets"
    run: pnpm run build

components:
  web:
    run: pnpm run dev
    on:
      startup:
        - build
`

	cfg, err := ParseConfigV2([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	comp := cfg.Components["web"]
	if comp.On == nil {
		t.Fatal("expected component on section")
	}
	if len(comp.On.Startup) != 1 || comp.On.Startup[0] != "build" {
		t.Errorf("unexpected startup references: %v", comp.On.Startup)
	}
}

func TestParseConfigV2_ComponentUnknownTaskReference(t *testing.T) {
	const src = `
components:
  web:
    run: pnpm run dev
    on:
      startup:
        - build
`

	_, err := ParseConfigV2([]byte(src))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "components.web.on.startup") {
		t.Errorf("expected component on error, got: %v", err)
	}
}

func TestParseConfigV2_UnknownSourceReference(t *testing.T) {
	const src = `
sources:
  backend:
    type: github
    url: git@github.com:AzraelSec/backend.git
    head: main
    path: backend

components:
  web:
    source: backned
    run: pnpm run dev
`

	_, err := ParseConfigV2([]byte(src))
	if err == nil {
		t.Fatal("expected validation error for dangling source reference")
	}
	if !strings.Contains(err.Error(), `components.web.source references unknown source "backned"`) {
		t.Errorf("expected unknown source error, got: %v", err)
	}
}

func TestParseConfigV2_UnknownWorkspaceComponent(t *testing.T) {
	const src = `
components:
  web:
    run: pnpm run dev

workspaces:
  frontend:
    - web
    - ghost
`

	_, err := ParseConfigV2([]byte(src))
	if err == nil {
		t.Fatal("expected validation error for phantom workspace component")
	}
	if !strings.Contains(err.Error(), `workspaces.frontend references unknown component "ghost"`) {
		t.Errorf("expected unknown component error, got: %v", err)
	}
}

func TestParseConfigV2_EmptyRunRejected(t *testing.T) {
	const src = `
components:
  web:
    run: ""
`

	_, err := ParseConfigV2([]byte(src))
	if err == nil {
		t.Fatal("expected validation error for empty run")
	}
}

func TestParseConfigV2_TaskMissingRunRejected(t *testing.T) {
	const src = `
tasks:
  build:
    description: "no run command"
`

	_, err := ParseConfigV2([]byte(src))
	if err == nil {
		t.Fatal("expected validation error for task without run")
	}
}

func TestDetectConfigVersion_Explicit(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		want    ConfigVersion
		wantErr bool
	}{
		{"v2 explicit", "version: 2\ncomponents:\n  web:\n    run: x", VersionV2, false},
		{"v2 explicit string", "version: \"2\"\n", VersionV2, false},
		{"v1 explicit", "version: 1\nrepos: {}", VersionV1, false},
		{"v2 structural", "components:\n  web:\n    run: x", VersionV2, false},
		{"v1 structural", "repos:\n  a:\n    remote: x", VersionV1, false},
		{"mixed", "version: 2\nrepos: {}\ncomponents: {}", "", true},
		{"empty", "", "", true},
		{"unknown version", "version: 3\n", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectConfigVersion([]byte(tt.src))
			if (err != nil) != tt.wantErr {
				t.Fatalf("DetectConfigVersion error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("DetectConfigVersion = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseConfigV2_OutTraversalRejected(t *testing.T) {
	const src = `
sources:
  evil:
    type: github
    path: ../../escape
    url: git@github.com:AzraelSec/backend.git
    head: main
`

	_, err := ParseConfigV2([]byte(src))
	if err == nil {
		t.Fatal("expected validation error for parent-traversing out")
	}
}

func TestSourceTypesMatchCUESchema(t *testing.T) {
	cs := schemaContext()
	src := cs.ctx.CompileBytes(schemaCue).LookupPath(cue.ParsePath("#Source"))

	_, alts := src.Expr()
	cueTypes := make(map[string]bool, len(alts))
	for _, a := range alts {
		s, err := a.LookupPath(cue.ParsePath("type")).String()
		if err != nil {
			t.Fatalf("read schema source type: %v", err)
		}
		cueTypes[s] = true
	}

	goTypes := []SourceType{
		SourceTypeGitHub,
		SourceTypeHTTP,
		SourceTypeS3,
		SourceTypeFilesystem,
	}
	goSet := make(map[string]bool, len(goTypes))
	for _, st := range goTypes {
		if _, ok := cueTypes[string(st)]; !ok {
			t.Errorf("source type %q exists in Go but not in the CUE schema", st)
		}
		goSet[string(st)] = true
	}
	for tname := range cueTypes {
		if !goSet[tname] {
			t.Errorf("source type %q exists in the CUE schema but not in Go", tname)
		}
	}
	if len(goTypes) != len(cueTypes) {
		t.Errorf("source type counts differ: Go has %d, CUE has %d", len(goTypes), len(cueTypes))
	}
}

func TestSourcePaths(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		valid        bool
	}{
		{"git", `type: github, path: repo, url: 'git@github.com:org/repo.git', head: main`, true},
		{"git missing", `type: github, url: 'git@github.com:org/repo.git', head: main`, false},
		{"git root", `type: github, path: ., url: 'git@github.com:org/repo.git', head: main`, false},
		{"git dot root", `type: github, path: ././, url: 'git@github.com:org/repo.git', head: main`, false},
		{"git absolute", `type: github, path: /tmp/repo, url: 'git@github.com:org/repo.git', head: main`, false},
		{"git traversal", `type: github, path: ../repo, url: 'git@github.com:org/repo.git', head: main`, false},
		{"old out", `type: github, out: repo, url: 'git@github.com:org/repo.git', head: main`, false},
		{"http", `type: http, path: downloads, url: 'https://example.com/a'`, true},
		{"http missing", `type: http, url: 'https://example.com/a'`, false},
		{"s3", `type: s3, path: data, url: 's3://bucket/key'`, true},
		{"s3 missing", `type: s3, url: 's3://bucket/key'`, false},
		{"filesystem absolute", `type: filesystem, path: /opt/tools`, true},
		{"filesystem relative", `type: filesystem, path: ../tools`, true},
		{"filesystem missing", `type: filesystem`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfigV2([]byte("version: 2\nsources:\n  src: {" + tc.source + "}\n"))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestComponentSourceExecutionPaths(t *testing.T) {
	root := t.TempDir()
	key := "local"
	for _, tc := range []struct{ path, want string }{
		{"tools", filepath.Join(root, "tools")},
		{"../outside", filepath.Join(root, "../outside")},
		{"/opt/tools", "/opt/tools"},
	} {
		cfg := Component{Source: &key}
		sources := map[string]Source{key: {Type: SourceTypeFilesystem, Path: &tc.path}}
		if got := cfg.ExecutionDir(sources, root); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.path, got, tc.want)
		}
		if got := TaskExecutionDir(Task{}, &cfg, sources, root); got != tc.want {
			t.Errorf("task %q: got %q want %q", tc.path, got, tc.want)
		}
	}
}
