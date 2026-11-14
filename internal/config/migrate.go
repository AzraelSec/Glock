package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// MigrateV1 converts only configurations whose execution semantics can be
// represented unambiguously. It does not execute commands or write files.
func MigrateV1(raw []byte, input, explicitRoot string) ([]byte, string, error) {
	version, err := DetectConfigVersion(raw)
	if err != nil {
		return nil, "", err
	}
	if version != VersionV1 {
		return nil, "", errors.New("migration requires a v1 configuration")
	}
	var old ConfigV1
	var blockers []error
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&old); err != nil {
		blockers = append(blockers, fmt.Errorf("legacy fields: %w; remove or convert unsupported fields manually", err))
	}
	if err := old.Validate(); err != nil {
		blockers = append(blockers, err)
	}
	input, err = filepath.Abs(input)
	if err != nil {
		return nil, "", err
	}
	root := filepath.Dir(input)
	if old.RootPath != "" && !filepath.IsAbs(old.RootPath) {
		if explicitRoot == "" {
			blockers = append(blockers, errors.New("relative v1 root_path is ambiguous; supply --v1-root with the absolute effective project root"))
		} else if !filepath.IsAbs(explicitRoot) {
			blockers = append(blockers, errors.New("--v1-root must be an absolute directory"))
		} else {
			root = filepath.Clean(explicitRoot)
		}
	} else {
		if explicitRoot != "" {
			blockers = append(blockers, errors.New("--v1-root is only valid when v1 root_path is relative"))
		}
		if old.RootPath != "" {
			root = filepath.Clean(old.RootPath)
		}
	}
	cfg := ConfigV2{Version: "2", TagPattern: old.TagPattern, Sources: map[string]Source{}, Components: map[string]Component{}, Tasks: map[string]Task{}}
	names := make([]string, 0, len(old.Repos))
	for name := range old.Repos {
		names = append(names, name)
	}
	sort.Strings(names)
	used := map[string]bool{}
	for _, name := range names {
		used[name] = true
	}
	unique := func(base string) string {
		name := base
		for i := 2; used[name]; i++ {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		used[name] = true
		return name
	}
	addHook := func(name, run string) *On {
		key := unique(name + "-shutdown")
		cfg.Tasks[key] = Task{Run: run}
		return &On{Shutdown: []string{key}}
	}
	for _, name := range names {
		repo := old.Repos[name]
		if repo == nil {
			continue
		}
		if len(repo.OnStart) > 1 {
			blockers = append(blockers, fmt.Errorf("repos.%s.on_start: multiple commands require manual component splitting", name))
		}
		if len(repo.OnStart) == 0 && repo.OnStop != "" {
			blockers = append(blockers, fmt.Errorf("repos.%s.on_stop: shutdown-only entry needs a manual lifecycle decision", name))
		}
		out := repo.Path
		if out == "" {
			out = name
		}
		if filepath.Clean(out) == "." || filepath.IsAbs(out) || out == ".." || strings.HasPrefix(filepath.Clean(out), ".."+string(filepath.Separator)) {
			blockers = append(blockers, fmt.Errorf("repos.%s.rel_path: path must be a non-root directory within the project; relocate manually", name))
		}
		out = filepath.ToSlash(filepath.Clean(out))
		head := repo.Refs
		if head == "" {
			head = old.RootRefs
		}
		if head == "" {
			head = "main"
		}
		cfg.Sources[name] = Source{Type: SourceTypeGitHub, URL: &repo.Remote, Head: &head, Path: &out, Updater: repo.Updater, ExcludeTag: repo.ExcludeTag}
		if len(repo.OnStart) == 1 {
			c := Component{Source: &name, Run: repo.OnStart[0], EnvFilenames: old.EnvFilenames}
			if repo.OnStop != "" {
				c.On = addHook(name, repo.OnStop)
			}
			cfg.Components[name] = c
		}
	}
	for i, srv := range old.Services {
		if root != filepath.Dir(input) {
			blockers = append(blockers, fmt.Errorf("services[%d]: project root differs from original service execution directory; relocate manually", i))
		}
		if srv.Cmd == "" {
			blockers = append(blockers, fmt.Errorf("services[%d].cmd: missing run command; convert manually", i))
			continue
		}
		name := srv.Tag
		if name == "" {
			name = fmt.Sprintf("service-%d", i+1)
		}
		name = unique(name)
		c := Component{Run: srv.Cmd}
		if srv.Dispose != "" {
			c.On = addHook(name, srv.Dispose)
		}
		cfg.Components[name] = c
	}
	if err := errors.Join(blockers...); err != nil {
		return nil, "", err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, "", err
	}
	if _, err = ParseConfigV2(data); err != nil {
		return nil, "", fmt.Errorf("converted config: %w", err)
	}
	return data, root, nil
}

// WriteMigration never overwrites an existing file, including the input.
func WriteMigration(input, output, explicitRoot string) error {
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	data, root, err := MigrateV1(raw, input, explicitRoot)
	if err != nil {
		return err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	if filepath.Base(output) != ConfigFileName || filepath.Dir(output) != root {
		return fmt.Errorf("output must be %s (the old effective project root)", filepath.Join(root, ConfigFileName))
	}
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create migration output: %w; move or back up the original if it occupies this destination", err)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if err = errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(output)
		return err
	}
	return nil
}
