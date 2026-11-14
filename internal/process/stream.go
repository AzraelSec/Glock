package process

// OutputLine is a lossless line notification, separate from diagnostic events
// and replaceable snapshots. Ordering is preserved within each OS pipe.
type OutputLine struct{ Kind, Name, Stream, Line string }
type OutputSink func(OutputLine)

type streamingOutput struct {
	Output
	kind, name string
	sink       OutputSink
}

func WithStream(output Output, kind, name string, sink OutputSink) Output {
	if sink == nil {
		return output
	}
	if previous, ok := output.(*streamingOutput); ok {
		output = previous.Output
	}
	return &streamingOutput{Output: output, kind: kind, name: name, sink: sink}
}
func (o *streamingOutput) AppendStream(stream, line string) {
	o.Output.Append(line)
	o.sink(OutputLine{Kind: o.kind, Name: o.name, Stream: stream, Line: line})
}
