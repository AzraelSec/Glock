package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Project is validated configuration and its absolute execution root.
type Project struct {
	Config     *ConfigV2
	Path, Root string
}

func LoadFile(path string) (*Project, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(absolute)
	if err != nil {
		return nil, err
	}
	version, err := DetectConfigVersion(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", absolute, err)
	}
	if version != VersionV2 {
		return nil, fmt.Errorf("%s: v1 configuration is no longer supported; run glock config migrate --output <project-root>/glock.yml (move or back up the original first if it occupies that destination)", absolute)
	}
	cfg, err := ParseConfigV2(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", absolute, err)
	}
	return &Project{Config: cfg, Path: absolute, Root: filepath.Dir(absolute)}, nil
}
