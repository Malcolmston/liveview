package liveview

import (
	"net/url"
	"sort"
	"strings"
)

// DecodeForm decodes flat, bracket-encoded form values into a nested map, the Go
// analog of Phoenix's (Plug.Conn.Query) form parameter decoding used by
// phx-change and phx-submit. Keys use the conventional bracket syntax:
//
//	user[name]=Ada           -> {"user": {"name": "Ada"}}
//	user[address][city]=Bath -> {"user": {"address": {"city": "Bath"}}}
//	tags[]=a&tags[]=b        -> {"tags": ["a", "b"]}
//
// A bare "[]" suffix accumulates repeated values into a []string in encounter
// order. Named subkeys build nested map[string]any values. When the same scalar
// key repeats, the last value wins, matching browser form semantics. Malformed
// keys (unbalanced brackets) are treated as flat keys.
func DecodeForm(values url.Values) map[string]any {
	out := map[string]any{}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	// Deterministic assembly regardless of map iteration order.
	sort.Strings(keys)
	for _, key := range keys {
		vs := values[key]
		if len(vs) == 0 {
			continue
		}
		path := formParsePath(key)
		formAssignPath(out, path, vs)
	}
	return out
}

// DecodeFormString parses a URL-encoded query string and decodes it with
// [DecodeForm]. It returns an error only if the query string itself is malformed
// (per net/url).
func DecodeFormString(query string) (map[string]any, error) {
	values, err := url.ParseQuery(query)
	if err != nil {
		return nil, err
	}
	return DecodeForm(values), nil
}

// formParsePath splits a bracket-encoded key such as "user[address][city]" into
// its segments (["user", "address", "city"]). A trailing "[]" becomes a final
// empty-string segment signalling list accumulation.
func formParsePath(key string) []string {
	open := strings.IndexByte(key, '[')
	if open < 0 {
		return []string{key}
	}
	segments := []string{key[:open]}
	rest := key[open:]
	for len(rest) > 0 {
		if rest[0] != '[' {
			// Malformed; treat the remainder as flat.
			return []string{key}
		}
		close := strings.IndexByte(rest, ']')
		if close < 0 {
			return []string{key}
		}
		segments = append(segments, rest[1:close])
		rest = rest[close+1:]
	}
	return segments
}

// formAssignPath writes vs into out at the nested location described by path,
// creating intermediate maps as needed. A trailing empty segment ("[]") stores
// the whole value slice as an accumulated []string list under the preceding
// segment; otherwise the last value wins as a scalar.
func formAssignPath(out map[string]any, path []string, vs []string) {
	// A trailing "[]" makes the second-to-last segment the list's name.
	list := path[len(path)-1] == ""
	if list {
		path = path[:len(path)-1]
	}
	cur := out
	for i := 0; i < len(path)-1; i++ {
		seg := path[i]
		next, ok := cur[seg].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[seg] = next
		}
		cur = next
	}
	last := path[len(path)-1]
	if list {
		existing, _ := cur[last].([]string)
		cur[last] = append(existing, vs...)
		return
	}
	cur[last] = vs[len(vs)-1]
}

// Form is a lightweight form abstraction pairing decoded parameters with
// per-field validation errors, the Go analog of Phoenix's form/changeset used to
// drive phx-change validation and error display. It carries no schema of its own;
// a view populates Params (typically from [DecodeForm]) and records errors during
// validation, then renders field values and messages from it.
type Form struct {
	// Name is the form's parameter namespace (for example "user"), used when
	// building input names.
	Name string
	// Params holds the current field values, usually the sub-map for Name from
	// a decoded submission.
	Params map[string]any
	errors map[string][]string
}

// NewForm returns a Form for the given namespace and initial params. A nil
// params map is replaced with an empty one.
func NewForm(name string, params map[string]any) *Form {
	if params == nil {
		params = map[string]any{}
	}
	return &Form{Name: name, Params: params, errors: map[string][]string{}}
}

// Get returns the raw value of field, or nil if absent.
func (f *Form) Get(field string) any { return f.Params[field] }

// GetString returns field as a string, or "" if absent or not a string.
func (f *Form) GetString(field string) string {
	if v, ok := f.Params[field].(string); ok {
		return v
	}
	return ""
}

// AddError records a validation message for field. Multiple errors may be added
// to the same field; they are returned in insertion order.
func (f *Form) AddError(field, msg string) {
	if f.errors == nil {
		f.errors = map[string][]string{}
	}
	f.errors[field] = append(f.errors[field], msg)
}

// Errors returns the validation messages recorded for field, or nil if the
// field is valid.
func (f *Form) Errors(field string) []string { return f.errors[field] }

// HasErrors reports whether any field has a validation error.
func (f *Form) HasErrors() bool {
	for _, e := range f.errors {
		if len(e) > 0 {
			return true
		}
	}
	return false
}

// Valid reports whether the form has no validation errors. It is the negation of
// [Form.HasErrors].
func (f *Form) Valid() bool { return !f.HasErrors() }

// InputName returns the bracket-encoded HTML input name for field within this
// form's namespace, for example "user[email]" for a form named "user". An empty
// form name yields the bare field.
func (f *Form) InputName(field string) string {
	if f.Name == "" {
		return field
	}
	return f.Name + "[" + field + "]"
}
