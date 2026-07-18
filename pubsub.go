package liveview

import "sync"

// PubSub is a tiny in-process publish/subscribe hub. Subscribers register an
// inbox channel against a topic; a broadcast delivers a message to every
// inbox currently subscribed to that topic. It is the mechanism behind
// server-initiated updates: a [Session] subscribes its inbox to topics of
// interest, and any code holding the PubSub can broadcast an event that the
// session then processes through HandleInfo.
//
// PubSub is safe for concurrent use. Delivery is non-blocking: if a
// subscriber's inbox is full the message is dropped for that subscriber rather
// than stalling the broadcaster, matching the "at most once, best effort"
// semantics appropriate for UI refresh signals.
type PubSub struct {
	mu   sync.RWMutex
	subs map[string]map[chan any]struct{}
}

// NewPubSub returns an empty hub ready for subscriptions.
func NewPubSub() *PubSub {
	return &PubSub{subs: make(map[string]map[chan any]struct{})}
}

// Subscribe registers inbox to receive messages broadcast on topic. Subscribing
// the same inbox to the same topic twice is a no-op.
func (p *PubSub) Subscribe(topic string, inbox chan any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	set := p.subs[topic]
	if set == nil {
		set = make(map[chan any]struct{})
		p.subs[topic] = set
	}
	set[inbox] = struct{}{}
}

// Unsubscribe removes inbox from topic. It is safe to call for a subscription
// that does not exist.
func (p *PubSub) Unsubscribe(topic string, inbox chan any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if set := p.subs[topic]; set != nil {
		delete(set, inbox)
		if len(set) == 0 {
			delete(p.subs, topic)
		}
	}
}

// UnsubscribeAll removes inbox from every topic it is subscribed to. The
// runtime calls this when a session ends so stale channels are not retained.
func (p *PubSub) UnsubscribeAll(inbox chan any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for topic, set := range p.subs {
		delete(set, inbox)
		if len(set) == 0 {
			delete(p.subs, topic)
		}
	}
}

// Broadcast delivers msg to every inbox subscribed to topic. Delivery is
// best-effort and non-blocking. It returns the number of subscribers the
// message was successfully queued to.
func (p *PubSub) Broadcast(topic string, msg any) int {
	p.mu.RLock()
	set := p.subs[topic]
	inboxes := make([]chan any, 0, len(set))
	for ch := range set {
		inboxes = append(inboxes, ch)
	}
	p.mu.RUnlock()

	delivered := 0
	for _, ch := range inboxes {
		select {
		case ch <- msg:
			delivered++
		default:
			// Drop rather than block a slow or gone subscriber.
		}
	}
	return delivered
}

// SubscriberCount returns the number of inboxes currently subscribed to topic,
// primarily for tests and introspection.
func (p *PubSub) SubscriberCount(topic string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.subs[topic])
}
