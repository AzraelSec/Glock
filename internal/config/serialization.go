package config

import "errors"

type Global struct {
	OpenCommand  string   `yaml:"open_command" json:"open_command,omitempty"`
	RootPath     string   `yaml:"root_path" json:"root_path,omitempty"`
	RootRefs     string   `yaml:"root_main_stream" json:"root_main_stream"`
	EnvFilenames []string `yaml:"env_filenames" json:"env_filenames"`
}

type ConfigRepo struct {
	Updater    string   `yaml:"updater" json:"updater,omitempty"`
	OnStart    []string `yaml:"on_start" json:"on_start,omitempty"`
	OnStop     string   `yaml:"on_stop" json:"on_stop,omitempty"`
	ExcludeTag bool     `yaml:"exclude_tag" json:"exclude_tag,omitempty"`
	Remote     string   `yaml:"remote" json:"remote,omitempty"`
	Path       string   `yaml:"rel_path" json:"rel_path,omitempty"`
	Refs       string   `yaml:"main_stream" json:"main_stream,omitempty"`
}

type Services struct {
	Tag     string `yaml:"tag" json:"tag,omitempty"`
	Cmd     string `yaml:"cmd" json:"cmd,omitempty"`
	Dispose string `yaml:"dispose" json:"dispose,omitempty"`
}

type Config struct {
	Repos    map[string]*ConfigRepo `yaml:"repos" json:"repos"`
	Services []Services             `yaml:"services" json:"services"`
	Global   `yaml:",inline"`
}

var ConfigValidationErr = errors.New("invalid config")

func (c Config) Validate() error {
	for _, rp := range c.Repos {
		if rp == nil || rp.Remote == "" {
			return ConfigValidationErr
		}
	}
	return nil
}
