package liveview

// Socket holds the server-side state for a single connected view: its "assigns"
// (an arbitrary key/value map) plus per-render change tracking. A View reads and
// writes assigns through the socket during Mount and HandleEvent; Render is then
// given a snapshot of those assigns.
//
// Change tracking records which keys were written since the last time [Socket.ResetChanges]
// was called (the runtime resets after every render). This lets a View cheaply
// ask whether it needs to recompute derived state, and mirrors how LiveView
// tracks assign changes to decide what to re-render.
type Socket struct {
	assigns map[string]any
	changed map[string]bool

	// Runtime side-channels populated during Mount/HandleEvent/HandleInfo and
	// drained by the owning Session. They are nil on a bare socket created with
	// NewSocket and wired up by NewSession.
	inbox     chan any                 // server-push mailbox (handle_info / pubsub)
	pubsub    *PubSub                  // hub for Subscribe; may be nil
	comps     *ComponentManager        // stateful component registry; may be nil
	nav       *Nav                     // pending push_patch / push_navigate
	events    []PushEvent              // pending push_event to the client
	uploads   map[string]*UploadConfig // declared upload slots
	streams   map[string]*Stream       // declared streams
	topics    []string                 // topics subscribed on this socket
	connected bool                     // true once a live (websocket) transport is attached
}

// NewSocket returns an empty Socket ready to receive assigns. The runtime
// side-channels (inbox, pubsub, components) are left nil; [NewSession] wires
// them up for a live connection.
func NewSocket() *Socket {
	return &Socket{
		assigns: make(map[string]any),
		changed: make(map[string]bool),
		uploads: make(map[string]*UploadConfig),
		streams: make(map[string]*Stream),
	}
}

// Assign sets a single assign and marks it changed.
func (s *Socket) Assign(key string, value any) {
	s.assigns[key] = value
	s.changed[key] = true
}

// AssignAll sets many assigns at once, marking each changed.
func (s *Socket) AssignAll(values map[string]any) {
	for k, v := range values {
		s.Assign(k, v)
	}
}

// Get returns the value for key and whether it was present.
func (s *Socket) Get(key string) (any, bool) {
	v, ok := s.assigns[key]
	return v, ok
}

// GetString returns the assign for key as a string, or "" if absent or not a
// string.
func (s *Socket) GetString(key string) string {
	if v, ok := s.assigns[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

// GetInt returns the assign for key as an int, or 0 if absent or not an int.
func (s *Socket) GetInt(key string) int {
	if v, ok := s.assigns[key]; ok {
		if n, ok := v.(int); ok {
			return n
		}
	}
	return 0
}

// Changed reports whether key was assigned since the last ResetChanges.
func (s *Socket) Changed(key string) bool {
	return s.changed[key]
}

// AnyChanged reports whether any assign changed since the last ResetChanges.
func (s *Socket) AnyChanged() bool {
	return len(s.changed) > 0
}

// ResetChanges clears the change set. The runtime calls this after each render.
func (s *Socket) ResetChanges() {
	if len(s.changed) > 0 {
		s.changed = make(map[string]bool)
	}
}

// Assigns returns a shallow copy of the current assigns, safe to hand to a
// Render method without exposing the socket's internal map.
func (s *Socket) Assigns() map[string]any {
	out := make(map[string]any, len(s.assigns))
	for k, v := range s.assigns {
		out[k] = v
	}
	return out
}
