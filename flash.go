package liveview

import "sort"

// flashAssignKey is the reserved assign under which a socket's [Flash] map is
// stored, so flash messages travel with the render like any other assign while
// staying out of user-chosen key space.
const flashAssignKey = "__flash__"

// Flash is a small map of transient user-facing messages keyed by kind (for
// example "info" or "error"), the Go analog of Phoenix LiveView's flash. A view
// puts a message during an event and renders it on the next frame; a typical
// client clears it after display. Flash is an ordinary map, safe to range over
// and to interpolate into templates by kind.
type Flash map[string]string

// NewFlash returns an empty Flash ready to receive messages.
func NewFlash() Flash { return Flash{} }

// Put stores msg under kind, replacing any existing message for that kind.
func (f Flash) Put(kind, msg string) { f[kind] = msg }

// Get returns the message stored under kind, or "" if none is set.
func (f Flash) Get(kind string) string { return f[kind] }

// Has reports whether a message is set for kind.
func (f Flash) Has(kind string) bool { _, ok := f[kind]; return ok }

// Delete removes the message stored under kind, if any.
func (f Flash) Delete(kind string) { delete(f, kind) }

// Clear removes every message, leaving the map empty but non-nil.
func (f Flash) Clear() {
	for k := range f {
		delete(f, k)
	}
}

// Kinds returns the kinds currently holding a message, sorted for deterministic
// iteration and rendering.
func (f Flash) Kinds() []string {
	kinds := make([]string, 0, len(f))
	for k := range f {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// Merge copies every message from other into f, overwriting on key collision,
// and returns f for chaining.
func (f Flash) Merge(other Flash) Flash {
	for k, v := range other {
		f[k] = v
	}
	return f
}

// SocketFlash returns the [Flash] stored on the socket, creating and attaching
// an empty one on first use so callers always receive a usable map.
func SocketFlash(s *Socket) Flash {
	if v, ok := s.assigns[flashAssignKey]; ok {
		if fl, ok := v.(Flash); ok {
			return fl
		}
	}
	fl := NewFlash()
	s.assigns[flashAssignKey] = fl
	return fl
}

// PutFlash stores a flash message of the given kind on the socket and marks the
// flash assign changed so the message is included in the next diff. It is the Go
// analog of Phoenix's put_flash.
func PutFlash(s *Socket, kind, msg string) {
	SocketFlash(s).Put(kind, msg)
	s.changed[flashAssignKey] = true
}

// GetFlash returns the socket's flash message for kind, or "" if none is set.
func GetFlash(s *Socket, kind string) string {
	if v, ok := s.assigns[flashAssignKey]; ok {
		if fl, ok := v.(Flash); ok {
			return fl.Get(kind)
		}
	}
	return ""
}

// ClearFlash removes every flash message from the socket and marks the flash
// assign changed. It is the Go analog of Phoenix's clear_flash.
func ClearFlash(s *Socket) {
	SocketFlash(s).Clear()
	s.changed[flashAssignKey] = true
}
