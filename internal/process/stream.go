package process

// OutputLine carries a bounded fragment. Partial means more fragments belong
// to the same line. Truncated reports a pipe closed before EOF.
type OutputLine struct {
	Kind, Name, Stream, Line    string
	Partial, Newline, Truncated bool
}
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
func (o *streamingOutput) AppendStream(stream, line string, partial, newline bool) {
	if out, ok := o.Output.(interface {
		AppendStream(string, string, bool, bool)
	}); ok {
		out.AppendStream(stream, line, partial, newline)
	} else {
		o.Output.Append(line)
	}
	o.sink(OutputLine{Kind: o.kind, Name: o.name, Stream: stream, Line: line, Partial: partial, Newline: newline})
}

func (o *streamingOutput) ReportTruncation() {
	if reporter, ok := o.Output.(interface{ ReportTruncation() }); ok {
		reporter.ReportTruncation()
	}
	o.sink(OutputLine{Kind: o.kind, Name: o.name, Truncated: true})
}

func (o *streamingOutput) Evicted() uint64 {
	if counter, ok := o.Output.(interface{ Evicted() uint64 }); ok {
		return counter.Evicted()
	}
	return 0
}

func (o *streamingOutput) Fragments() []OutputFragment {
	if reader, ok := o.Output.(interface{ Fragments() []OutputFragment }); ok {
		return reader.Fragments()
	}
	return nil
}

func (o *streamingOutput) Truncated() bool {
	if reader, ok := o.Output.(interface{ Truncated() bool }); ok {
		return reader.Truncated()
	}
	return false
}

func (o *streamingOutput) RetainedOutput() ([]OutputFragment, uint64, bool) {
	if reader, ok := o.Output.(interface {
		RetainedOutput() ([]OutputFragment, uint64, bool)
	}); ok {
		return reader.RetainedOutput()
	}
	return nil, 0, false
}
