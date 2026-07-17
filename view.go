package liveview

// View is the interface a live view implements. Its lifecycle is:
//
//  1. Mount is called once when a session is established. It seeds the socket's
//     assigns from the connection params (route params, query, etc.).
//  2. Render is called to produce the current HTML as a static/dynamic
//     [Rendered]. It is a pure function of the assigns snapshot and must not
//     mutate the socket.
//  3. HandleEvent is called for each client event (a button click, form submit,
//     ...). It reads the event name and payload, updates assigns on the socket,
//     and returns. The runtime then re-renders and diffs.
//
// A View should be safe to construct fresh per session; per-connection state
// lives in the [Socket], not in the View value.
type View interface {
	// Mount initializes assigns for a new session.
	Mount(params map[string]any, socket *Socket) error
	// HandleEvent processes a client event and updates the socket's assigns.
	HandleEvent(event string, payload map[string]any, socket *Socket) error
	// Render builds the current view from an assigns snapshot.
	Render(assigns map[string]any) *Rendered
}
