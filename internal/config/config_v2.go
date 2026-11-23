package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	cueyaml "cuelang.org/go/encoding/yaml"
	"gopkg.in/yaml.v3"
)

// ConfigVersion identifies the major configuration schema.
type ConfigVersion string

const (
	VersionV1 ConfigVersion = "v1"
	VersionV2 ConfigVersion = "v2"
)

// DetectConfigVersion inspects the raw YAML and returns the schema
// version it belongs to. An explicit `version` field is authoritative
// but must agree with the structural hints in the document. An
// ambiguous document (no recognized keys, or conflicting version hints)
// is treated as an error so the user gets a clear message instead of a
// cryptic CUE validation failure.
func DetectConfigVersion(data []byte) (ConfigVersion, error) {
	var doc struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("invalid YAML: %w", err)
	}

	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return "", fmt.Errorf("invalid YAML: %w", err)
	}

	v1Keys := map[string]bool{"repos": true, "services": true, "root_path": true, "root_main_stream": true}
	v2Keys := map[string]bool{"sources": true, "components": true, "workspaces": true, "tasks": true}

	var hasV1, hasV2 bool
	for k := range root {
		if v1Keys[k] {
			hasV1 = true
		}
		if v2Keys[k] {
			hasV2 = true
		}
	}

	switch {
	case hasV1 && hasV2:
		return "", fmt.Errorf("config mixes v1 and v2 keys")
	case hasV2:
		detected := VersionV2
		if doc.Version != "" && !versionMatches(doc.Version, detected) {
			return "", fmt.Errorf("version field %q conflicts with v2 structural hints", doc.Version)
		}
		return detected, nil
	case hasV1:
		detected := VersionV1
		if doc.Version != "" && !versionMatches(doc.Version, detected) {
			return "", fmt.Errorf("version field %q conflicts with v1 structural hints", doc.Version)
		}
		return detected, nil
	}

	// No structural hints: an explicit version is required.
	switch doc.Version {
	case "2", "v2":
		return VersionV2, nil
	case "1", "v1":
		return VersionV1, nil
	case "":
		return "", fmt.Errorf("config is missing recognized v1 or v2 keys")
	default:
		return "", fmt.Errorf("unsupported config version %q", doc.Version)
	}
}

func versionMatches(raw string, v ConfigVersion) bool {
	switch v {
	case VersionV1:
		return raw == "1" || raw == "v1"
	case VersionV2:
		return raw == "2" || raw == "v2"
	}
	return false
}

// ConfigV2 is the Go representation of the v2 glock.yml schema.
type ConfigV2 struct {
	TagPattern string               `yaml:"tag_pattern,omitempty" json:"tag_pattern,omitempty"`
	Version    string               `yaml:"version,omitempty" json:"version,omitempty"`
	Sources    map[string]Source    `yaml:"sources,omitempty" json:"sources,omitempty"`
	Components map[string]Component `yaml:"components,omitempty" json:"components,omitempty"`
	Workspaces map[string][]string  `yaml:"workspaces,omitempty" json:"workspaces,omitempty"`
	On         *On                  `yaml:"on,omitempty" json:"on,omitempty"`
	Tasks      map[string]Task      `yaml:"tasks,omitempty" json:"tasks,omitempty"`
}

// SourceType is the discriminant of a Source. The set of supported types is
// kept in sync with the CUE schema by TestSourceTypesMatchCUESchema.
type SourceType string

const (
	SourceTypeGitHub     SourceType = "github"
	SourceTypeHTTP       SourceType = "http"
	SourceTypeS3         SourceType = "s3"
	SourceTypeFilesystem SourceType = "filesystem"
)

// Source declares how to acquire a working directory.
// The Type field discriminates which other fields are valid. CUE validation
// enforces the per-type requirements defined in schema.cue.
type Source struct {
	Updater    string     `yaml:"updater,omitempty" json:"updater,omitempty"`
	ExcludeTag bool       `yaml:"exclude_tag,omitempty" json:"exclude_tag,omitempty"`
	Type       SourceType `yaml:"type" json:"type"`
	URL        *string    `yaml:"url,omitempty" json:"url,omitempty"`
	Head       *string    `yaml:"head,omitempty" json:"head,omitempty"`
	Path       *string    `yaml:"path,omitempty" json:"path,omitempty"`
}

// Component is a logical service or asset.
type Component struct {
	Source       *string  `yaml:"source,omitempty" json:"source,omitempty"`
	EnvFilenames []string `yaml:"env_filenames,omitempty" json:"env_filenames,omitempty"`
	Run          string   `yaml:"run" json:"run"`
	On           *On      `yaml:"on,omitempty" json:"on,omitempty"`
}

// On holds task name references triggered by lifecycle events.
type On struct {
	Startup  []string   `yaml:"startup,omitempty" json:"startup,omitempty"`
	Shutdown []string   `yaml:"shutdown,omitempty" json:"shutdown,omitempty"`
	Steps    []HookStep `yaml:"steps,omitempty" json:"steps,omitempty"`
}

type HookStep struct {
	Startup string `yaml:"startup" json:"startup"`
	Cleanup string `yaml:"cleanup,omitempty" json:"cleanup,omitempty"`
}

// Task is a reusable one-shot command.
type Task struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Run         string `yaml:"run" json:"run"`
}

// compiledSchema holds the shared CUE context and the looked-up config
// schema. The schema is embedded at build time, so a compile failure
// is a programming error and panics on first use.
type compiledSchema struct {
	ctx    *cue.Context
	schema cue.Value
}

// schemaContext compiles the embedded CUE schema once per process.
var schemaContext = sync.OnceValue(func() compiledSchema {
	ctx := cuecontext.New()
	schemaVal := ctx.CompileBytes(schemaCue)
	if err := schemaVal.Err(); err != nil {
		panic(fmt.Sprintf("invalid embedded schema: %v", err))
	}
	return compiledSchema{ctx: ctx, schema: schemaVal.LookupPath(cue.ParsePath("config"))}
})

// ParseConfigV2 parses and validates glock.yml data.
//
// The pipeline is CUE-first: the raw YAML is decoded into a CUE value
// (preserving file positions), unified with the embedded schema, and
// validated — so unknown fields, type errors, and constraint violations
// are reported against the user's file. Only then is the YAML decoded
// into Go structs, followed by referential-integrity checks that the
// schema cannot express (task keys, source keys, component names).
func ParseConfigV2(data []byte) (*ConfigV2, error) {
	cs := schemaContext()

	yamlFile, err := cueyaml.Extract("glock.yml", data)
	if err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	dataVal := cs.ctx.BuildFile(yamlFile)
	if err := dataVal.Err(); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	unified := cs.schema.Unify(dataVal)
	if err := unified.Validate(cue.Final(), cue.Concrete(true)); err != nil {
		return nil, fmt.Errorf("config validation failed: %w\n%s", err, cueerrors.Details(err, nil))
	}

	var cfg ConfigV2
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}

	if err := validateReferences(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// validateReferences checks every cross-reference the CUE schema cannot
// express: task keys used in on.* sections, source keys used by
// components, and component names used by workspaces. All violations
// are collected (sorted for determinism) and returned together.
func validateReferences(cfg *ConfigV2) error {
	var violations []string

	checkTaskRefs := func(section string, keys []string) {
		for _, key := range keys {
			if _, ok := cfg.Tasks[key]; !ok {
				violations = append(violations,
					fmt.Sprintf("%s references unknown task %q", section, key))
			}
		}
	}

	checkSteps := func(section string, on *On) {
		for i, step := range on.Steps {
			checkTaskRefs(fmt.Sprintf("%s.steps[%d].startup", section, i), []string{step.Startup})
			if step.Cleanup != "" {
				checkTaskRefs(fmt.Sprintf("%s.steps[%d].cleanup", section, i), []string{step.Cleanup})
			}
		}
	}
	if cfg.On != nil {
		checkSteps("on", cfg.On)
		checkTaskRefs("on.startup", cfg.On.Startup)
		checkTaskRefs("on.shutdown", cfg.On.Shutdown)
	}

	for compName, comp := range cfg.Components {
		if comp.On != nil {
			checkSteps(fmt.Sprintf("components.%s.on", compName), comp.On)
			checkTaskRefs(fmt.Sprintf("components.%s.on.startup", compName), comp.On.Startup)
			checkTaskRefs(fmt.Sprintf("components.%s.on.shutdown", compName), comp.On.Shutdown)
		}
		if comp.Source != nil {
			if _, ok := cfg.Sources[*comp.Source]; !ok {
				violations = append(violations,
					fmt.Sprintf("components.%s.source references unknown source %q", compName, *comp.Source))
			}
		}
	}

	for wsName, compNames := range cfg.Workspaces {
		for _, compName := range compNames {
			if _, ok := cfg.Components[compName]; !ok {
				violations = append(violations,
					fmt.Sprintf("workspaces.%s references unknown component %q", wsName, compName))
			}
		}
	}

	if len(violations) == 0 {
		return nil
	}
	slices.Sort(violations)
	errs := make([]error, len(violations))
	for i, v := range violations {
		errs[i] = errors.New(v)
	}
	return errors.Join(errs...)
}

// LocalDir resolves the local source directory. Filesystem paths may be absolute;
// remote source paths are validated as relative to the project directory.
func (s Source) LocalDir(projectDir string) string {
	if s.Path == nil {
		return ""
	}
	if filepath.IsAbs(*s.Path) {
		return filepath.Clean(*s.Path)
	}
	return filepath.Join(projectDir, *s.Path)
}

// ExecutionDir resolves a component's source directory, or the project root
// for components with no source.
func (c *Component) ExecutionDir(sources map[string]Source, projectDir string) string {
	if c.Source != nil {
		if src, ok := sources[*c.Source]; ok {
			return src.LocalDir(projectDir)
		}
	}
	return projectDir
}

// TaskExecutionDir returns the working directory for a task.
// If the task belongs to a component, it uses that component's execution dir.
// Top-level tasks run in the project directory.
func TaskExecutionDir(task Task, component *Component, sources map[string]Source, projectDir string) string {
	if component != nil {
		return component.ExecutionDir(sources, projectDir)
	}
	return projectDir
}
