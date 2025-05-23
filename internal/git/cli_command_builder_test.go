package git

import (
	"reflect"
	"testing"
)

func NewTestCommandBuilder() *CommandBuilder {
	return &CommandBuilder{
		entryPoint: "git",
		args:       []string{},
	}
}

func TestArgs(t *testing.T) {
	args1 := []string{"rebase"}
	args2 := []string{"--abort"}
	want := []string{"rebase", "--abort"}
	cb := NewTestCommandBuilder()

	got := cb.Arg(args1...).Arg(args2...).args

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildCommand(t *testing.T) {
	args := [][]string{
		{"rebase"},
		{"--autosquash", "origin/master"},
	}
	wantArgs := []string{"rebase", "--autosquash", "origin/master"}
	cb := NewCommandBuilder()

	for _, a := range args {
		cb.Arg(a...)
	}

	gotEp, gotArgs := cb.buildCommand()
	if gotEp != cb.entryPoint {
		t.Errorf("[entrypoint] - got %v, want %v", gotEp, cb.entryPoint)
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Errorf("[args] - got %v, want %v", gotArgs, wantArgs)
	}
}
