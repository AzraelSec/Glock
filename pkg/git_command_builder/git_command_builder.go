package gitcb

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/AzraelSec/glock/pkg/git"
)

type executor interface {
	Output() ([]byte, error, int)
}

type CommandBuilder struct {
	exec       func(e string, args ...string) executor
	repo       *git.Repo
	entryPoint string
	args       []string
}

type commandBuilderExecutor struct {
	exec.Cmd
}

func (be commandBuilderExecutor) Output() ([]byte, error, int) {
	o, e := be.Cmd.Output()
	return o, e, be.Cmd.ProcessState.ExitCode()
}

func (e *commandBuilderExecutor) ExitCode() int {
	return e.Cmd.ProcessState.ExitCode()
}

func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{
		entryPoint: "git", // todo: change this to dynamic
		args:       []string{},
		exec: func(e string, args ...string) executor {
			cmd := exec.Command(e, args...)
			return &commandBuilderExecutor{*cmd}
		},
	}
}

func (cb *CommandBuilder) SetRepo(r git.Repo) *CommandBuilder {
	if cb.repo == nil {
		cb.repo = &r
	}
	return cb
}

func (cb *CommandBuilder) Arg(args ...string) *CommandBuilder {
	cb.args = append(cb.args, args...)
	return cb
}

func (cb *CommandBuilder) ArgIf(cond bool, args ...string) *CommandBuilder {
	if cond {
		cb.args = append(cb.args, args...)
	}
	return cb
}

func (cb *CommandBuilder) buildCommand() (string, []string) {
	args := []string{}
	if cb.repo != nil {
		args = append(args, "--git-dir", fmt.Sprintf("%s/.git", cb.repo.Path))
		args = append(args, "--work-tree", cb.repo.Path)
	}
	args = append(args, cb.args...)
	return cb.entryPoint, args
}

func (cb *CommandBuilder) RunWithOutput() (string, error) {
	ep, args := cb.buildCommand()

	bo, err, _ := cb.exec(ep, args...).Output()
	if err != nil {
		return "", err
	}

	op := strings.Trim(string(bo), " \n\t")
	return op, nil
}

func (cb *CommandBuilder) RunWithExitCode() int {
	ep, args := cb.buildCommand()
	_, _, ec := cb.exec(ep, args...).Output()
	return ec
}

func (cb *CommandBuilder) Run() error {
	_, err := cb.RunWithOutput()
	return err
}

func (cb *CommandBuilder) String() string {
	ep, arg := cb.buildCommand()
	return fmt.Sprintf("%s %s", ep, strings.Join(arg, " "))
}
