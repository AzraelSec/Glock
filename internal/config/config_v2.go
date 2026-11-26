package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	cueyaml "cuelang.org/go/encoding/yaml"
	"gopkg.in/yaml.v3"
)

// ConfigV2 is the Go representation of the v2 glock.yml schema.
type ConfigV2 struct {
	TagPattern string               `yaml:"tag_pattern,omitempty" json:"tag_pattern,omitempty"`
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
	return parseConfigV2("glock.yml", data)
}

func parseConfigV2(filename string, data []byte) (*ConfigV2, error) {
	cs := schemaContext()

	yamlFile, err := cueyaml.Extract(filename, data)
	if err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	dataVal := cs.ctx.BuildFile(yamlFile)
	if err := dataVal.Err(); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	unified := cs.schema.Unify(dataVal)
	if err := unified.Validate(cue.Final(), cue.Concrete(true)); err != nil {
		return nil, fmt.Errorf("config validation failed:\n%s", strings.TrimSuffix(cueerrors.Details(err, nil), "\n"))
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
