// Package liveview is a small, dependency-free (standard library only) reactive
// server-rendered UI framework in the spirit of Phoenix LiveView. Server-held
// state drives the UI: the client sends events, the server updates state,
// re-renders, and ships back a minimal patch describing only what changed.
//
// # Lifecycle
//
// A UI is a [View]:
//
//	Mount(params, socket)        seed server state ("assigns") for a new session
//	HandleEvent(event, payload, socket)  react to a client event by updating assigns
//	Render(assigns)              produce HTML as a static/dynamic [Rendered]
//
// A [Session] wires these together for one connection. [Session.Mount] runs
// Mount and the first Render; [Session.Event] runs HandleEvent, re-renders, and
// returns the [Diff] against the previous render. Assigns live in a [Socket],
// which also tracks which keys changed since the last render.
//
// # Assigns
//
// Assigns are an arbitrary map[string]any of server-side state. Views read and
// write them through the [Socket] (Assign/Get and typed helpers). Render
// receives an immutable snapshot, so rendering is a pure function of state.
//
// # The static/dynamic diff model
//
// This is the core idea. Rendering does not yield a flat string; it yields a
// [Rendered]: a template cut into Statics (literal fragments that never change)
// and Dynamics (the interpolated values). The full document is the two
// interleaved. Because the statics of repeated renders of the same view are
// identical, a diff only needs to carry the dynamics that actually changed.
//
//	template:  <span class="value">{{ count }}</span>
//	statics:   ["<span class=\"value\">", "</span>"]
//	dynamics:  ["0"]                      // count == 0
//
// After an "inc" event only the dynamic changes, so [DiffRendered] returns the
// sparse patch {"0":"1"} — the static wrapper is never re-sent. Dynamics may
// themselves be nested [Rendered] values (components), and the diff recurses
// into them, so an unchanged component contributes nothing to the patch. On the
// very first render [FullDiff] additionally includes the statics under the "s"
// key so a fresh client can build the document from scratch. Diffs marshal to
// the same compact JSON shape LiveView uses, e.g. {"s":[...],"0":"1"}.
//
// Values are HTML-escaped by default; wrap already-trusted markup in [Safe] to
// opt out.
//
// # Runtime and transport
//
// [NewHandler] adapts a View factory into an [http.Handler]: GET serves the full
// initial HTML page (with a tiny inline JS stub and a per-request session id),
// and POST /event accepts a JSON event and replies with the JSON diff. The
// transport is deliberately simple (HTTP + JSON); the state -> render -> diff
// engine is independent of it and can be driven directly, as the tests and
// examples do.
//
// # Example
//
// See [Counter] for a complete example View, and the package examples for a full
// mount/render/event/diff walkthrough.
package liveview
