package liveview

import "net/url"

// Nav is a pending live navigation requested by a view during an event. A patch
// stays on the same view and re-invokes [ParamsHandler.HandleParams] with the
// new URL's params; a navigate performs a full live redirect to a new URL. The
// runtime relays it to the client, which updates the browser history.
type Nav struct {
	// Kind is "patch" (same view, handle_params) or "navigate" (live redirect).
	Kind string `json:"kind"`
	// To is the destination URL or path.
	To string `json:"to"`
	// Replace requests history replacement rather than a push.
	Replace bool `json:"replace"`
}

// PushEvent is a server-initiated client-side event, the analog of Phoenix's
// push_event: it names a browser event and carries an arbitrary JSON payload for
// a client hook to react to. Pending push events are drained into the diff reply.
type PushEvent struct {
	// Name is the event name dispatched on the client.
	Name string `json:"event"`
	// Payload is the arbitrary JSON data delivered with the event.
	Payload map[string]any `json:"payload"`
}

// ParamsHandler is an optional [View] interface. When implemented, the runtime
// calls HandleParams after Mount and on every live patch, passing the current
// URL params and the full request URI. It lets a view react to URL changes
// (pagination, tabs, filters) without a full remount.
type ParamsHandler interface {
	// HandleParams reacts to the connection's URL parameters and updates assigns.
	HandleParams(params map[string]any, uri string, socket *Socket) error
}

// InfoHandler is an optional [View] interface for server-initiated messages.
// When implemented, messages delivered to the session (via [Socket.Send],
// [Socket.Subscribe] + [PubSub.Broadcast], or upload progress) are dispatched to
// HandleInfo, which updates assigns; the runtime then re-renders and diffs.
type InfoHandler interface {
	// HandleInfo reacts to a server-side message and updates assigns.
	HandleInfo(msg any, socket *Socket) error
}

// PushPatch requests a live patch to the given path: the URL changes and
// [ParamsHandler.HandleParams] runs, but the view is not remounted. It is the
// Go analog of Phoenix's push_patch.
func (s *Socket) PushPatch(to string) {
	s.nav = &Nav{Kind: "patch", To: to}
}

// PushNavigate requests a live redirect to the given path, mounting the target
// route fresh. It is the Go analog of Phoenix's push_navigate.
func (s *Socket) PushNavigate(to string) {
	s.nav = &Nav{Kind: "navigate", To: to}
}

// Nav returns the pending navigation, or nil if none was requested.
func (s *Socket) Nav() *Nav { return s.nav }

// takeNav returns and clears the pending navigation.
func (s *Socket) takeNav() *Nav {
	n := s.nav
	s.nav = nil
	return n
}

// PushEvent queues a client-side event with the given name and payload, to be
// dispatched in the browser after the next diff is applied.
func (s *Socket) PushEvent(name string, payload map[string]any) {
	s.events = append(s.events, PushEvent{Name: name, Payload: payload})
}

// takeEvents returns and clears the queued push events.
func (s *Socket) takeEvents() []PushEvent {
	ev := s.events
	s.events = nil
	return ev
}

// Send delivers msg to this session's own mailbox, to be processed by
// [InfoHandler.HandleInfo]. It is the Go analog of Phoenix's send(self(), msg)
// and is a no-op if the socket is not attached to a live session.
func (s *Socket) Send(msg any) {
	if s.inbox == nil {
		return
	}
	select {
	case s.inbox <- msg:
	default:
	}
}

// Subscribe subscribes this session's mailbox to a PubSub topic. Messages
// broadcast on the topic are delivered to [InfoHandler.HandleInfo]. It is a
// no-op if no PubSub is attached.
func (s *Socket) Subscribe(topic string) {
	if s.pubsub == nil || s.inbox == nil {
		return
	}
	s.pubsub.Subscribe(topic, s.inbox)
	s.topics = append(s.topics, topic)
}

// Connected reports whether a live (websocket) transport is attached. A view can
// use this to defer expensive work until the socket connects, mirroring
// Phoenix's connected?/1.
func (s *Socket) Connected() bool { return s.connected }

// Components returns the socket's component manager, lazily creating one so a
// bare socket (constructed with NewSocket, e.g. in tests) can still reference
// components.
func (s *Socket) Components() *ComponentManager {
	if s.comps == nil {
		s.comps = NewComponentManager()
	}
	return s.comps
}

// LiveComponent references a stateful [Component] from a view's render. The first
// reference mounts the component; later references reuse its persisted state. The
// returned value is embedded in the render tree and diffs by cid under the "c"
// key. It panics only if the component's Mount returns an error, which is a
// programming error in Render; use [Socket.TryLiveComponent] to handle it.
func (s *Socket) LiveComponent(c Component) *componentRef {
	ref, err := s.Components().ensure(c)
	if err != nil {
		panic("liveview: component mount failed: " + err.Error())
	}
	return ref
}

// TryLiveComponent is like [Socket.LiveComponent] but returns an error instead
// of panicking when the component's Mount fails.
func (s *Socket) TryLiveComponent(c Component) (*componentRef, error) {
	return s.Components().ensure(c)
}

// AllowUpload declares a named upload slot with the given options, enabling the
// client to stream files to it. Re-declaring an existing slot keeps its
// in-flight entries.
func (s *Socket) AllowUpload(name string, opts UploadOptions) {
	if _, ok := s.uploads[name]; ok {
		return
	}
	s.uploads[name] = newUploadConfig(name, opts)
}

// Upload returns the config for a declared upload slot, or nil.
func (s *Socket) Upload(name string) *UploadConfig { return s.uploads[name] }

// UploadEntries returns the in-flight entries for the named upload slot.
func (s *Socket) UploadEntries(name string) []*UploadEntry {
	if u := s.uploads[name]; u != nil {
		return u.Entries()
	}
	return nil
}

// ConsumeUploadedEntries invokes fn for each completed entry of the named upload
// slot, removing them, and returns fn's results. Incomplete entries are left in
// place. It is the Go analog of Phoenix's consume_uploaded_entries.
func ConsumeUploadedEntries(s *Socket, name string, fn func(*UploadEntry) any) []any {
	u := s.uploads[name]
	if u == nil {
		return nil
	}
	return u.consume(fn)
}

// Stream returns the named stream, creating it on first use. A stream lets a
// view push append/prepend/delete operations to a phx-update="stream" container
// without retaining the full item list server-side.
func (s *Socket) Stream(name string) *Stream {
	st, ok := s.streams[name]
	if !ok {
		st = &Stream{Name: name}
		s.streams[name] = st
	}
	return st
}

// StreamInsert queues an item insert on the named stream (at == -1 appends,
// 0 prepends), a convenience over Stream(name).Insert.
func (s *Socket) StreamInsert(name, dom string, at int, r *Rendered) {
	s.Stream(name).Insert(dom, at, r)
}

// StreamDelete queues removal of an item from the named stream.
func (s *Socket) StreamDelete(name, dom string) { s.Stream(name).Delete(dom) }

// drainStreams collects every stream's pending operations keyed by stream name,
// clearing the queues. It returns nil when nothing is pending.
func (s *Socket) drainStreams() map[string][]StreamOp {
	var out map[string][]StreamOp
	for name, st := range s.streams {
		if ops := st.drain(); len(ops) > 0 {
			if out == nil {
				out = make(map[string][]StreamOp)
			}
			out[name] = ops
		}
	}
	return out
}

// parseURIParams extracts query parameters from a URI into a params map, taking
// the first value of each key. A malformed URI yields an empty map.
func parseURIParams(uri string) map[string]any {
	params := map[string]any{}
	u, err := url.Parse(uri)
	if err != nil {
		return params
	}
	for k, vs := range u.Query() {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}
	return params
}
