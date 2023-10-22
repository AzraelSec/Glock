package git

type BranchName string
type RemoteGitUrl string

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
	Fetch(repo Repo) error
	Status(repo Repo) (StatusRes, error)
	CurrentBranch(repo Repo) (BranchName, error)
	HasChanges(repo Repo) (bool, error)
	DiffersFromRemote(repo Repo) (bool, error)
	Switch(repo Repo, branch BranchName, force bool) error
	Pull(repo Repo, rebase bool) error
	ListBranches(repo Repo) ([]BranchName, error)
}
