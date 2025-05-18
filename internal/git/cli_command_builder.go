package git

import (
	"fmt"
	"os/exec"
	"strings"
)

type CommandBuilder struct {
	repo       *Repo
	entryPoint string
	args       []string
}

func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{
		entryPoint: "git", // todo: change this to dynamic
		args:       []string{},
	}
}

func (cb *CommandBuilder) SetRepo(r Repo) *CommandBuilder {
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

func (cb *CommandBuilder) run() ([]byte, int, error) {
	ep, args := cb.buildCommand()
	cmd := exec.Command(ep, args...)

	o, e := cmd.Output()
	return o, cmd.ProcessState.ExitCode(), e
}

func (cb *CommandBuilder) RunWithOutput() (string, error) {
	bo, _, err := cb.run()
	if err != nil {
		return "", err
	}

	op := strings.Trim(string(bo), " \n\t")
	return op, nil
}

func (cb *CommandBuilder) RunWithExitCode() int {
	_, ec, _ := cb.run()
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
