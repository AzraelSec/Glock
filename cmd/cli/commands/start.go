package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
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
	var failFast bool
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
		if err := runStart(ctx, project, names, cmd.OutOrStdout(), cmd.ErrOrStderr(), failFast); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), color.GreenString("Execution completed"))
		return nil
	}}
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop the stack when a component fails")
	cmd.Flags().StringArrayVarP(&repos, "repos", "r", nil, "list of repository you want to start")
	return cmd
}

func runStart(ctx context.Context, project *config.Project, names []string, stdout, stderr io.Writer, failFast bool) (err error) {
	session := engine.OpenProject(project)
	client, e := frontend.New(context.Background(), session)
	if e != nil {
		_ = session.Close()
		return e
	}

	outputErrors := make(chan error, 1)
	reports := make(chan error, len(names)+1)
	report := func(e error) {
		select {
		case reports <- e:
		default:
		}
	}
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		writers := map[string]*log.TagStreamWriter{}
		failed := false
		reportErr := func(e error) {
			if !failed {
				failed = true
				outputErrors <- e
			}
		}
		writeReport := func(e error) {
			if !failed {
				if _, err := fmt.Fprintln(stderr, e); err != nil {
					reportErr(err)
				}
			}
		}
		for {
			select {
			case e := <-reports:
				writeReport(e)
			case line, ok := <-session.Output():
				if !ok {
					for {
						select {
						case e := <-reports:
							writeReport(e)
						default:
							loss := session.OutputLoss()
							if !failed && (loss.DroppedChunks > 0 || loss.TruncatedDrains > 0) {
								if _, err := fmt.Fprintf(stderr, "[glock] warning: output incomplete (%d chunks dropped, %d pipe drains truncated)\n", loss.DroppedChunks, loss.TruncatedDrains); err != nil {
									reportErr(err)
								}
							}
							return
						}
					}
				}
				if line.Truncated || failed {
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
				text := line.Line
				if line.Newline {
					text += "\n"
				}
				if _, e := fmt.Fprint(writer, text); e != nil {
					reportErr(e)
				}
			}
		}
	}()
	defer func() {
		shutdownErr := client.Execute(context.Background(), runtime.Command{Action: runtime.Shutdown})
		closeErr := session.Close()
		select {
		case <-outputDone:
		case <-time.After(2 * time.Second):
			err = errors.Join(err, errors.New("output incomplete: terminal writer did not drain before shutdown"))
		}
		client.Close()
		select {
		case e := <-outputErrors:
			err = errors.Join(err, e)
		default:
		}
		err = errors.Join(err, shutdownErr, closeErr)
	}()

	initialized := make(chan error, 1)
	go func() { initialized <- client.Execute(ctx, runtime.Command{Action: runtime.Initialize}) }()
	select {
	case <-ctx.Done():
		return nil
	case e := <-outputErrors:
		return e
	case e := <-initialized:
		if e != nil {
			return e
		}
	}
	if len(names) == 0 {
		return nil
	}
	type componentResult struct {
		name string
		err  error
	}
	results := make(chan componentResult, len(names))
	pending := make(map[string]bool, len(names))
	for _, name := range names {
		pending[name] = true
		go func() {
			results <- componentResult{name, client.Execute(ctx, runtime.Command{Action: runtime.StartComponent, Target: name})}
		}()
	}
	failures := map[string]error{}
	finished := map[string]bool{}
	targets := map[string]bool{}
	for _, name := range names {
		targets[name] = true
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.Join(mapErrors(failures)...)
		case e := <-outputErrors:
			return e
		case result := <-results:
			delete(pending, result.name)
			if result.err != nil {
				failure := fmt.Errorf("component %s: %w", result.name, result.err)
				failures[result.name] = failure
				report(failure)
				if failFast {
					return errors.Join(mapErrors(failures)...)
				}
			}
		case <-ticker.C:
			for _, entry := range client.Components(runtime.WorkspaceAll) {
				name := entry.Component.Name
				if !targets[name] || pending[name] || failures[name] != nil || finished[name] || entry.Process == nil {
					continue
				}
				switch entry.Process.State {
				case process.Stopped:
					finished[name] = true
				case process.Failed:
					failure := fmt.Errorf("component %s failed", name)
					report(failure)
					if !failFast {
						cleanupErr := client.Execute(ctx, runtime.Command{Action: runtime.StopComponent, Target: name})
						if cleanupErr != nil && ctx.Err() == nil {
							failure = errors.Join(failure, fmt.Errorf("component %s cleanup: %w", name, cleanupErr))
						}
					}
					failures[name] = failure
					if failFast {
						return errors.Join(mapErrors(failures)...)
					}
				}
			}
		}
		if len(pending) == 0 && len(finished)+len(failures) == len(names) {
			return errors.Join(mapErrors(failures)...)
		}
	}

}

func mapErrors(failures map[string]error) []error {
	names := make([]string, 0, len(failures))
	for name := range failures {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]error, 0, len(names))
	for _, name := range names {
		out = append(out, failures[name])
	}
	return out
}
