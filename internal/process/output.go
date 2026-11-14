package process

import "sync"

// DefaultOutputCapacity is the default number of lines retained by a
// RingOutput. A chatty dev server emits ~360k lines/hour; 10k lines is
// plenty of scrollback for a terminal panel while bounding memory.
const DefaultOutputCapacity = 10000

// Output is an abstraction over the buffer that captures stdout/stderr
// from a running Process or Task.
//
// Implementations may keep all lines in memory or perform log rotation,
// retaining only a recent slice of data in memory and spilling the rest
// to disk. Callers should never assume that all historical lines are
// available; use Len to check how many lines the buffer currently holds.
type Output interface {
	// Append adds a line to the buffer.
	Append(line string)
	// Lines returns a defensive copy of the currently buffered lines.
	// The returned slice is safe to read and modify without affecting the
	// buffer or racing with concurrent appends.
	Lines() []string
	// LinesRange returns a defensive copy of the lines in the half-open
	// range [start, end), clamped to the buffer bounds. Use it to read a
	// visible window without copying the whole buffer.
	LinesRange(start, end int) []string
	// Len returns the number of lines currently held in the buffer.
	Len() int
	// Clear removes all buffered content.
	Clear()
}

// MemoryOutput is a simple in-memory implementation of Output. It stores
// all lines in a Go slice with no rotation.
type MemoryOutput struct {
	mu    sync.Mutex
	lines []string
}

func NewMemoryOutput() *MemoryOutput {
	return &MemoryOutput{}
}

func (m *MemoryOutput) Append(line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines = append(m.lines, line)
}

func (m *MemoryOutput) Lines() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.lines))
	copy(out, m.lines)
	return out
}

func (m *MemoryOutput) LinesRange(start, end int) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	start, end = clampRange(start, end, len(m.lines))
	out := make([]string, end-start)
	copy(out, m.lines[start:end])
	return out
}

func (m *MemoryOutput) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.lines)
}

func (m *MemoryOutput) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines = nil
}

// Ensure MemoryOutput implements Output.
var _ Output = (*MemoryOutput)(nil)

// RingOutput is a fixed-capacity implementation of Output. When the
// buffer is full, the oldest lines are dropped to make room for new
// ones. It has O(1) insertion with zero steady-state allocation and is
// safe for concurrent use.
type RingOutput struct {
	mu    sync.Mutex
	buf   []string
	head  int // index of the oldest line
	count int
}

// NewRingOutput returns a RingOutput retaining up to capacity lines.
// If capacity <= 0 DefaultOutputCapacity is used.
func NewRingOutput(capacity int) *RingOutput {
	if capacity <= 0 {
		capacity = DefaultOutputCapacity
	}
	return &RingOutput{buf: make([]string, capacity)}
}

func (r *RingOutput) Append(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count < len(r.buf) {
		r.buf[(r.head+r.count)%len(r.buf)] = line
		r.count++
		return
	}
	r.buf[r.head] = line
	r.head = (r.head + 1) % len(r.buf)
}

func (r *RingOutput) Lines() []string {
	return r.LinesRange(0, r.Len())
}

func (r *RingOutput) LinesRange(start, end int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	start, end = clampRange(start, end, r.count)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, r.buf[(r.head+i)%len(r.buf)])
	}
	return out
}

func (r *RingOutput) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func (r *RingOutput) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head = 0
	r.count = 0
}

// Ensure RingOutput implements Output.
var _ Output = (*RingOutput)(nil)

// clampRange clamps a half-open [start, end) range to [0, n].
func clampRange(start, end, n int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start > end {
		start = end
	}
	return start, end
}
