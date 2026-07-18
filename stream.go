package liveview

// StreamOp is a single stream mutation queued for the client. Streams let a
// view manage a large, append-only or windowed collection (a chat log, a table)
// without the server holding the whole list in assigns: the server emits only
// the delta operations, and the client applies them to the DOM container marked
// phx-update="stream".
//
// The Action is one of "insert", "delete", or "reset". For an insert, DOM is
// the stable per-item id, HTML is the rendered markup, and At is the insert
// position (-1 appends, 0 prepends). For a delete, only DOM is meaningful. A
// reset clears the container before applying any inserts in the same batch.
type StreamOp struct {
	// Action is "insert", "delete", or "reset".
	Action string `json:"action"`
	// DOM is the stable element id the operation targets (empty for reset).
	DOM string `json:"id,omitempty"`
	// HTML is the rendered item markup for an insert.
	HTML string `json:"html,omitempty"`
	// At is the insert index: -1 appends, 0 prepends, n inserts before n.
	At int `json:"at,omitempty"`
}

// Stream accumulates pending [StreamOp] values for one named collection since
// the last render. The runtime drains the queue after each render and ships the
// operations in the diff; the server never retains the item list itself.
type Stream struct {
	// Name is the stream's identifier, used as the DOM container id prefix.
	Name string
	ops  []StreamOp
}

// Insert queues an append (at == -1), prepend (at == 0), or positional insert
// of an item with the given stable dom id and rendered HTML.
func (s *Stream) Insert(dom string, at int, r *Rendered) {
	s.ops = append(s.ops, StreamOp{Action: "insert", DOM: dom, HTML: r.HTML(), At: at})
}

// Append queues an item at the end of the stream container.
func (s *Stream) Append(dom string, r *Rendered) { s.Insert(dom, -1, r) }

// Prepend queues an item at the start of the stream container.
func (s *Stream) Prepend(dom string, r *Rendered) { s.Insert(dom, 0, r) }

// Delete queues removal of the item with the given dom id.
func (s *Stream) Delete(dom string) {
	s.ops = append(s.ops, StreamOp{Action: "delete", DOM: dom})
}

// Reset queues clearing the whole stream container. Inserts queued after a
// reset in the same batch repopulate it.
func (s *Stream) Reset() {
	s.ops = append(s.ops, StreamOp{Action: "reset"})
}

// drain returns the queued operations and clears the queue.
func (s *Stream) drain() []StreamOp {
	ops := s.ops
	s.ops = nil
	return ops
}

// Pending returns the number of queued operations, primarily for tests.
func (s *Stream) Pending() int { return len(s.ops) }
