package git

type Repo struct {
	Remote RemoteGitUrl `yaml:"remote,omitempty" json:"remote,omitempty"`
	Path   string       `yaml:"rel_path,omitempty" json:"rel_path,omitempty"`
	Refs   BranchName   `yaml:"main_stream,omitempty" json:"main_stream,omitempty"`
}
