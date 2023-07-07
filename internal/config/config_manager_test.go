package config

import (
	"testing"

	"github.com/AzraelSec/glock/pkg/serializer"
)

const completeConfigYamlSrc = `
open_command: open
root_path: /tmp/
root_main_stream: master
credentials:
  github_token: G1tHub
  shortcut_token: 5h0r7cu7
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
	"open_command": "open",
	"root_path": "/tmp/",
	"root_main_stream": "master",
	"credentials": {
		"github_token": "G1tHub",
		"shortcut_token": "5h0r7cu7"
	},
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

	for _, tt := range repoTests {
		repo := cm.Repos[tt.key]
		if repo == nil {
			t.Errorf("unexpected nil value for %s", tt.key)
		}

		if repo.Remote != tt.remote {
			t.Errorf("wanted: %s, got: %s", tt.remote, repo.Remote)
		}
		if repo.Path != tt.path {
			t.Errorf("wanted: %s, got: %s", tt.path, repo.Path)
		}
		if repo.Refs != tt.main_stream {
			t.Errorf("wanted: %s, got: %s", tt.main_stream, repo.Refs)
		}
		if repo.ExcludeTag != tt.exclude {
			t.Errorf("wanted: %t, got: %t", tt.exclude, repo.ExcludeTag)
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

	for _, tt := range repoTests {
		repo := cm.Repos[tt.key]
		if repo == nil {
			t.Errorf("unexpected nil value for %s", tt.key)
		}

		if repo.Remote != tt.remote {
			t.Errorf("wanted: %s, got: %s", tt.remote, repo.Remote)
		}
		if repo.Path != tt.path {
			t.Errorf("wanted: %s, got: %s", tt.path, repo.Path)
		}
		if repo.Refs != tt.main_stream {
			t.Errorf("wanted: %s, got: %s", tt.main_stream, repo.Refs)
		}
		if repo.ExcludeTag != tt.exclude {
			t.Errorf("wanted: %t, got: %t", tt.exclude, repo.ExcludeTag)
		}
	}
}
