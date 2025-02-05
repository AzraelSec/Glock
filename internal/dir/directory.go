package dir

import (
	"errors"
	"os"
	"path"
)

type Directory interface {
	Path() string
	Exists(name string) bool
	Directories() ([]string, error)
	Files() ([]string, error)
	Entries() ([]string, error)
}

type osDirectory struct {
	path string
}

func NewDirectory(path string) (Directory, error) {
	if !DirExists(path) {
		return nil, errors.New("unexistent directory")
	}
	return osDirectory{
		path: path,
	}, nil
}

func (d osDirectory) Directories() ([]string, error) {
	res := []string{}
	entries, err := os.ReadDir(d.path)
	if err != nil {
		return res, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			res = append(res, entry.Name())
		}
	}
	return res, err
}

func (d osDirectory) Entries() ([]string, error) {
	res := []string{}
	entries, err := os.ReadDir(d.path)
	if err != nil {
		return res, err
	}

	for _, entry := range entries {
		res = append(res, entry.Name())
	}
	return res, err
}

func (d osDirectory) Files() ([]string, error) {
	res := []string{}
	entries, err := os.ReadDir(d.path)
	if err != nil {
		return res, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			res = append(res, entry.Name())
		}
	}
	return res, err
}

func (d osDirectory) Path() string {
	return d.path
}

func (d osDirectory) Exists(name string) bool {
	return DirExists(path.Join(d.path, name))
}
