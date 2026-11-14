package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/AzraelSec/glock/internal/config"
	"github.com/AzraelSec/glock/internal/dependency"
	"github.com/AzraelSec/glock/internal/engine"
	"github.com/AzraelSec/glock/internal/frontend"
	"github.com/AzraelSec/glock/internal/log"
	"github.com/AzraelSec/glock/internal/process"
	"github.com/AzraelSec/glock/internal/runtime"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func selectedComponents(cfg *config.ConfigV2, repos []string) ([]string, error) {
	selected := map[string]bool{}
	for _, name := range repos {
		src, ok := cfg.Sources[name]
		if !ok || src.Type != config.SourceTypeGitHub {
			return nil, fmt.Errorf("unknown Git source %q in --repos", name)
		}
		selected[name] = true
	}
	names := []string{}
	for name, c := range cfg.Components {
		if len(repos) == 0 || (c.Source != nil && selected[*c.Source]) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(repos) > 0 && len(names) == 0 {
		return nil, errors.New("selected Git sources have no runnable components")
	}
	return names, nil
}

func startFactory(dm *dependency.Manager) *cobra.Command {
	var repos []string
	cmd := &cobra.Command{Use: "start", Short: "Start the development stack 🚀", RunE: func(cmd *cobra.Command, args []string) error {
		project, err := dm.LoadProject()
		if err != nil {
			return err
		}
		names, err := selectedComponents(project.Config, repos)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := runStart(ctx, project, names, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), color.GreenString("Execution completed"))
		return nil
	}}
	cmd.Flags().StringArrayVarP(&repos, "repos", "r", nil, "list of repository you want to start")
	return cmd
}

func runStart(ctx context.Context, project *config.Project, names []string, stdout, stderr io.Writer) (err error) {
	session := engine.OpenProject(project)
	client, e := frontend.New(context.Background(), session)
	if e != nil {
		_ = session.Close()
		return e
	}
	outputErrors := make(chan error, 1)
	var output sync.WaitGroup
	output.Add(1)
	go func() {
		defer output.Done()
		writers := map[string]*log.TagStreamWriter{}
		failed := false
		for line := range session.Output() {
			if failed {
				continue
			}
			key := line.Kind + ":" + line.Name + ":" + line.Stream
			writer := writers[key]
			if writer == nil {
				dest := stdout
				if line.Stream == "stderr" {
					dest = stderr
				}
				writer = log.NewTagStreamWriter(line.Name, dest)
				writers[key] = writer
			}
			if _, e := fmt.Fprintln(writer, line.Line); e != nil {
				failed = true
				outputErrors <- e
			}
		}
	}()
	defer func() {
		shutdownErr := client.Execute(context.Background(), runtime.Command{Action: runtime.Shutdown})
		closeErr := session.Close()
		output.Wait()
		client.Close()
		select {
		case e := <-outputErrors:
			err = errors.Join(err, e)
		default:
		}
		err = errors.Join(err, shutdownErr, closeErr)
	}()
	launched := make(chan error, 1)
	go func() {
		e := client.Execute(ctx, runtime.Command{Action: runtime.Initialize})
		if e == nil {
			results := make(chan error, len(names))
			var wg sync.WaitGroup
			for _, name := range names {
				wg.Add(1)
				go func() {
					defer wg.Done()
					results <- client.Execute(ctx, runtime.Command{Action: runtime.StartComponent, Target: name})
				}()
			}
			wg.Wait()
			close(results)
			for result := range results {
				e = errors.Join(e, result)
			}
		}
		launched <- e
	}()
	select {
	case <-ctx.Done():
		return nil
	case e := <-outputErrors:
		return e
	case e := <-launched:
		if e != nil {
			return e
		}
	}
	if len(names) == 0 {
		return nil
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	targets := map[string]bool{}
	for _, name := range names {
		targets[name] = true
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-outputErrors:
			return e
		case <-ticker.C:
			finished := 0
			var failures []error
			for _, entry := range client.Components(runtime.WorkspaceAll) {
				if !targets[entry.Component.Name] || entry.Process == nil {
					continue
				}
				switch entry.Process.State {
				case process.Stopped:
					finished++
				case process.Failed:
					finished++
					failures = append(failures, fmt.Errorf("component %s failed", entry.Component.Name))
				}
			}
			if len(failures) > 0 {
				return errors.Join(failures...)
			}
			if finished == len(names) {
				return nil
			}
		}
	}
}
