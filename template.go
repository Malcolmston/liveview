package liveview

import (
	"fmt"
	"strconv"
	"strings"
)

// Template is a compiled template: a fixed list of static fragments plus, for
// each dynamic slot, the name of the assign to substitute there. Compiling once
// and rendering many times is what makes the static/dynamic split cheap: the
// Statics of every render produced by the same Template are identical, so the
// diff engine can ignore them entirely.
//
// Template syntax is intentionally tiny. Text is copied verbatim; a dynamic
// slot is written as {{ name }} where name is a key looked up in the socket
// assigns at render time. Whitespace inside the braces is trimmed. A literal
// "{{" is written as "{{{{". Values are HTML-escaped unless they implement
// [Safe] or are a *Rendered.
type Template struct {
	statics []string
	fields  []string
}

// MustParse is like [Parse] but panics on error. It is meant for package-level
// template variables where a parse failure is a programming error.
func MustParse(src string) *Template {
	t, err := Parse(src)
	if err != nil {
		panic(err)
	}
	return t
}

// Parse compiles a template source string into a [Template]. It returns an
// error if a "{{" is not matched by a closing "}}".
func Parse(src string) (*Template, error) {
	var (
		statics []string
		fields  []string
		cur     strings.Builder
	)
	for i := 0; i < len(src); {
		// Escaped literal "{{{{" -> "{{".
		if strings.HasPrefix(src[i:], "{{{{") {
			cur.WriteString("{{")
			i += 4
			continue
		}
		if strings.HasPrefix(src[i:], "{{") {
			end := strings.Index(src[i+2:], "}}")
			if end < 0 {
				return nil, fmt.Errorf("liveview: unterminated {{ at offset %d", i)
			}
			name := strings.TrimSpace(src[i+2 : i+2+end])
			if name == "" {
				return nil, fmt.Errorf("liveview: empty {{ }} slot at offset %d", i)
			}
			statics = append(statics, cur.String())
			cur.Reset()
			fields = append(fields, name)
			i += 2 + end + 2
			continue
		}
		cur.WriteByte(src[i])
		i++
	}
	statics = append(statics, cur.String())
	return &Template{statics: statics, fields: fields}, nil
}

// Render produces a [Rendered] by looking up each dynamic slot's field in
// assigns and escaping it. Missing assigns render as the empty string. The
// returned Statics are shared (read-only) with the Template, so callers must
// not mutate them.
func (t *Template) Render(assigns map[string]any) *Rendered {
	dyn := make([]any, len(t.fields))
	for i, f := range t.fields {
		dyn[i] = escape(assigns[f])
	}
	return &Rendered{Statics: t.statics, Dynamics: dyn}
}

// toString renders a scalar value the way templates interpolate it, before
// escaping. It avoids fmt for the common cases to keep output deterministic.
func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}
