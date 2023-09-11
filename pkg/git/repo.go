package git

type Repo struct {
	Remote RemoteGitUrl `yaml:"remote" json:"remote,omitempty"`
	Path   string       `yaml:"rel_path" json:"rel_path,omitempty"`
	Refs   BranchName   `yaml:"main_stream" json:"main_stream,omitempty"`
}
