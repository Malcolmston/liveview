package liveview

import (
	"sync"
)

// Session is the per-connection runtime for one mounted [View]. It owns the
// [Socket], remembers the last render, and drives the state -> render -> diff
// cycle: Mount produces the first render, then each Event re-renders and returns
// only the diff against the previous render.
//
// A Session is safe for concurrent use; events on the same session are
// serialized by an internal mutex so assigns and the cached render stay
// consistent.
type Session struct {
	mu     sync.Mutex
	view   View
	socket *Socket
	last   *Rendered
	id     string
}

// NewSession creates a session bound to view with a fresh, empty socket. Call
// [Session.Mount] before dispatching events.
func NewSession(view View) *Session {
	return &Session{view: view, socket: NewSocket()}
}

// ID returns the session identifier (set by the runtime/handler that created
// it), or "" if unset.
func (s *Session) ID() string { return s.id }

// Socket exposes the session's socket, primarily for tests and introspection.
func (s *Session) Socket() *Socket { return s.socket }

// Mount runs the view's Mount with the given params, performs the initial
// render, caches it, and returns it. It is an error to call Mount more than
// once.
func (s *Session) Mount(params map[string]any) (*Rendered, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if params == nil {
		params = map[string]any{}
	}
	if err := s.view.Mount(params, s.socket); err != nil {
		return nil, err
	}
	s.socket.ResetChanges()
	s.last = s.view.Render(s.socket.Assigns())
	return s.last, nil
}

// Render returns the most recent render without dispatching an event.
func (s *Session) Render() *Rendered {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// Event dispatches a client event: it runs HandleEvent, re-renders, computes the
// minimal [Diff] against the previous render, caches the new render, and returns
// the diff. An empty diff means the event changed nothing observable.
func (s *Session) Event(event string, payload map[string]any) (Diff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if payload == nil {
		payload = map[string]any{}
	}
	if err := s.view.HandleEvent(event, payload, s.socket); err != nil {
		return nil, err
	}
	s.socket.ResetChanges()
	next := s.view.Render(s.socket.Assigns())
	diff := DiffRendered(s.last, next)
	s.last = next
	return diff, nil
}
