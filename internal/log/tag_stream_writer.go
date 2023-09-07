package log

import (
	"bytes"
	"fmt"
	"io"
)

type TagStreamWriter struct {
	tag            string
	output         io.Writer
	buffer         bytes.Buffer
	newLineTouched bool
}

func NewTagStreamWriter(tag string, out io.Writer) *TagStreamWriter {
	return &TagStreamWriter{
		tag:    tag,
		output: out,
		buffer: bytes.Buffer{},
	}
}

func (w *TagStreamWriter) Write(p []byte) (int, error) {
	w.buffer.Reset()

	for _, b := range p {
		if !w.newLineTouched {
			w.buffer.WriteString(fmt.Sprintf("[%s] ", w.tag))
			w.newLineTouched = true
		}

		w.buffer.WriteByte(b)

		if b == '\n' {
			w.newLineTouched = false
		}
	}

	n, err := w.output.Write(w.buffer.Bytes())

	if err != nil {
		if n > len(p) {
			n = len(p)
		}
		return n, err
	}

	return len(p), nil
}
