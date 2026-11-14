package event

import (
	"sync"
	"time"
)

// DefaultBufferSize is the default capacity of the bus inbound channel.
const DefaultBufferSize = 256

// DefaultSubscriberBufferSize is the default capacity of each subscriber
// channel.
const DefaultSubscriberBufferSize = 64

// bus is the concrete asynchronous event-bus implementation.
type bus struct {
	mu     sync.RWMutex
	subs   map[int]*subscription
	nextID int
	in     chan Event
	closed bool
}

// subscription is a concrete Subscription implementation.
type subscription struct {
	bus    *bus
	id     int
	filter FilterFunc
	ch     chan Event
	closed bool
	mu     sync.Mutex
}

// NewBus creates an asynchronous event bus with the given inbound buffer
// size. If size <= 0 DefaultBufferSize is used.
func NewBus(size int) Bus {
	if size <= 0 {
		size = DefaultBufferSize
	}
	b := &bus{
		subs:   make(map[int]*subscription),
		nextID: 1,
		in:     make(chan Event, size),
	}
	go b.dispatcher()
	return b
}

// Publish stamps the event with the current time and enqueues it.
// If the inbound buffer is full, or the bus has been closed, the event
// is dropped silently.
//
// Publish holds the bus read-lock so that a concurrent Close either
// runs entirely before the send (event is drained) or after (event is
// dropped via the closed check). A send on a closed channel is
// therefore impossible.
func (b *bus) Publish(e Event) {
	e.Timestamp = time.Now()
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	select {
	case b.in <- e:
	default:
		// inbound buffer full — drop event to avoid blocking the publisher
	}
}

// Subscribe registers a new subscriber. If the bus is already closed,
// the returned subscription's channel is closed immediately.
func (b *bus) Subscribe(filter FilterFunc) Subscription {
	if filter == nil {
		filter = func(e Event) bool { return true }
	}

	b.mu.Lock()
	id := b.nextID
	b.nextID++
	sub := &subscription{
		bus:    b,
		id:     id,
		filter: filter,
		ch:     make(chan Event, DefaultSubscriberBufferSize),
	}
	if b.closed {
		sub.closed = true
		close(sub.ch)
	} else {
		b.subs[id] = sub
	}
	b.mu.Unlock()

	return sub
}

// Close shuts down the dispatcher and cleans up all subscriptions.
// It is idempotent. Closing the inbound channel (under the write lock,
// so no Publish can be in flight) causes the dispatcher to drain any
// buffered events and then close every subscriber channel.
func (b *bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	close(b.in)
	b.mu.Unlock()
}

// ------------------------------------------------------------------
// subscription methods
// ------------------------------------------------------------------

func (s *subscription) Events() <-chan Event {
	return s.ch
}

func (s *subscription) Cancel() {
	s.bus.remove(s.id)
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
	s.mu.Unlock()
}

// ------------------------------------------------------------------
// internal helpers
// ------------------------------------------------------------------

func (b *bus) remove(id int) {
	b.mu.Lock()
	delete(b.subs, id)
	b.mu.Unlock()
}

// dispatcher ranges the inbound channel. Close closes b.in under the
// write lock, so range drains all buffered events before exiting.
func (b *bus) dispatcher() {
	for e := range b.in {
		b.deliver(e)
	}
	b.drainAndClose()
}

func (b *bus) drainAndClose() {
	b.mu.Lock()
	for _, sub := range b.subs {
		sub.mu.Lock()
		if !sub.closed {
			sub.closed = true
			close(sub.ch)
		}
		sub.mu.Unlock()
	}
	b.subs = make(map[int]*subscription)
	b.mu.Unlock()
}

// deliver snapshots matching subscribers, then sends to each while
// holding the subscription's own mutex. This serializes the send with
// Cancel (which closes the channel under the same mutex), so a send on
// a closed channel is impossible.
func (b *bus) deliver(e Event) {
	b.mu.RLock()
	var matching []*subscription
	for _, sub := range b.subs {
		if sub.filter(e) {
			matching = append(matching, sub)
		}
	}
	b.mu.RUnlock()

	for _, sub := range matching {
		sub.mu.Lock()
		if !sub.closed {
			select {
			case sub.ch <- e:
			default:
				// subscriber buffer full — drop event for this subscriber
			}
		}
		sub.mu.Unlock()
	}
}
