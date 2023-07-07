package git

import (
	"errors"
	"regexp"
)

const RemoteGitUrlRegex = "((git|ssh|http(s)?)|(git@[\\w\\.]+))(:(//)?)([\\w\\.@\\:/\\-~]+)(\\.git)(/)?"

var (
	RemoteGitUrlErr = errors.New("invalid git remote repository url")
)

func NewRemoteGitUrl(raw string) (RemoteGitUrl, error) {
	match, err := regexp.Match(RemoteGitUrlRegex, []byte(raw))
	if err != nil {
		return "", err
	}

	if !match {
		return "", RemoteGitUrlErr
	}

	return RemoteGitUrl(raw), nil
}
