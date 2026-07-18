package liveview

// Upstream-parity tests. Each TestParity* function encodes concrete
// known-answer vectors taken from the ORIGINAL library this package mirrors,
// Phoenix LiveView (phoenixframework/phoenix_live_view), as deterministic
// assertions against this port's real exported API.
//
// Sources (fetched from raw.githubusercontent.com/phoenixframework/phoenix_live_view/main):
//   - test/phoenix_live_view/engine_test.exs   ("rendered structure", escaping, change tracking)
//   - test/phoenix_live_view/diff_test.exs      (full renders, diffed renders, components)
//
// Where Phoenix templates use <%= expr %> / {@assign} and this port uses the
// tiny {{ name }} syntax, the vector is translated to the equivalent Go
// template; the static/dynamic split, escaping, and diff wire-shape assertions
// are the upstream ones. Two deliberate, documented divergences are called out
// inline: (1) this port omits UNCHANGED leaf dynamics from a diff whereas
// Phoenix re-sends them (a stricter optimization, noted in TestParityDiff*),
// and (2) the JS command builder is a Go analog with its own matched client and
// is intentionally not asserted for byte-exact wire parity.

import (
	"encoding/json"
	"reflect"
	"testing"
)

// strDynamics extracts the dynamic slots of r as plain strings, asserting each
// slot is a leaf string (the shape the upstream "rendered structure" vectors
// use). It fails the test on any non-string slot.
func strDynamics(t *testing.T, r *Rendered) []string {
	t.Helper()
	out := make([]string, len(r.Dynamics))
	for i, d := range r.Dynamics {
		s, ok := d.(string)
		if !ok {
			t.Fatalf("slot %d is %T, want string", i, d)
		}
		out[i] = s
	}
	return out
}

// TestParityRenderedStructure mirrors engine_test.exs "rendered structure":
// a template is cut into static fragments surrounding dynamic slots, with the
// invariant len(static) == len(dynamic) + 1. Phoenix uses <%= 123 %>; here the
// equivalent {{ a }} / {{ b }} slots stand in, and the assigns supply the same
// literal results ("123", "456").
func TestParityRenderedStructure(t *testing.T) {
	assigns := map[string]any{"a": "123", "b": "456"}
	cases := []struct {
		name     string
		src      string
		statics  []string
		dynamics []string
	}{
		// "contains two static parts and one dynamic": foo<%= 123 %>bar
		{"two-static-one-dynamic", "foo{{ a }}bar", []string{"foo", "bar"}, []string{"123"}},
		// "one static part at the beginning and one dynamic": foo<%= 123 %>
		{"leading-static", "foo{{ a }}", []string{"foo", ""}, []string{"123"}},
		// "one static part at the end and one dynamic": <%= 123 %>bar
		{"trailing-static", "{{ a }}bar", []string{"", "bar"}, []string{"123"}},
		// "contains one dynamic only": <%= 123 %>
		{"one-dynamic-only", "{{ a }}", []string{"", ""}, []string{"123"}},
		// "contains two dynamics only": <%= 123 %><%= 456 %>
		{"two-dynamics-only", "{{ a }}{{ b }}", []string{"", "", ""}, []string{"123", "456"}},
		// "two static parts and two dynamics": foo<%= 123 %><%= 456 %>bar
		{"two-static-two-dynamic", "foo{{ a }}{{ b }}bar", []string{"foo", "", "bar"}, []string{"123", "456"}},
		// "three static parts and two dynamics": foo<%= 123 %>bar<%= 456 %>baz
		{"three-static-two-dynamic", "foo{{ a }}bar{{ b }}baz", []string{"foo", "bar", "baz"}, []string{"123", "456"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := MustParse(tc.src).Render(assigns)
			if !reflect.DeepEqual(r.Statics, tc.statics) {
				t.Errorf("statics = %#v, want %#v", r.Statics, tc.statics)
			}
			if got := strDynamics(t, r); !reflect.DeepEqual(got, tc.dynamics) {
				t.Errorf("dynamics = %#v, want %#v", got, tc.dynamics)
			}
			if len(r.Statics) != len(r.Dynamics)+1 {
				t.Errorf("invariant broken: len(statics)=%d len(dynamics)=%d", len(r.Statics), len(r.Dynamics))
			}
		})
	}
}

// TestParityEscaping mirrors engine_test.exs "escapes HTML" / "does not escape
// safe expressions" / "handles assigns". Untrusted interpolation is
// HTML-escaped; a Safe value passes through verbatim. The escaped forms are
// Phoenix's exact entities (note "&quot;" for the double quote, not Go's
// default "&#34;").
func TestParityEscaping(t *testing.T) {
	// "escapes HTML": <%= "<escaped>" %> -> "&lt;escaped&gt;"
	if got := MustParse(`{{ v }}`).Render(map[string]any{"v": "<escaped>"}).HTML(); got != "&lt;escaped&gt;" {
		t.Errorf("escape(<escaped>) = %q, want &lt;escaped&gt;", got)
	}
	// "handles assigns": <%= @foo %> with foo: "<hello>" -> "&lt;hello&gt;"
	if got := MustParse(`{{ foo }}`).Render(map[string]any{"foo": "<hello>"}).HTML(); got != "&lt;hello&gt;" {
		t.Errorf("escape assign = %q, want &lt;hello&gt;", got)
	}
	// "does not escape safe expressions": {:safe, "<value>"} -> "<value>"
	if got := MustParse(`Safe {{ v }}`).Render(map[string]any{"v": Safe("<value>")}).HTML(); got != "Safe <value>" {
		t.Errorf("safe passthrough = %q, want Safe <value>", got)
	}
	// Phoenix html_escape entity table (Phoenix.HTML): the double quote is the
	// only char whose entity differs from the Go stdlib default.
	if got := MustParse(`{{ v }}`).Render(map[string]any{"v": `"&<>'`}).HTML(); got != "&quot;&amp;&lt;&gt;&#39;" {
		t.Errorf("full entity table = %q, want &quot;&amp;&lt;&gt;&#39;", got)
	}
}

// TestParityChangeTracking mirrors engine_test.exs "change tracking": a dynamic
// is only re-rendered when its assigns changed. Phoenix threads a __changed__
// map; this port tracks the same information per key on the Socket. The vectors
// below reproduce the three states of engine_test's changed/3 for a single key
// and for the multi-assign "renders dynamic if any of the assigns change" case.
func TestParityChangeTracking(t *testing.T) {
	s := NewSocket()

	// nil __changed__ (first render): everything is considered changed.
	s.Assign("foo", 123)
	if !s.Changed("foo") {
		t.Error("first assign: foo must be changed")
	}
	if !s.AnyChanged() {
		t.Error("first assign: AnyChanged must be true")
	}

	// %{} (empty changed map): nothing changed since the last render.
	s.ResetChanges()
	if s.Changed("foo") {
		t.Error("after reset: foo must be unchanged")
	}
	if s.AnyChanged() {
		t.Error("after reset: AnyChanged must be false")
	}

	// %{foo: true}: writing foo marks only foo changed.
	s.Assign("foo", 124)
	if !s.Changed("foo") || s.Changed("bar") {
		t.Errorf("changed set wrong: foo=%v bar=%v, want true/false", s.Changed("foo"), s.Changed("bar"))
	}

	// "renders dynamic if any of the assigns change": foo + bar, either side.
	s.ResetChanges()
	s.Assign("bar", 456)
	if s.Changed("foo") {
		t.Error("only bar was written; foo must be unchanged")
	}
	if !s.Changed("bar") || !s.AnyChanged() {
		t.Error("bar written; bar and AnyChanged must be true")
	}
}

// TestParityFullDiffBasicTemplate mirrors diff_test.exs "basic template": a
// full render carries every dynamic slot keyed by index plus the statics under
// "s", and materializes to the exact upstream binary. The template statics are
// the same fragments Phoenix produces for
// `<div>\n  <h2>It's {@time}</h2>\n  {@subtitle}\n</div>`.
func TestParityFullDiffBasicTemplate(t *testing.T) {
	tmpl := MustParse("<div>\n  <h2>It's {{ time }}</h2>\n  {{ subtitle }}\n</div>")
	r := tmpl.Render(map[string]any{"time": "10:30", "subtitle": "Sunny"})

	full := FullDiff(r)
	if full["0"] != "10:30" || full["1"] != "Sunny" {
		t.Errorf("dynamics = %#v, want 0=10:30 1=Sunny", full)
	}
	if _, ok := full["s"].([]string); !ok {
		t.Errorf("full diff must carry statics under \"s\", got %#v", full["s"])
	}
	// diff_test.exs asserts rendered_to_binary(full_render) == this string.
	const wantHTML = "<div>\n  <h2>It's 10:30</h2>\n  Sunny\n</div>"
	if got := r.HTML(); got != wantHTML {
		t.Errorf("HTML = %q, want %q", got, wantHTML)
	}
}

// TestParityLiteralEscaping mirrors diff_test.exs "template with literal":
// interpolating the literal string "<div>" escapes to "&lt;div&gt;" and the
// binary matches Phoenix's exactly.
func TestParityLiteralEscaping(t *testing.T) {
	tmpl := MustParse("<div>\n  {{ title }}\n  {{ lit }}\n</div>")
	r := tmpl.Render(map[string]any{"title": "foo", "lit": "<div>"})

	full := FullDiff(r)
	if full["0"] != "foo" || full["1"] != "&lt;div&gt;" {
		t.Errorf("dynamics = %#v, want 0=foo 1=&lt;div&gt;", full)
	}
	const wantHTML = "<div>\n  foo\n  &lt;div&gt;\n</div>"
	if got := r.HTML(); got != wantHTML {
		t.Errorf("HTML = %q, want %q", got, wantHTML)
	}
}

// nestedParityRendered builds the Go equivalent of diff_test.exs's
// nested_rendered/1 fixture: a parent with statics
// ["<h2>","</h2>","<span>","</span>"] whose dynamics are a leaf "hi" and two
// nested *Rendered children. changed selects the "abc"/"efg" values (true) or
// leaves them identical to a prev (used for the unchanged-diff vector).
func nestedParityRendered(first, second string) *Rendered {
	return &Rendered{
		Statics: []string{"<h2>", "</h2>", "<span>", "</span>"},
		Dynamics: []any{
			"hi",
			&Rendered{Statics: []string{"s1", "s2", "s3"}, Dynamics: []any{first, "efg"}},
			&Rendered{Statics: []string{"s1", "s2"}, Dynamics: []any{second}},
		},
	}
}

// TestParityNestedRenderedFull mirrors diff_test.exs "nested %Rendered{}'s":
// a full render nests each child's own {statics, dynamics} and materializes to
// the exact upstream binary "<h2>hi</h2>s1abcs2efgs3<span>s1efgs2</span>".
func TestParityNestedRenderedFull(t *testing.T) {
	r := nestedParityRendered("abc", "efg")

	const wantHTML = "<h2>hi</h2>s1abcs2efgs3<span>s1efgs2</span>"
	if got := r.HTML(); got != wantHTML {
		t.Errorf("HTML = %q, want %q", got, wantHTML)
	}

	full := FullDiff(r)
	if full["0"] != "hi" {
		t.Errorf("slot 0 = %#v, want hi", full["0"])
	}
	child1, ok := full["1"].(Diff)
	if !ok || child1["0"] != "abc" || child1["1"] != "efg" {
		t.Errorf("slot 1 nested full = %#v, want {0:abc,1:efg,s:...}", full["1"])
	}
	if _, ok := child1["s"].([]string); !ok {
		t.Errorf("nested child must carry its own statics under \"s\", got %#v", child1["s"])
	}
	child2, ok := full["2"].(Diff)
	if !ok || child2["0"] != "efg" {
		t.Errorf("slot 2 nested full = %#v, want {0:efg,s:...}", full["2"])
	}
	if _, ok := full["s"].([]string); !ok {
		t.Errorf("parent must carry statics under \"s\", got %#v", full["s"])
	}
}

// TestParityDiffSkipsStaticsForKnownShape mirrors diff_test.exs "basic template
// skips statics for known fingerprints": when the previous and next renders
// share the same static shape, the diff omits "s" entirely and carries only the
// changed dynamic slots.
func TestParityDiffSkipsStaticsForKnownShape(t *testing.T) {
	tmpl := MustParse("<div>\n  <h2>It's {{ time }}</h2>\n  {{ subtitle }}\n</div>")
	prev := tmpl.Render(map[string]any{"time": "09:00", "subtitle": "Rainy"})
	next := tmpl.Render(map[string]any{"time": "10:30", "subtitle": "Sunny"})

	d := DiffRendered(prev, next)
	if _, ok := d["s"]; ok {
		t.Errorf("known shape must not re-send statics, got %#v", d)
	}
	if d["0"] != "10:30" || d["1"] != "Sunny" {
		t.Errorf("diff = %#v, want {0:10:30,1:Sunny}", d)
	}
}

// TestParityDiffNestedChanged mirrors diff_test.exs "renders nested
// %Rendered{}'s": with matching static shapes, a diff over nested children that
// all changed yields exactly {0:"hi",1:{0:"abc",1:"efg"},2:{0:"efg"}} — nested
// sub-diffs, no statics. Here prev holds different child values so every leaf
// changes, matching the upstream assertion byte for byte.
func TestParityDiffNestedChanged(t *testing.T) {
	// prev differs from next at EVERY leaf so all dynamics change; this makes
	// this port's diff carry every slot, matching the upstream vector exactly
	// (upstream re-sends leaves unconditionally, so its assertion is the same).
	prev := &Rendered{
		Statics: []string{"<h2>", "</h2>", "<span>", "</span>"},
		Dynamics: []any{
			"OLD-HI",
			&Rendered{Statics: []string{"s1", "s2", "s3"}, Dynamics: []any{"OLD1", "OLD2"}},
			&Rendered{Statics: []string{"s1", "s2"}, Dynamics: []any{"OLD3"}},
		},
	}
	next := nestedParityRendered("abc", "efg")

	d := DiffRendered(prev, next)
	if d["0"] != "hi" {
		t.Errorf("slot 0 = %#v, want hi", d["0"])
	}
	c1, _ := d["1"].(Diff)
	if c1["0"] != "abc" || c1["1"] != "efg" {
		t.Errorf("slot 1 sub-diff = %#v, want {0:abc,1:efg}", d["1"])
	}
	if _, ok := c1["s"]; ok {
		t.Errorf("changed nested child must not re-send statics, got %#v", c1)
	}
	c2, _ := d["2"].(Diff)
	if c2["0"] != "efg" {
		t.Errorf("slot 2 sub-diff = %#v, want {0:efg}", d["2"])
	}
}

// TestParityDiffNestedUnchanged mirrors diff_test.exs "does not emit nested
// %Rendered{}'s if they did not change": a nested child whose render is
// unchanged contributes nothing to the diff.
//
// DIVERGENCE (documented): Phoenix re-sends unchanged LEAF strings and so its
// vector is %{0 => "hi"}; this port additionally omits the unchanged "hi" leaf,
// yielding an empty diff when nothing changed at all. The shared, load-bearing
// semantic — unchanged nested subtrees are omitted — is what is asserted here,
// plus the stricter leaf behavior this port guarantees.
func TestParityDiffNestedUnchanged(t *testing.T) {
	r := nestedParityRendered("abc", "efg")

	// Nothing changed: no nested subtree (and, by this port's optimization, no
	// leaf) is emitted.
	if d := DiffRendered(r, r); !d.Empty() {
		t.Errorf("unchanged render must diff empty, got %#v", d)
	}

	// Only the leaf changes: the two unchanged nested children stay omitted;
	// just the leaf slot travels.
	next := nestedParityRendered("abc", "efg")
	next.Dynamics[0] = "yo"
	d := DiffRendered(r, next)
	if d["0"] != "yo" {
		t.Errorf("changed leaf = %#v, want yo", d["0"])
	}
	if _, ok := d["1"]; ok {
		t.Errorf("unchanged nested child 1 must be omitted, got %#v", d["1"])
	}
	if _, ok := d["2"]; ok {
		t.Errorf("unchanged nested child 2 must be omitted, got %#v", d["2"])
	}
}

// TestParityDiffJSONWireShape mirrors the compact JSON wire shape Phoenix uses
// for diffs: a sparse object keyed by decimal slot index. A single changed leaf
// marshals to {"0":"1"}.
func TestParityDiffJSONWireShape(t *testing.T) {
	tmpl := MustParse(`<span>{{ count }}</span>`)
	prev := tmpl.Render(map[string]any{"count": 0})
	next := tmpl.Render(map[string]any{"count": 1})
	b, err := json.Marshal(DiffRendered(prev, next))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"0":"1"}` {
		t.Errorf("diff JSON = %s, want {\"0\":\"1\"}", b)
	}
}

// parityComp is a minimal stateful component used to exercise the "c" wire key.
type parityComp struct{ id string }

func (c *parityComp) ID() string { return c.id }
func (c *parityComp) Mount(s *Socket) error {
	s.Assign("from", "WORLD")
	return nil
}
func (c *parityComp) HandleEvent(_ string, _ map[string]any, _ *Socket) error { return nil }
func (c *parityComp) Render(a map[string]any) *Rendered {
	return MustParse(`<div>FROM {{ from }}</div>`).Render(a)
}

// parityCompView embeds a single stateful component in its template, mirroring
// diff_test.exs's "with live_component": the parent slot carries only the
// component's cid, and the component's own render travels under the reserved
// "c" key keyed by that cid.
type parityCompView struct{}

func (parityCompView) Mount(_ map[string]any, s *Socket) error {
	s.Assign("child", s.LiveComponent(&parityComp{id: "c1"}))
	return nil
}
func (parityCompView) HandleEvent(_ string, _ map[string]any, _ *Socket) error { return nil }
func (parityCompView) Render(a map[string]any) *Rendered {
	return MustParse(`<main>{{ child }}</main>`).Render(a)
}

// TestParityComponentUnderCKey mirrors diff_test.exs "with live_component":
// the initial diff embeds the component's cid at the parent slot and its full
// render under "c" keyed by the same cid.
func TestParityComponentUnderCKey(t *testing.T) {
	sess := NewSession(parityCompView{})
	if _, err := sess.Mount(nil); err != nil {
		t.Fatal(err)
	}
	d := sess.InitialDiff()

	cid, ok := d["0"].(int)
	if !ok || cid <= 0 {
		t.Fatalf("parent slot must carry component cid (int > 0), got %#v", d["0"])
	}
	comps, ok := d["c"].(map[string]any)
	if !ok {
		t.Fatalf("initial diff must carry components under \"c\", got %#v", d["c"])
	}
	sub, ok := comps[itoa(cid)].(Diff)
	if !ok {
		t.Fatalf("component diff must be keyed by cid %d, got %#v", cid, comps)
	}
	if sub["0"] != "WORLD" {
		t.Errorf("component full render slot 0 = %#v, want WORLD", sub["0"])
	}
	if _, ok := sub["s"].([]string); !ok {
		t.Errorf("component full render must carry its statics under \"s\", got %#v", sub["s"])
	}
}
