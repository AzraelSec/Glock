package config

import "errors"

type GlobalV1 struct {
	RootPath     string   `yaml:"root_path" json:"root_path,omitempty"`
	RootRefs     string   `yaml:"root_main_stream" json:"root_main_stream"`
	EnvFilenames []string `yaml:"env_filenames" json:"env_filenames"`
	TagPattern   string   `yaml:"tag_pattern" json:"tag_pattern"`
}

type ConfigRepoV1 struct {
	Updater    string   `yaml:"updater" json:"updater,omitempty"`
	OnStart    []string `yaml:"on_start" json:"on_start,omitempty"`
	OnStop     string   `yaml:"on_stop" json:"on_stop,omitempty"`
	ExcludeTag bool     `yaml:"exclude_tag" json:"exclude_tag,omitempty"`
	Remote     string   `yaml:"remote" json:"remote,omitempty"`
	Path       string   `yaml:"rel_path" json:"rel_path,omitempty"`
	Refs       string   `yaml:"main_stream" json:"main_stream,omitempty"`
}

type ServicesV1 struct {
	Tag     string `yaml:"tag" json:"tag,omitempty"`
	Cmd     string `yaml:"cmd" json:"cmd,omitempty"`
	Dispose string `yaml:"dispose" json:"dispose,omitempty"`
}

type ConfigV1 struct {
	Version  string                   `yaml:"version,omitempty"`
	Repos    map[string]*ConfigRepoV1 `yaml:"repos" json:"repos"`
	Services []ServicesV1             `yaml:"services" json:"services"`
	GlobalV1 `yaml:",inline"`
}

var ErrConfigValidation = errors.New("invalid config")

func (c ConfigV1) Validate() error {
	for _, rp := range c.Repos {
		if rp == nil || rp.Remote == "" {
			return ErrConfigValidation
		}
	}
	return nil
}
