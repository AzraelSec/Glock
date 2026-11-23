package process

import (
	"sync"
	"testing"
)

func TestMemoryOutput_ConcurrentAppendAndLines(t *testing.T) {
	mo := NewMemoryOutput()

	var wg sync.WaitGroup

	// Writer goroutine: append 100 lines
	wg.Go(func() {
		for range 100 {
			mo.Append("line")
		}
	})

	// Reader goroutine: read Lines() 100 times
	wg.Go(func() {
		for range 100 {
			_ = mo.Lines()
		}
	})

	wg.Wait()

	lines := mo.Lines()
	if len(lines) != 100 {
		t.Errorf("expected 100 lines, got %d", len(lines))
	}
}

func TestMemoryOutput_ConcurrentAppendAndLen(t *testing.T) {
	mo := NewMemoryOutput()

	var wg sync.WaitGroup

	wg.Go(func() {
		for range 100 {
			mo.Append("line")
		}
	})

	wg.Go(func() {
		for range 100 {
			_ = mo.Len()
		}
	})

	wg.Wait()

	if mo.Len() != 100 {
		t.Errorf("expected len 100, got %d", mo.Len())
	}
}

func TestMemoryOutput_LinesReturnsCopy(t *testing.T) {
	mo := NewMemoryOutput()
	mo.Append("first")
	mo.Append("second")

	snapshot := mo.Lines()
	if len(snapshot) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(snapshot))
	}

	// Mutating the returned slice should not affect the internal state
	snapshot[0] = "tampered"

	lines := mo.Lines()
	if lines[0] != "first" {
		t.Errorf("internal state was mutated: got %q, want %q", lines[0], "first")
	}
}

func TestMemoryOutput_Clear(t *testing.T) {
	mo := NewMemoryOutput()
	mo.Append("a")
	mo.Append("b")
	if mo.Len() != 2 {
		t.Fatalf("expected len 2, got %d", mo.Len())
	}

	mo.Clear()
	if mo.Len() != 0 {
		t.Errorf("expected len 0 after Clear, got %d", mo.Len())
	}
	if len(mo.Lines()) != 0 {
		t.Errorf("expected empty Lines after Clear, got %d lines", len(mo.Lines()))
	}
}

func TestRingOutput_Rotation(t *testing.T) {
	r := NewRingOutput(3)
	for _, l := range []string{"a", "b", "c", "d", "e"} {
		r.Append(l)
	}

	if r.Len() != 3 {
		t.Fatalf("expected len 3, got %d", r.Len())
	}
	lines := r.Lines()
	want := []string{"c", "d", "e"}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("lines[%d]: wanted %q, got %q", i, w, lines[i])
		}
	}
}

func TestRingOutput_LinesRange(t *testing.T) {
	r := NewRingOutput(5)
	for _, l := range []string{"a", "b", "c", "d"} {
		r.Append(l)
	}

	got := r.LinesRange(1, 3)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Errorf("wanted [b c], got %v", got)
	}

	// Out-of-range requests are clamped.
	if got := r.LinesRange(-5, 100); len(got) != 4 {
		t.Errorf("wanted clamped full range (4), got %d", len(got))
	}
	if got := r.LinesRange(3, 1); len(got) != 0 {
		t.Errorf("wanted empty range, got %d", len(got))
	}
}

func TestRingOutput_ClearAndReuse(t *testing.T) {
	r := NewRingOutput(2)
	r.Append("x")
	r.Append("y")
	r.Clear()

	if r.Len() != 0 {
		t.Fatalf("expected len 0 after Clear, got %d", r.Len())
	}
	r.Append("z")
	if got := r.Lines(); len(got) != 1 || got[0] != "z" {
		t.Errorf("wanted [z] after reuse, got %v", got)
	}
}

func TestRingOutput_ConcurrentAppendAndRead(t *testing.T) {
	r := NewRingOutput(50)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 500 {
			r.Append("line")
		}
	}()
	go func() {
		defer wg.Done()
		for range 500 {
			_ = r.LinesRange(0, 10)
			_ = r.Lines()
		}
	}()
	wg.Wait()

	if r.Len() != 50 {
		t.Fatalf("expected len 50 (capped), got %d", r.Len())
	}
}

func TestRingOutputTracksFragmentsAndEviction(t *testing.T) {
	r := NewRingOutput(2)
	r.AppendStream("stdout", "first", true, false)
	r.AppendStream("stdout", "last", false, true)
	r.AppendStream("stderr", "error", false, false)
	f := r.Fragments()
	if len(f) != 2 || f[0].Text != "last" || !f[0].Newline || f[1].Stream != "stderr" || f[1].Newline || r.Evicted() != 1 {
		t.Fatalf("fragments=%+v evicted=%d", f, r.Evicted())
	}
	r.Clear()
	if r.Evicted() != 0 || len(r.Fragments()) != 0 {
		t.Fatal("clear did not reset output history")
	}
}
