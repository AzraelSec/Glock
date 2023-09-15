package log

import (
	"bytes"
	"testing"
)

func TestTagStreamWriter(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			"first_line\nsecond_line",
			"[test] first_line\n[test] second_line",
		},
		{
			"",
			"",
		},
		{
			"one_line",
			"[test] one_line",
		},
		{
			"first_line\n\nthird_line",
			"[test] first_line\n[test] \n[test] third_line",
		},
	}

	for _, tt := range tests {
		var output bytes.Buffer
		tsw := NewTagStreamWriter("test", &output)
		tsw.Write([]byte(tt.input))

		if output.String() != tt.want {
			t.Errorf("unexpected output. got=%v, want=%v", output.String(), tt.want)
		}
	}

}
