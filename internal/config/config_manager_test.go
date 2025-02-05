package config

import (
	"testing"

	"github.com/AzraelSec/glock/internal/serializer"
)

const completeConfigYamlSrc = `
root_path: /tmp/
root_main_stream: master
tag_template: v{.Now.Format("02-01-06.150405")}
env_filenames:
  - ".env.defaults"
  - ".env.something"
services:
  - name: "virtual1"
    cmd: "echo start"
    dispose: "echo stop"
  - name: "virtual2"
    cmd: "echo start two"
    dispose: "echo stop two"
repos:
  first:
    remote: git@github.com:AzraelSec/first.git
    main_stream: main
    rel_path: something
  second:
    remote: git@github.com:AzraelSec/second.git
    exclude_tag: true
`

const completeConfigJsonSrc = `{
	"root_path": "/tmp/",
	"root_main_stream": "master",
	"env_filenames": [
  ".env.defaults",
  ".env.something"
	],
	"repos": {
		"first": {
			"remote": "git@github.com:AzraelSec/first.git",
			"main_stream": "main",
			"rel_path": "something"
		},
		"second": {
			"remote": "git@github.com:AzraelSec/second.git",
			"exclude_tag": true
		}
	}
}
`

func TestNewConfigManagerYaml(t *testing.T) {
	cm, err := NewConfigManager("/", []byte(completeConfigYamlSrc), serializer.NewYamlDecoder())
	if err != nil {
		t.Errorf("got error: %v", err)
	}

	repoTests := []struct {
		remote      string
		path        string
		key         string
		main_stream string
		exclude     bool
	}{
		{
			key:         "first",
			remote:      "git@github.com:AzraelSec/first.git",
			main_stream: "main",
			path:        "something",
		},
		{
			key:     "second",
			remote:  "git@github.com:AzraelSec/second.git",
			exclude: true,
		},
	}

	for idx, tt := range repoTests {
		repo := cm.Repos[idx]
		if repo.Config.Remote != tt.remote {
			t.Errorf("wanted: %s, got: %s", tt.remote, repo.Config.Remote)
		}
		if repo.Config.Path != tt.path {
			t.Errorf("wanted: %s, got: %s", tt.path, repo.Config.Path)
		}
		if repo.Config.Refs != tt.main_stream {
			t.Errorf("wanted: %s, got: %s", tt.main_stream, repo.Config.Refs)
		}
		if repo.Config.ExcludeTag != tt.exclude {
			t.Errorf("wanted: %t, got: %t", tt.exclude, repo.Config.ExcludeTag)
		}
	}
}

func TestNewConfigManagerJson(t *testing.T) {
	cm, err := NewConfigManager("/", []byte(completeConfigJsonSrc), serializer.NewJsonDecoder())
	if err != nil {
		t.Errorf("got error: %v", err)
	}

	repoTests := []struct {
		remote      string
		path        string
		key         string
		main_stream string
		exclude     bool
	}{
		{
			key:         "first",
			remote:      "git@github.com:AzraelSec/first.git",
			main_stream: "main",
			path:        "something",
		},
		{
			key:     "second",
			remote:  "git@github.com:AzraelSec/second.git",
			exclude: true,
		},
	}

	for idx, tt := range repoTests {
		repo := cm.Repos[idx]
		if repo.Config.Remote != tt.remote {
			t.Errorf("wanted: %s, got: %s", tt.remote, repo.Config.Remote)
		}
		if repo.Config.Path != tt.path {
			t.Errorf("wanted: %s, got: %s", tt.path, repo.Config.Path)
		}
		if repo.Config.Refs != tt.main_stream {
			t.Errorf("wanted: %s, got: %s", tt.main_stream, repo.Config.Refs)
		}
		if repo.Config.ExcludeTag != tt.exclude {
			t.Errorf("wanted: %t, got: %t", tt.exclude, repo.Config.ExcludeTag)
		}
	}
}
