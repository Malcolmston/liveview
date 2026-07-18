package liveview

// Component is a stateful live component: a self-contained, independently
// addressable piece of UI with its own assigns, lifecycle, and render, embedded
// inside a parent [View]. Components are identified by a stable [Component.ID];
// the runtime assigns each a numeric component id ("cid") and tracks its render
// separately, so a component's changes travel in the diff under the reserved
// "c" key addressed by cid — the parent's statics and unrelated dynamics never
// re-send when only a component updates.
//
// A component is referenced from a view with [Socket.LiveComponent], typically
// during Mount, storing the returned reference in an assign that the template
// interpolates. The first reference mounts the component and renders it; its
// per-component socket then persists, so component state survives across parent
// re-renders. Events targeting a component are routed to its HandleEvent by cid.
type Component interface {
	// ID returns the component's stable identifier. Two references with the
	// same ID address the same component instance and state.
	ID() string
	// Mount initializes the component's assigns. It runs once, when the
	// component is first referenced.
	Mount(socket *Socket) error
	// HandleEvent processes an event routed to this component by cid.
	HandleEvent(event string, payload map[string]any, socket *Socket) error
	// Render builds the component's markup from an assigns snapshot.
	Render(assigns map[string]any) *Rendered
}

// componentRef is the value embedded in a parent [Rendered]'s dynamics to stand
// in for a component. The parent's diff slot carries only the stable cid; the
// component's own render and diff are managed by the [ComponentManager] and shipped
// under the "c" key. The state pointer lets the parent produce full server HTML.
type componentRef struct {
	cid   int
	state *componentState
}

// componentState is the runtime's per-component bookkeeping: its socket, the
// most recently produced render (for HTML), and the render last shipped to the
// client (for diffing).
type componentState struct {
	cid     int
	comp    Component
	socket  *Socket
	current *Rendered
	sent    *Rendered
}

// ComponentManager tracks the stateful components referenced by one [Session]:
// it assigns cids, persists each component's socket and renders, routes events
// by cid, and produces the per-cid component diffs shipped under the "c" key. It
// is not safe for concurrent use on its own; the owning Session serializes access.
type ComponentManager struct {
	byID  map[string]*componentState
	byCID map[int]*componentState
	order []*componentState
	next  int
}

// NewComponentManager returns an empty manager. cids are assigned starting at 1.
func NewComponentManager() *ComponentManager {
	return &ComponentManager{
		byID:  make(map[string]*componentState),
		byCID: make(map[int]*componentState),
		next:  1,
	}
}

// ensure returns a reference to c, mounting and rendering it on first sight and
// reusing its persisted state thereafter.
func (m *ComponentManager) ensure(c Component) (*componentRef, error) {
	st, ok := m.byID[c.ID()]
	if !ok {
		st = &componentState{cid: m.next, comp: c, socket: NewSocket()}
		m.next++
		if err := c.Mount(st.socket); err != nil {
			return nil, err
		}
		st.socket.ResetChanges()
		st.current = c.Render(st.socket.Assigns())
		m.byID[c.ID()] = st
		m.byCID[st.cid] = st
		m.order = append(m.order, st)
	}
	return &componentRef{cid: st.cid, state: st}, nil
}

// event routes an event to the component addressed by cid and updates its
// assigns. The subsequent commit re-renders the component and produces its diff.
func (m *ComponentManager) event(cid int, event string, payload map[string]any) error {
	st, ok := m.byCID[cid]
	if !ok {
		return nil
	}
	return st.comp.HandleEvent(event, payload, st.socket)
}

// EventByID routes an event to the component with the given string ID, primarily
// for tests and direct drivers that address components by name rather than cid.
func (m *ComponentManager) EventByID(id, event string, payload map[string]any) error {
	st, ok := m.byID[id]
	if !ok {
		return nil
	}
	return st.comp.HandleEvent(event, payload, st.socket)
}

// CID returns the numeric component id assigned to the component with the given
// string ID, or 0 if it has not been referenced yet.
func (m *ComponentManager) CID(id string) int {
	if st, ok := m.byID[id]; ok {
		return st.cid
	}
	return 0
}

// fullComponents renders every registered component and returns the initial "c"
// map: each cid mapped to its full diff. It marks each render as sent.
func (m *ComponentManager) fullComponents() map[string]any {
	if len(m.order) == 0 {
		return nil
	}
	c := make(map[string]any, len(m.order))
	for _, st := range m.order {
		st.sent = st.current
		c[itoa(st.cid)] = FullDiff(st.current)
	}
	return c
}

// diffComponents re-renders every registered component and returns the "c" map
// of the components whose render changed since the last commit. It updates each
// component's current and sent renders.
func (m *ComponentManager) diffComponents() map[string]any {
	var c map[string]any
	for _, st := range m.order {
		st.socket.ResetChanges()
		next := st.comp.Render(st.socket.Assigns())
		sub := DiffRendered(st.sent, next)
		st.current = next
		st.sent = next
		if !sub.Empty() {
			if c == nil {
				c = make(map[string]any)
			}
			c[itoa(st.cid)] = sub
		}
	}
	return c
}
