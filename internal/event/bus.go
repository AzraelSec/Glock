package event

// FilterFunc returns true if the event should be delivered to a subscriber.
type FilterFunc func(e Event) bool

// Subscription represents an active event subscription.
type Subscription interface {
	// Events returns the receive-only channel that delivers matching events.
	Events() <-chan Event
	// Cancel removes the subscriber and closes the channel.
	Cancel()
}

// Publisher pushes events into the bus.
//
// Managers receive this interface — they can publish but cannot subscribe.
type Publisher interface {
	Publish(e Event)
}

// Bus is the full bidirectional event bus.
//
// Only the concrete implementation and the Project orchestrator need this
// interface; managers and consumers work with Publisher or Subscription
type Bus interface {
	Publisher
	// Subscribe registers a new subscriber. The filter decides which events
	// are delivered to the returned Subscription's channel.
	Subscribe(filter FilterFunc) Subscription
	// Close shuts down the dispatcher and cleans up all subscriptions.
	Close()
}
