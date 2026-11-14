package commands

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AzraelSec/glock/cmd/cli/commands/init_cmd"
	"github.com/AzraelSec/glock/cmd/cli/commands/status"
	switchcmd "github.com/AzraelSec/glock/cmd/cli/commands/switch_cmd"
	"github.com/AzraelSec/glock/cmd/cli/commands/tag"
	"github.com/AzraelSec/glock/cmd/cli/commands/update"
	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/spf13/cobra"
)

func TestGitCommandsRejectInapplicableConfiguration(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"v1", "repos: {}\n", "config migrate"},
		{"non-git", "version: 2\ncomponents: {local: {run: echo hello}}\n", "no Git sources"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "glock.yml")
			if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(config.ConfigPathEnv, path)
			dm := dependency.NewManager()
			commands := []*cobra.Command{init_cmd.New(dm).Command(), status.NewStatus(dm).Command(), switchcmd.New(dm).Command(), resetFactory(dm), update.NewUpdate(dm).Command(), tag.New(dm).Command()}
			for _, cmd := range commands {
				t.Run(cmd.Name(), func(t *testing.T) {
					cmd.SilenceErrors = true
					cmd.SilenceUsage = true
					cmd.SetOut(io.Discard)
					cmd.SetErr(io.Discard)
					args := []string{}
					if cmd.Name() == "init" {
						args = []string{"local"}
					}
					cmd.SetArgs(args)
					if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
						t.Fatalf("want %s, got %v", tc.want, err)
					}
				})
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 1 {
				t.Fatal("command caused side effects before config rejection")
			}
		})
	}
}

func TestStartRejectsV1AndUnknownFilterBeforeHooks(t *testing.T) {
	for _, raw := range []string{"repos: {}\n", "version: 2\non: {startup: [side-effect]}\ntasks: {side-effect: {run: touch forbidden}}\n"} {
		root := t.TempDir()
		path := filepath.Join(root, "glock.yml")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(config.ConfigPathEnv, path)
		cmd := startFactory(dependency.NewManager())
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--repos", "unknown"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("invalid configuration/filter accepted")
		}
		if _, err := os.Stat(filepath.Join(root, "forbidden")); !os.IsNotExist(err) {
			t.Fatal("hook ran before filter validation")
		}
	}
}
