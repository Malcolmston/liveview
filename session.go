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
	uri    string
	pubsub *PubSub
}

// NewSession creates a session bound to view with a fresh, empty socket, its own
// server-push mailbox, and a component manager. Call [Session.Mount] before
// dispatching events.
func NewSession(view View) *Session {
	s := &Session{view: view, socket: NewSocket()}
	s.socket.inbox = make(chan any, 64)
	s.socket.comps = NewComponentManager()
	return s
}

// ID returns the session identifier (set by the runtime/handler that created
// it), or "" if unset.
func (s *Session) ID() string { return s.id }

// Socket exposes the session's socket, primarily for tests and introspection.
func (s *Session) Socket() *Socket { return s.socket }

// Inbox returns the session's server-push mailbox. The transport loop selects on
// it to deliver [Socket.Send] and [PubSub] messages into [Session.Info].
func (s *Session) Inbox() <-chan any { return s.socket.inbox }

// AttachPubSub wires a [PubSub] hub into the session so views can call
// [Socket.Subscribe]. The transport typically shares one hub across sessions.
func (s *Session) AttachPubSub(p *PubSub) {
	s.mu.Lock()
	s.pubsub = p
	s.socket.pubsub = p
	s.mu.Unlock()
}

// SetConnected marks whether a live transport is attached, so views can gate
// work on [Socket.Connected].
func (s *Session) SetConnected(v bool) {
	s.mu.Lock()
	s.socket.connected = v
	s.mu.Unlock()
}

// Mount runs the view's Mount with the given params, invokes HandleParams if the
// view implements [ParamsHandler], performs the initial render, caches it, and
// returns it.
func (s *Session) Mount(params map[string]any) (*Rendered, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if params == nil {
		params = map[string]any{}
	}
	if err := s.view.Mount(params, s.socket); err != nil {
		return nil, err
	}
	if ph, ok := s.view.(ParamsHandler); ok {
		if err := ph.HandleParams(params, s.uri, s.socket); err != nil {
			return nil, err
		}
	}
	s.socket.ResetChanges()
	s.last = s.view.Render(s.socket.Assigns())
	return s.last, nil
}

// SetURI records the connection's current request URI, used as the base for
// [ParamsHandler.HandleParams] and live navigation.
func (s *Session) SetURI(uri string) {
	s.mu.Lock()
	s.uri = uri
	s.mu.Unlock()
}

// InitialDiff returns the full diff for the current render plus any side
// channels (streams, push events, navigation) set during Mount. The WebSocket
// transport ships it as the first frame so a fresh client can build the
// document.
func (s *Session) InitialDiff() Diff {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := FullDiff(s.last)
	if c := s.socket.comps.fullComponents(); c != nil {
		d[componentsKey] = c
	}
	s.attachSideChannels(d)
	return d
}

// Render returns the most recent render without dispatching an event.
func (s *Session) Render() *Rendered {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// Event dispatches a client event: it runs HandleEvent, applies any live patch
// the handler requested, re-renders, computes the minimal [Diff] against the
// previous render (including component, stream, push-event, and navigation
// side-channels), caches the new render, and returns the diff.
func (s *Session) Event(event string, payload map[string]any) (Diff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if payload == nil {
		payload = map[string]any{}
	}
	if err := s.view.HandleEvent(event, payload, s.socket); err != nil {
		return nil, err
	}
	return s.commitLocked()
}

// ComponentEvent routes an event to a stateful component by cid, then re-renders
// the parent view and returns the resulting diff (which carries the component's
// change under the "c" key).
func (s *Session) ComponentEvent(cid int, event string, payload map[string]any) (Diff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if payload == nil {
		payload = map[string]any{}
	}
	if err := s.socket.comps.event(cid, event, payload); err != nil {
		return nil, err
	}
	return s.commitLocked()
}

// Info dispatches a server-side message to [InfoHandler.HandleInfo] (if the view
// implements it), then re-renders and returns the diff. Messages arrive via
// [Socket.Send], [PubSub] broadcasts, or upload progress.
func (s *Session) Info(msg any) (Diff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ih, ok := s.view.(InfoHandler); ok {
		if err := ih.HandleInfo(msg, s.socket); err != nil {
			return nil, err
		}
	}
	return s.commitLocked()
}

// Params dispatches a live patch: it updates the recorded URI and invokes
// [ParamsHandler.HandleParams] (if implemented), then re-renders and returns the
// diff.
func (s *Session) Params(params map[string]any, uri string) (Diff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uri = uri
	if params == nil {
		params = parseURIParams(uri)
	}
	if ph, ok := s.view.(ParamsHandler); ok {
		if err := ph.HandleParams(params, uri, s.socket); err != nil {
			return nil, err
		}
	}
	return s.commitLocked()
}

// UploadChunk feeds a chunk of an upload entry's bytes into the named slot,
// updates progress, and, when the entry completes or on any progress step,
// re-renders and returns the diff so the client reflects the new progress.
func (s *Session) UploadChunk(name, ref string, data []byte, last bool) (Diff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.socket.uploads[name]
	if u == nil {
		return Diff{}, nil
	}
	if _, err := u.appendChunk(ref, data, last); err != nil {
		return nil, err
	}
	return s.commitLocked()
}

// RegisterUploadEntry announces an upload entry's metadata (before its chunks
// arrive) on the named slot.
func (s *Session) RegisterUploadEntry(name, ref, fileName string, size int64, mime string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.socket.uploads[name]
	if u == nil {
		return nil
	}
	_, err := u.register(ref, fileName, size, mime)
	return err
}

// Close unsubscribes the session from all PubSub topics. Call it when the
// transport disconnects so stale mailbox channels are not retained.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pubsub != nil {
		s.pubsub.UnsubscribeAll(s.socket.inbox)
	}
}

// commitLocked re-renders the view, diffs against the last render, attaches the
// stream/push-event/navigation side-channels, caches the new render, and returns
// the diff. The caller must hold s.mu.
func (s *Session) commitLocked() (Diff, error) {
	s.socket.ResetChanges()
	next := s.view.Render(s.socket.Assigns())
	diff := DiffRendered(s.last, next)
	s.last = next
	if c := s.socket.comps.diffComponents(); c != nil {
		diff[componentsKey] = c
	}
	s.attachSideChannels(diff)
	return diff, nil
}

// attachSideChannels folds pending stream operations, push events, and a pending
// navigation into diff under reserved keys.
func (s *Session) attachSideChannels(diff Diff) {
	if streams := s.socket.drainStreams(); streams != nil {
		diff[streamKey] = streams
	}
	if events := s.socket.takeEvents(); len(events) > 0 {
		diff[eventsKey] = events
	}
	if nav := s.socket.takeNav(); nav != nil {
		diff[navKey] = nav
	}
}

// Reserved diff keys for the runtime side-channels.
const (
	streamKey = "stream"
	eventsKey = "e"
	navKey    = "nav"
)
