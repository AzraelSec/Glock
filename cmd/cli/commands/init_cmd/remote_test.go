package init_cmd

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
)

type configTransport struct {
	status int
	raw    string
}

func (r configTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: r.status, Status: http.StatusText(r.status), Body: io.NopCloser(strings.NewReader(r.raw)), Header: make(http.Header)}, nil
}

func TestSetupConfigValidatesBeforeWriting(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	const sources = "sources: {x: {type: github, url: 'git@github.com:org/repo.git', head: main, path: repo}}\n"
	for _, tc := range []configTransport{
		{404, "not found"},
		{200, "repos: {}\n"},
		{200, "version: 2\n" + sources},
		{200, "components: {x: {run: echo}}"},
		{200, "sources: {x: {type: github}}"},
	} {
		http.DefaultTransport = tc
		dir := filepath.Join(t.TempDir(), "project")
		if _, err := setupConfig("https://example.invalid/config", dir); err == nil {
			t.Fatal("invalid remote config accepted")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("invalid config created destination")
		}
	}
	http.DefaultTransport = configTransport{200, sources}
	dir := filepath.Join(t.TempDir(), "project")
	path, err := setupConfig("https://example.invalid/config", dir)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err = setupConfig("https://example.invalid/config", dir); err == nil {
		t.Fatal("overwrote config")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("existing file changed")
	}
}

func TestRemoteInitRejectsMissingYAMLDocument(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	for _, tc := range []struct{ name, raw string }{
		{"blank", ""},
		{"whitespace only", " \n  \n"},
		{"comment only", "# Project configuration\n  # Add sources here\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			http.DefaultTransport = configTransport{http.StatusOK, tc.raw}
			target := filepath.Join(t.TempDir(), "project")
			cmd := remoteCommand(dependency.NewManager())
			cmd.SetArgs([]string{"https://example.invalid/glock.yml", target})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "no YAML document found") {
				t.Fatalf("expected missing YAML document error, got %v", err)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("expected the destination to remain absent, got %v", err)
			}
		})
	}
}

func TestInitReportsCloneFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "repo"), 0700); err != nil {
		t.Fatal(err)
	}
	raw := "sources: {x: {type: github, url: 'git@github.com:org/repo.git', head: main, path: repo}}\n"
	path := filepath.Join(root, "glock.yml")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.ConfigPathEnv, path)
	cmd := localCommand(dependency.NewManager())
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not a usable Git checkout") {
		t.Fatalf("expected clone failure, got %v", err)
	}
}
