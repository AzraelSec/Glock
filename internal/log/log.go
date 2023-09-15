package log

import (
	"fmt"
	"io"

	"github.com/AzraelSec/glock/pkg/log"
	"github.com/fatih/color"
)

type repoLogger struct {
	output   io.Writer
	repoName string
}

func NewRepoLogger(output io.Writer, repoName string) log.Logger {
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
