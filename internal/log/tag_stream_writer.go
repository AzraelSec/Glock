package log

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"io"
	"strings"

	"github.com/fatih/color"
)

type TagStreamWriter struct {
	tag            string
	output         io.Writer
	color          *color.Color
	buffer         bytes.Buffer
	nl bool
}

var fgColors = []color.Attribute{
	color.FgRed,
	color.FgGreen,
	color.FgYellow,
	color.FgBlue,
	color.FgMagenta,
	color.FgCyan,
}

func colorByTag(tag string) *color.Color{
	hash := md5.Sum([]byte(strings.ToLower(tag)))
	index := int(binary.BigEndian.Uint32(hash[:])) % len(fgColors)

	if index < 0 {
		index = -index
	}
	return color.New(fgColors[index])
}

func NewTagStreamWriter(tag string, out io.Writer) *TagStreamWriter {
	return &TagStreamWriter{
		tag:    tag,
		output: out,
		color:  colorByTag(tag),
		buffer: bytes.Buffer{},
	}
}

func (w *TagStreamWriter) Write(p []byte) (int, error) {
	w.buffer.Reset()

	for _, b := range p {
		if !w.nl {
			w.buffer.WriteString(w.color.Sprintf("[%s] ", w.tag))
			w.nl = true
		}

		w.buffer.WriteByte(b)

		if b == '\n' {
			w.nl = false
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
