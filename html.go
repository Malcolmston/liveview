package liveview

import (
	"html"
	"sort"
	"strings"
)

// ClassList builds a space-separated CSS class string from a set of
// class-name -> enabled flags, the Go analog of Phoenix's dynamic class list
// helper. Only classes mapped to true are included, and the result is sorted so
// the same input always yields the same string (important for the diff engine,
// which compares rendered output byte for byte). Class names are emitted
// verbatim and are assumed to be developer-controlled, not user input.
func ClassList(classes map[string]bool) string {
	names := make([]string, 0, len(classes))
	for name, on := range classes {
		if on && name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

// AttrList renders an HTML attribute string from a map of attribute name ->
// value, sorted by name for deterministic output. Values are HTML-escaped;
// a bool value renders as a bare boolean attribute when true and is omitted when
// false (for example {"disabled": true} -> ` disabled`). The result always
// begins with a leading space when non-empty so it can be concatenated directly
// after a tag name. It is returned as [Safe] because it is already escaped.
func AttrList(attrs map[string]any) Safe {
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		switch v := attrs[name].(type) {
		case bool:
			if v {
				b.WriteByte(' ')
				b.WriteString(html.EscapeString(name))
			}
		default:
			b.WriteByte(' ')
			b.WriteString(html.EscapeString(name))
			b.WriteString(`="`)
			b.WriteString(html.EscapeString(toString(v)))
			b.WriteByte('"')
		}
	}
	return Safe(b.String())
}

// HiddenInputs renders a sorted set of hidden <input> elements from a map of
// name -> value, the Go analog of Phoenix's hidden form inputs (used for tokens,
// ids, and method overrides). Names and values are HTML-escaped and the output
// is deterministic. It is returned as [Safe] for direct template interpolation.
func HiddenInputs(fields map[string]string) Safe {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(`<input type="hidden" name="`)
		b.WriteString(html.EscapeString(name))
		b.WriteString(`" value="`)
		b.WriteString(html.EscapeString(fields[name]))
		b.WriteString(`">`)
	}
	return Safe(b.String())
}

// LivePatch renders an anchor that performs a live patch to href when clicked,
// the Go analog of Phoenix's live_patch link. The href and text are
// HTML-escaped. The served client interprets the data-phx-link attribute to
// patch the current view without a full navigation.
func LivePatch(text, href string) Safe {
	return liveLink(text, href, "patch")
}

// LiveNavigate renders an anchor that performs a live navigation to href when
// clicked, the Go analog of Phoenix's live_redirect link. The href and text are
// HTML-escaped. The served client interprets the data-phx-link attribute to
// mount the target route without a full page load.
func LiveNavigate(text, href string) Safe {
	return liveLink(text, href, "redirect")
}

// liveLink renders the shared anchor markup for [LivePatch] and [LiveNavigate].
func liveLink(text, href, kind string) Safe {
	var b strings.Builder
	b.WriteString(`<a href="`)
	b.WriteString(html.EscapeString(href))
	b.WriteString(`" data-phx-link="`)
	b.WriteString(kind)
	b.WriteString(`" data-phx-link-state="push">`)
	b.WriteString(html.EscapeString(text))
	b.WriteString(`</a>`)
	return Safe(b.String())
}
