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
	cfg, err := parseConfigV2(absolute, raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", absolute, err)
	}
	return &Project{Config: cfg, Path: absolute, Root: filepath.Dir(absolute)}, nil
}
