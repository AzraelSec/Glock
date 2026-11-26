package commands

import (
	"bytes"
	"testing"

	"github.com/fatih/color"
)

func TestRedWriterPreservesColoredMultilineOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	previous := color.NoColor
	color.NoColor = false
	t.Cleanup(func() { color.NoColor = previous })

	var stderr bytes.Buffer
	message := []byte("first line\nsecond line\n")
	n, err := (RedWriter{output: &stderr}).Write(message)
	if err != nil || n != len(message) {
		t.Fatalf("Write returned (%d, %v)", n, err)
	}
	if got, want := stderr.String(), "\x1b[91m"+string(message)+"\x1b[0m"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
