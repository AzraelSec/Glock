package git

type BranchName string
type RemoteGitUrl string
type Remote string
type Tag string

type CloneOps struct {
	Remote RemoteGitUrl
	Path   string
	Refs   BranchName
}

type StatusRes struct {
	Change bool
	Branch BranchName
}

type Git interface {
	Clone(ops CloneOps) error
	ListRemotes(repo Repo) ([]Remote, error)
	Fetch(repo Repo) error
	Status(repo Repo) (StatusRes, error)
	CurrentBranch(repo Repo) (BranchName, error)
	HasChanges(repo Repo) (bool, error)
	DiffersFromRemote(repo Repo) (bool, error)
	Switch(repo Repo, branch BranchName, force bool) error
	Pull(repo Repo, rebase bool) error
	PullBranch(repo Repo, branch BranchName, rebase bool) error
	ListBranches(repo Repo) ([]BranchName, error)
	CreateLightweightTag(repo Repo, tag string, branch BranchName) error
	PushTag(repo Repo, tag Tag, remote Remote) error
}
