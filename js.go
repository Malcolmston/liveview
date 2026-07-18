package liveview

import "encoding/json"

// JS builds a client-side command list, mirroring Phoenix LiveView's JS
// command API. Instead of a round-trip to the server, a JS value describes DOM
// operations (toggling a class, hiding an element, dispatching an event) that
// the browser client executes locally, optionally alongside a server push.
//
// A JS value is immutable-per-step: each builder method returns a new JS with
// the command appended, so chains read left to right:
//
//	liveview.NewJS().AddClass("open", "#menu").Push("refresh").String()
//
// The resulting string is JSON — an array of [op, args] pairs — suitable for a
// phx-click / phx-* attribute value. The served client parses it and runs each
// command in order.
type JS struct {
	cmds []jsCmd
}

// jsCmd is a single client command: an operation name and its arguments. It
// marshals to the two-element array ["op", {args}] the client expects.
type jsCmd struct {
	op   string
	args map[string]any
}

// MarshalJSON encodes the command as ["op", {args}].
func (c jsCmd) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{c.op, c.args})
}

// NewJS returns an empty JS command list.
func NewJS() *JS { return &JS{} }

// with returns a copy of j with cmd appended, keeping each builder step pure.
func (j *JS) with(op string, args map[string]any) *JS {
	next := make([]jsCmd, len(j.cmds), len(j.cmds)+1)
	copy(next, j.cmds)
	next = append(next, jsCmd{op: op, args: args})
	return &JS{cmds: next}
}

// Push appends a command that sends event to the server with an optional target
// selector (the phx element the payload is gathered from).
func (j *JS) Push(event string) *JS {
	return j.with("push", map[string]any{"event": event})
}

// PushTo is like [JS.Push] but scopes the event to a specific component or DOM
// target selector.
func (j *JS) PushTo(event, target string) *JS {
	return j.with("push", map[string]any{"event": event, "target": target})
}

// AddClass appends a command that adds the space-separated names to the
// selector's class list.
func (j *JS) AddClass(names, selector string) *JS {
	return j.with("add_class", map[string]any{"names": names, "to": selector})
}

// RemoveClass appends a command that removes the space-separated names from the
// selector's class list.
func (j *JS) RemoveClass(names, selector string) *JS {
	return j.with("remove_class", map[string]any{"names": names, "to": selector})
}

// ToggleClass appends a command that toggles the space-separated names on the
// selector.
func (j *JS) ToggleClass(names, selector string) *JS {
	return j.with("toggle_class", map[string]any{"names": names, "to": selector})
}

// Toggle appends a command that toggles the visibility of the selector.
func (j *JS) Toggle(selector string) *JS {
	return j.with("toggle", map[string]any{"to": selector})
}

// Show appends a command that shows the selector.
func (j *JS) Show(selector string) *JS {
	return j.with("show", map[string]any{"to": selector})
}

// Hide appends a command that hides the selector.
func (j *JS) Hide(selector string) *JS {
	return j.with("hide", map[string]any{"to": selector})
}

// SetAttribute appends a command that sets attr=value on the selector.
func (j *JS) SetAttribute(attr, value, selector string) *JS {
	return j.with("set_attr", map[string]any{"attr": attr, "value": value, "to": selector})
}

// Dispatch appends a command that dispatches a DOM event of the given type on
// the selector.
func (j *JS) Dispatch(event, selector string) *JS {
	return j.with("dispatch", map[string]any{"event": event, "to": selector})
}

// Commands returns the raw command tuples, primarily for tests.
func (j *JS) Commands() []jsCmd { return j.cmds }

// MarshalJSON encodes the command list as a JSON array.
func (j *JS) MarshalJSON() ([]byte, error) {
	if len(j.cmds) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(j.cmds)
}

// String returns the JSON encoding of the command list, ready to embed in a
// phx-* attribute value. It is also a [Safe] string when used in templates via
// [JS.Safe].
func (j *JS) String() string {
	b, err := j.MarshalJSON()
	if err != nil {
		return "[]"
	}
	return string(b)
}

// Safe returns the command list as a [Safe] string for direct interpolation
// into a template attribute without double-escaping the JSON.
func (j *JS) Safe() Safe { return Safe(j.String()) }
