package event

import (
	"testing"
	"time"
)

func TestBus_PublishAndReceive(t *testing.T) {
	b := NewBus(64)
	defer b.Close()

	sub := b.Subscribe(func(e Event) bool { return true })
	defer sub.Cancel()

	b.Publish(Event{Name: ComponentStarted, Component: "api"})

	select {
	case e := <-sub.Events():
		if e.Name != ComponentStarted {
			t.Fatalf("expected %s, got %s", ComponentStarted, e.Name)
		}
		if e.Component != "api" {
			t.Fatalf("expected component api, got %s", e.Component)
		}
		if e.Timestamp.IsZero() {
			t.Fatal("expected non-zero timestamp")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBus_Filter(t *testing.T) {
	b := NewBus(64)
	defer b.Close()

	allSub := b.Subscribe(func(e Event) bool { return true })
	defer allSub.Cancel()

	apiSub := b.Subscribe(func(e Event) bool { return e.Component == "api" })
	defer apiSub.Cancel()

	b.Publish(Event{Name: ComponentStarted, Component: "api"})
	b.Publish(Event{Name: ComponentStarted, Component: "worker"})

	// allSub should receive both
	for i := 0; i < 2; i++ {
		select {
		case <-allSub.Events():
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for allSub event %d", i)
		}
	}

	// apiSub should receive only the api event
	select {
	case e := <-apiSub.Events():
		if e.Component != "api" {
			t.Fatalf("expected api, got %s", e.Component)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for apiSub event")
	}

	// apiSub should have no more events
	select {
	case <-apiSub.Events():
		t.Fatal("apiSub received unexpected second event")
	case <-time.After(100 * time.Millisecond):
		// expected — no extra event
	}
}

func TestBus_NilFilter(t *testing.T) {
	b := NewBus(64)
	defer b.Close()

	// nil filter should default to accept all
	sub := b.Subscribe(nil)
	defer sub.Cancel()

	b.Publish(Event{Name: TaskStarted})

	select {
	case e := <-sub.Events():
		if e.Name != TaskStarted {
			t.Fatalf("expected %s, got %s", TaskStarted, e.Name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBus_Cancel(t *testing.T) {
	b := NewBus(64)
	defer b.Close()

	sub := b.Subscribe(func(e Event) bool { return true })
	sub.Cancel()

	b.Publish(Event{Name: ComponentStopped})

	// Cancel closes the channel, so reading from it should return the
	// zero value immediately (or panic on closed). We expect the
	// channel to be closed.
	select {
	case _, ok := <-sub.Events():
		if ok {
			t.Fatal("expected closed channel after cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for closed channel")
	}
}

func TestBus_DropWhenSubscriberBufferFull(t *testing.T) {
	b := NewBus(64)
	defer b.Close()

	// Create a subscriber with a tiny buffer (2 slots) and never read from it.
	b.(*bus).mu.Lock()
	sub := &subscription{
		bus:    b.(*bus),
		id:     b.(*bus).nextID,
		filter: func(e Event) bool { return true },
		ch:     make(chan Event, 2),
	}
	b.(*bus).nextID++
	b.(*bus).subs[sub.id] = sub
	b.(*bus).mu.Unlock()

	// Publish three events — only the first two fit in the 2-slot buffer.
	b.Publish(Event{Name: ComponentStarted})
	b.Publish(Event{Name: ComponentStopping})
	b.Publish(Event{Name: ComponentStopped})

	// Give the dispatcher time to deliver.
	time.Sleep(50 * time.Millisecond)

	// We should receive exactly the first two events.
	for _, want := range []string{ComponentStarted, ComponentStopping} {
		select {
		case e := <-sub.Events():
			if e.Name != want {
				t.Fatalf("expected %s, got %s", want, e.Name)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s", want)
		}
	}

	// There should be no more events — the third was dropped.
	select {
	case <-sub.Events():
		t.Fatal("expected dropped event, got one")
	case <-time.After(100 * time.Millisecond):
		// expected — third event was dropped
	}

	sub.Cancel()
}

func TestBus_TimestampStamping(t *testing.T) {
	b := NewBus(64)
	defer b.Close()

	sub := b.Subscribe(func(e Event) bool { return true })
	defer sub.Cancel()

	before := time.Now()
	b.Publish(Event{Name: ApplicationStarted})
	after := time.Now()

	select {
	case e := <-sub.Events():
		if e.Timestamp.Before(before) || e.Timestamp.After(after) {
			t.Fatalf("timestamp %v not in range [%v, %v]", e.Timestamp, before, after)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBus_Close(t *testing.T) {
	b := NewBus(64)

	sub := b.Subscribe(func(e Event) bool { return true })

	b.Close()

	// Wait for the dispatcher to finish
	time.Sleep(100 * time.Millisecond)

	// The inbound channel should be closed, causing the dispatcher to exit
	// and close all subscriber channels.
	select {
	case _, ok := <-sub.Events():
		if ok {
			t.Fatal("expected closed channel after Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for closed channel")
	}
}

// TestBus_CancelDuringDeliver hammers Cancel concurrently with Publish
// to exercise the deliver-vs-Cancel serialization (previously a
// send-on-closed-channel panic). Must be run with -race.
func TestBus_CancelDuringDeliver(t *testing.T) {
	b := NewBus(64)
	defer b.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			b.Publish(Event{Name: ComponentStarted, Component: "api"})
		}
	}()

	for i := 0; i < 100; i++ {
		sub := b.Subscribe(func(e Event) bool { return true })
		// Drain a couple of events then cancel mid-stream.
		select {
		case <-sub.Events():
		case <-time.After(10 * time.Millisecond):
		}
		sub.Cancel()
	}
	<-done
}

// TestBus_PublishAfterClose verifies publishing on a closed bus is a
// silent no-op (previously relied on recover from a channel panic).
func TestBus_PublishAfterClose(t *testing.T) {
	b := NewBus(4)
	b.Close()

	// Must not panic.
	for i := 0; i < 100; i++ {
		b.Publish(Event{Name: ComponentStarted})
	}
}

// TestBus_SubscribeAfterClose verifies a subscription created after
// Close has an already-closed channel.
func TestBus_SubscribeAfterClose(t *testing.T) {
	b := NewBus(4)
	b.Close()

	sub := b.Subscribe(nil)
	select {
	case _, ok := <-sub.Events():
		if ok {
			t.Fatal("expected closed channel for post-Close subscription")
		}
	default:
		t.Fatal("expected channel to be closed immediately")
	}
}

func TestBus_PayloadTypeAssertion(t *testing.T) {
	b := NewBus(64)
	defer b.Close()

	sub := b.Subscribe(func(e Event) bool { return e.Name == ComponentStarted })
	defer sub.Cancel()

	b.Publish(Event{
		Name:      ComponentStarted,
		Component: "api",
		Payload:   ComponentStartedPayload{PID: 12345},
	})

	select {
	case e := <-sub.Events():
		payload, ok := e.Payload.(ComponentStartedPayload)
		if !ok {
			t.Fatalf("expected ComponentStartedPayload, got %T", e.Payload)
		}
		if payload.PID != 12345 {
			t.Fatalf("expected PID 12345, got %d", payload.PID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}
