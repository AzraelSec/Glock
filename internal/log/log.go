package log

import (
	"fmt"
	"io"

	"github.com/fatih/color"
)

type Logger interface {
	Error(format string, args ...any) (int, error)
	Info(format string, args ...any) (int, error)
	Success(format string, args ...any) (int, error)
	Print(format string, args ...any) (int, error)
}

type repoLogger struct {
	output   io.Writer
	repoName string
}

func NewRepoLogger(output io.Writer, repoName string) Logger {
	return repoLogger{
		output:   output,
		repoName: repoName,
	}
}

func (l repoLogger) Error(format string, args ...any) (int, error) {
	return fmt.Println(color.RedString("[%s]: %s", l.repoName, fmt.Sprintf(format, args...)))
}

func (l repoLogger) Info(format string, args ...any) (int, error) {
	return fmt.Println(color.YellowString("[%s]: %s", l.repoName, fmt.Sprintf(format, args...)))
}

func (l repoLogger) Print(format string, args ...any) (int, error) {
	return fmt.Printf("[%s]: %s\n", l.repoName, args)
}

func (l repoLogger) Success(format string, args ...any) (int, error) {
	return fmt.Println(color.GreenString("[%s]: %s", l.repoName, fmt.Sprintf(format, args...)))
}
