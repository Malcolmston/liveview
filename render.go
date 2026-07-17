package liveview

import (
	"html"
	"strings"
)

// Safe wraps a string that is already valid, trusted HTML and therefore should
// not be HTML-escaped when interpolated into a template. Use it deliberately;
// wrapping attacker-controlled data in Safe defeats the framework's automatic
// escaping.
type Safe string

// Rendered is the result of rendering a [View]. It is deliberately NOT a flat
// string: it is the classic Phoenix LiveView "static/dynamic" split.
//
// A template is cut into Statics, the literal parts that never change between
// renders, and Dynamics, the values that are substituted between the static
// parts. The invariant is:
//
//	len(Statics) == len(Dynamics) + 1
//
// and the full document is reconstructed by interleaving them:
//
//	Statics[0] + Dynamics[0] + Statics[1] + Dynamics[1] + ... + Statics[n]
//
// Each entry in Dynamics is either a string (already HTML-escaped, ready to be
// written verbatim) or a nested *Rendered (a sub-template / component). Because
// the statics are known to be identical for repeated renders of the same view,
// a diff only needs to transmit the dynamics that actually changed. See [Diff].
type Rendered struct {
	// Statics holds the literal template fragments surrounding each dynamic
	// slot. It always has exactly one more element than Dynamics.
	Statics []string
	// Dynamics holds the value for each dynamic slot. Each element is either
	// a string (escaped, render-ready) or a *Rendered (nested component).
	Dynamics []any
}

// HTML materializes the Rendered tree into a single HTML string by interleaving
// statics and dynamics (recursing into nested *Rendered values).
func (r *Rendered) HTML() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	r.writeHTML(&b)
	return b.String()
}

func (r *Rendered) writeHTML(b *strings.Builder) {
	for i, s := range r.Statics {
		b.WriteString(s)
		if i < len(r.Dynamics) {
			switch d := r.Dynamics[i].(type) {
			case string:
				b.WriteString(d)
			case *Rendered:
				d.writeHTML(b)
			}
		}
	}
}

// escape converts an arbitrary interpolation value into the string or *Rendered
// form stored in Dynamics, applying HTML escaping to untrusted strings.
func escape(v any) any {
	switch t := v.(type) {
	case *Rendered:
		return t
	case Safe:
		return string(t)
	case string:
		return html.EscapeString(t)
	default:
		return html.EscapeString(toString(t))
	}
}
