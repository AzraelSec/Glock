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
	"github.com/AzraelSec/glock/internal/log"
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	run := engine.StartRun(ctx, project, names, failFast)
	outputErrors := make(chan error, 1)
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		writers := map[string]*log.TagStreamWriter{}
		failed := false
		reportErr := func(e error) {
			if !failed {
				failed = true
				outputErrors <- e
				cancel()
			}
		}
		writeReport := func(e error) {
			if !failed {
				if _, err := fmt.Fprintln(stderr, e); err != nil {
					reportErr(err)
				}
			}
		}
		reports, lines := run.Failures, run.Output
		for reports != nil || lines != nil {
			select {
			case e, ok := <-reports:
				if !ok {
					reports = nil
					continue
				}
				writeReport(e)
			case line, ok := <-lines:
				if !ok {
					lines = nil
					continue
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
		loss := run.OutputLoss()
		if !failed && (loss.DroppedChunks > 0 || loss.TruncatedDrains > 0) {
			_, e := fmt.Fprintf(stderr, "[glock] warning: output incomplete (%d chunks dropped, %d pipe drains truncated)\n", loss.DroppedChunks, loss.TruncatedDrains)
			if e != nil {
				reportErr(e)
			}
		}
	}()
	err = run.Wait()
	select {
	case <-outputDone:
	case <-time.After(2 * time.Second):
		err = errors.Join(err, errors.New("output incomplete: terminal writer did not drain before shutdown"))
	}
	select {
	case e := <-outputErrors:
		return errors.Join(err, e)
	default:
		return err
	}
}
