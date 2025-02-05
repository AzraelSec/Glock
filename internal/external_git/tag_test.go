package external_git

import (
	"testing"

	"github.com/AzraelSec/glock/internal/git"
)

func TestNewCreateLightWaightTag(t *testing.T) {
	repo := git.Repo{
		Remote: "origin",
		Path:   "./repo",
		Refs:   "git@githublocal.com/user/repo",
	}

	tests := []struct {
		tag    string
		branch string
		want   string
	}{
		{
			tag:    "v1",
			branch: "master",
			want:   "git --git-dir ./repo/.git --work-tree ./repo tag v1 master",
		},
	}
	for _, tt := range tests {
		if s := newCreateLightWeightTag(repo, tt.tag, git.BranchName(tt.branch)); s.String() != tt.want {
			t.Errorf("wrong push command. newCreateLightWeightTag(repo, %s, %s)=%s, want=%s", tt.tag, tt.branch, s, tt.want)
		}
	}
}

func TestNewPushTag(t *testing.T) {
	repo := git.Repo{
		Remote: "origin",
		Path:   "./repo",
		Refs:   "git@githublocal.com/user/repo",
	}

	tests := []struct {
		tag    string
		remote string
		want   string
	}{
		{
			tag:    "v1",
			remote: "origin",
			want:   "git --git-dir ./repo/.git --work-tree ./repo push origin v1",
		},
	}
	for _, tt := range tests {
		if s := newPushTag(repo, git.Tag(tt.tag), git.Remote(tt.remote)); s.String() != tt.want {
			t.Errorf("wrong push command. newPushTag(repo, %s, %s)=%s, want=%s", tt.tag, tt.remote, s, tt.want)
		}
	}
}
