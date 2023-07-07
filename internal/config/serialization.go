package config

import "errors"

type Global struct {
	OpenCommand string `yaml:"open_command,omitempty" json:"open_command,omitempty"`
	RootPath    string `yaml:"root_path,omitempty" json:"root_path,omitempty"`
	RootRefs    string `yaml:"root_main_stream,omitempty" json:"root_main_stream"`
}

type Credentials struct {
	Github   string `yaml:"github_token,omitempty" json:"github_token,omitempty"`
	Shortcut string `yaml:"shortcut_token,omitempty" json:"shortcut_token,omitempty"`
}

type Repo struct {
	Updater    string `yaml:"updater,omitempty" json:"updater,omitempty"`
	StartCmd   string `yaml:"start_cmd,omitempty" json:"start_cmd,omitempty"`
	StopCmd    string `yaml:"stop_cmd,omitempty" json:"stop_cmd,omitempty"`
	ExcludeTag bool   `yaml:"exclude_tag,omitempty" json:"exclude_tag,omitempty"`
	Remote     string `yaml:"remote,omitempty" json:"remote,omitempty"`
	Path       string `yaml:"rel_path,omitempty" json:"rel_path,omitempty"`
	Refs       string `yaml:"main_stream,omitempty" json:"main_stream,omitempty"`
}

type Config struct {
	Credentials Credentials      `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	Repos       map[string]*Repo `yaml:"repos" json:"repos"`
	Global      `yaml:",inline"`
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
