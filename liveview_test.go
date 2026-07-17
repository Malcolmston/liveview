package liveview

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestTemplateParseAndRender(t *testing.T) {
	tmpl, err := Parse(`Hi {{ name }}, you have {{ n }} msgs`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	wantStatics := []string{"Hi ", ", you have ", " msgs"}
	if !reflect.DeepEqual(tmpl.statics, wantStatics) {
		t.Fatalf("statics = %#v, want %#v", tmpl.statics, wantStatics)
	}
	r := tmpl.Render(map[string]any{"name": "Ann", "n": 3})
	if got := r.HTML(); got != "Hi Ann, you have 3 msgs" {
		t.Fatalf("html = %q", got)
	}
	if len(r.Statics) != len(r.Dynamics)+1 {
		t.Fatalf("invariant violated: %d statics, %d dynamics", len(r.Statics), len(r.Dynamics))
	}
}

func TestTemplateEscaping(t *testing.T) {
	tmpl := MustParse(`<p>{{ body }}</p>`)
	r := tmpl.Render(map[string]any{"body": `<script>"x"&y</script>`})
	got := r.HTML()
	if strings.Contains(got, "<script>") {
		t.Fatalf("unescaped output: %q", got)
	}
	want := "<p>&lt;script&gt;&#34;x&#34;&amp;y&lt;/script&gt;</p>"
	if got != want {
		t.Fatalf("html = %q, want %q", got, want)
	}
}

func TestSafeBypassesEscaping(t *testing.T) {
	tmpl := MustParse(`<div>{{ body }}</div>`)
	r := tmpl.Render(map[string]any{"body": Safe("<b>ok</b>")})
	if got := r.HTML(); got != "<div><b>ok</b></div>" {
		t.Fatalf("html = %q", got)
	}
}

func TestTemplateMissingAssignIsEmpty(t *testing.T) {
	tmpl := MustParse(`[{{ a }}]`)
	if got := tmpl.Render(map[string]any{}).HTML(); got != "[]" {
		t.Fatalf("html = %q, want []", got)
	}
}

func TestTemplateLiteralBraces(t *testing.T) {
	tmpl := MustParse(`{{{{ literal }}`)
	if got := tmpl.Render(nil).HTML(); got != "{{ literal }}" {
		t.Fatalf("html = %q", got)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse(`oops {{ unclosed`); err == nil {
		t.Fatal("expected unterminated error")
	}
	if _, err := Parse(`empty {{  }} slot`); err == nil {
		t.Fatal("expected empty slot error")
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	MustParse(`{{ oops`)
}

func TestToStringVariants(t *testing.T) {
	tmpl := MustParse(`{{ v }}`)
	cases := []struct {
		v    any
		want string
	}{
		{nil, ""},
		{"s", "s"},
		{true, "true"},
		{42, "42"},
		{int64(64), "64"},
		{3.5, "3.5"},
		{stringer{}, "STR"},
		{[]int{1, 2}, "[1 2]"},
	}
	for _, c := range cases {
		if got := tmpl.Render(map[string]any{"v": c.v}).HTML(); got != c.want {
			t.Errorf("value %#v -> %q, want %q", c.v, got, c.want)
		}
	}
}

type stringer struct{}

func (stringer) String() string { return "STR" }

func TestSocket(t *testing.T) {
	s := NewSocket()
	s.Assign("a", 1)
	s.Assign("b", "x")
	if !s.Changed("a") || !s.AnyChanged() {
		t.Fatal("expected changes tracked")
	}
	if v, ok := s.Get("a"); !ok || v != 1 {
		t.Fatalf("Get a = %v,%v", v, ok)
	}
	if s.GetInt("a") != 1 || s.GetString("b") != "x" {
		t.Fatal("typed getters wrong")
	}
	if s.GetInt("missing") != 0 || s.GetString("missing") != "" {
		t.Fatal("missing typed getters should be zero values")
	}
	if s.GetInt("b") != 0 {
		t.Fatal("GetInt on non-int should be 0")
	}
	s.AssignAll(map[string]any{"c": 3})
	snap := s.Assigns()
	snap["a"] = 999 // must not affect socket
	if v, _ := s.Get("a"); v != 1 {
		t.Fatal("Assigns snapshot must be a copy")
	}
	s.ResetChanges()
	if s.AnyChanged() || s.Changed("a") {
		t.Fatal("changes should be cleared")
	}
	s.ResetChanges() // no-op path
}

// --- Diff engine: the core deliverable ---

func TestFullDiffIncludesStatics(t *testing.T) {
	r := MustParse(`<b>{{ x }}</b>`).Render(map[string]any{"x": "hi"})
	d := FullDiff(r)
	if _, ok := d[staticsKey]; !ok {
		t.Fatal("full diff must include statics under 's'")
	}
	if d["0"] != "hi" {
		t.Fatalf("dynamic 0 = %v", d["0"])
	}
}

func TestDiffOnlyChangedDynamics(t *testing.T) {
	tmpl := MustParse(`<h1>{{ label }}</h1><span>{{ count }}</span>`)
	prev := tmpl.Render(map[string]any{"label": "Counter", "count": 0})
	next := tmpl.Render(map[string]any{"label": "Counter", "count": 1})

	d := DiffRendered(prev, next)

	// Only slot 1 (count) changed; slot 0 (label) and statics must be absent.
	if _, ok := d[staticsKey]; ok {
		t.Fatal("incremental diff must NOT re-send statics")
	}
	if _, ok := d["0"]; ok {
		t.Fatal("unchanged label slot 0 must be omitted from diff")
	}
	v, ok := d["1"]
	if !ok || v != "1" {
		t.Fatalf("changed count slot 1 = %v, ok=%v; want \"1\"", v, ok)
	}
	if len(d) != 1 {
		t.Fatalf("diff should contain exactly one changed slot, got %#v", d)
	}
}

func TestDiffNoChangeIsEmpty(t *testing.T) {
	tmpl := MustParse(`<span>{{ count }}</span>`)
	a := tmpl.Render(map[string]any{"count": 5})
	b := tmpl.Render(map[string]any{"count": 5})
	d := DiffRendered(a, b)
	if !d.Empty() {
		t.Fatalf("identical renders should diff empty, got %#v", d)
	}
}

func TestDiffNilPrevIsFull(t *testing.T) {
	r := MustParse(`{{ a }}{{ b }}`).Render(map[string]any{"a": "1", "b": "2"})
	d := DiffRendered(nil, r)
	if _, ok := d[staticsKey]; !ok {
		t.Fatal("diff against nil prev should be a full diff with statics")
	}
	if d["0"] != "1" || d["1"] != "2" {
		t.Fatalf("full diff dynamics wrong: %#v", d)
	}
}

func TestDiffNilNextEmpty(t *testing.T) {
	if !DiffRendered(nil, nil).Empty() {
		t.Fatal("nil next should be empty diff")
	}
}

func TestFullDiffNil(t *testing.T) {
	if !FullDiff(nil).Empty() {
		t.Fatal("FullDiff(nil) should be empty")
	}
}

func TestRenderedNilHTML(t *testing.T) {
	var r *Rendered
	if r.HTML() != "" {
		t.Fatal("nil Rendered HTML should be empty")
	}
}

func TestDiffShorterPrevDynamics(t *testing.T) {
	// prev has fewer dynamics than next but identical statics length mismatch
	// is guarded; here we force equal statics with next having an extra slot
	// value read against a missing prev index.
	statics := []string{"a", "b"}
	prev := &Rendered{Statics: statics, Dynamics: []any{"x"}}
	next := &Rendered{Statics: statics, Dynamics: []any{"y"}}
	d := DiffRendered(prev, next)
	if d["0"] != "y" {
		t.Fatalf("diff = %#v", d)
	}
}

func TestDiffNestedPrevNotRendered(t *testing.T) {
	// prev slot is a string, next slot is a *Rendered: recurse with nil prev,
	// producing a full sub-diff.
	statics := []string{"<p>", "</p>"}
	child := MustParse(`<i>{{ v }}</i>`).Render(map[string]any{"v": "a"})
	prev := &Rendered{Statics: statics, Dynamics: []any{"plain"}}
	next := &Rendered{Statics: statics, Dynamics: []any{child}}
	d := DiffRendered(prev, next)
	if _, ok := d["0"].(Diff); !ok {
		t.Fatalf("expected nested full diff, got %#v", d["0"])
	}
}

func TestDiffStructuralChangeResendsStatics(t *testing.T) {
	a := MustParse(`<a>{{ x }}</a>`).Render(map[string]any{"x": "1"})
	b := MustParse(`<b>{{ x }}</b>`).Render(map[string]any{"x": "1"})
	d := DiffRendered(a, b)
	if _, ok := d[staticsKey]; !ok {
		t.Fatal("changed statics must force a full re-send")
	}
}

func TestDiffNestedComponents(t *testing.T) {
	// Build parent renders with a nested child *Rendered in a dynamic slot.
	child := MustParse(`<i>{{ v }}</i>`)
	parentStatics := []string{"<p>", "</p>"}

	prev := &Rendered{Statics: parentStatics, Dynamics: []any{
		child.Render(map[string]any{"v": "a"}),
	}}
	next := &Rendered{Statics: parentStatics, Dynamics: []any{
		child.Render(map[string]any{"v": "b"}),
	}}
	d := DiffRendered(prev, next)
	sub, ok := d["0"].(Diff)
	if !ok {
		t.Fatalf("expected nested Diff at slot 0, got %#v", d["0"])
	}
	if sub["0"] != "b" {
		t.Fatalf("nested diff = %#v", sub)
	}

	// Unchanged nested component contributes nothing.
	same := DiffRendered(prev, prev)
	if !same.Empty() {
		t.Fatalf("unchanged nested component must diff empty, got %#v", same)
	}

	// Full diff recurses into nested component.
	full := FullDiff(next)
	if _, ok := full["0"].(Diff); !ok {
		t.Fatalf("full diff should nest child diff, got %#v", full["0"])
	}
}

func TestDiffJSONShape(t *testing.T) {
	tmpl := MustParse(`<span>{{ count }}</span>`)
	prev := tmpl.Render(map[string]any{"count": 0})
	next := tmpl.Render(map[string]any{"count": 1})
	d := DiffRendered(prev, next)
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"0":"1"}` {
		t.Fatalf("diff json = %s, want {\"0\":\"1\"}", b)
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 5: "5", 31: "31", 32: "32", 100: "100", -7: "-7"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

// --- Session lifecycle ---

func TestSessionMountRenderEventDiff(t *testing.T) {
	sess := NewSession(&Counter{Start: 0})
	initial, err := sess.Mount(map[string]any{"label": "Hits"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initial.HTML(), "<h1>Hits</h1>") {
		t.Fatalf("initial render missing label: %q", initial.HTML())
	}
	if !strings.Contains(initial.HTML(), `<span class="value">0</span>`) {
		t.Fatalf("initial render missing count: %q", initial.HTML())
	}

	// Dispatch inc; assigns must update and diff must contain only the count.
	diff, err := sess.Event("inc", nil)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := sess.Socket().Get("count"); v != 1 {
		t.Fatalf("HandleEvent did not update assigns: count=%v", v)
	}
	if len(diff) != 1 || diff["1"] != "1" {
		t.Fatalf("diff should carry only changed count slot, got %#v", diff)
	}
	if !strings.Contains(sess.Render().HTML(), `value">1</span>`) {
		t.Fatalf("re-render wrong: %q", sess.Render().HTML())
	}

	// dec back to 0.
	diff, _ = sess.Event("dec", nil)
	if diff["1"] != "0" {
		t.Fatalf("dec diff = %#v", diff)
	}

	// set via payload.
	diff, _ = sess.Event("set", map[string]any{"value": 9})
	if diff["1"] != "9" {
		t.Fatalf("set diff = %#v", diff)
	}

	// label event changes slot 0, not slot 1.
	diff, _ = sess.Event("label", map[string]any{"text": "Total"})
	if diff["0"] != "Total" {
		t.Fatalf("label diff = %#v", diff)
	}
	if _, ok := diff["1"]; ok {
		t.Fatal("count slot must be absent when only label changed")
	}

	// unknown event changes nothing.
	diff, _ = sess.Event("noop", nil)
	if !diff.Empty() {
		t.Fatalf("noop should diff empty, got %#v", diff)
	}
}

func TestSessionMountStartParam(t *testing.T) {
	sess := NewSession(&Counter{Start: 3})
	if _, err := sess.Mount(map[string]any{"start": 10}); err != nil {
		t.Fatal(err)
	}
	if sess.Socket().GetInt("count") != 10 {
		t.Fatalf("start param ignored: %d", sess.Socket().GetInt("count"))
	}

	sess2 := NewSession(&Counter{Start: 3})
	if _, err := sess2.Mount(nil); err != nil {
		t.Fatal(err)
	}
	if sess2.Socket().GetInt("count") != 3 {
		t.Fatalf("default Start not applied: %d", sess2.Socket().GetInt("count"))
	}
}

type errView struct{ mountErr, eventErr bool }

func (e *errView) Mount(_ map[string]any, _ *Socket) error {
	if e.mountErr {
		return errBoom
	}
	return nil
}
func (e *errView) HandleEvent(_ string, _ map[string]any, _ *Socket) error {
	if e.eventErr {
		return errBoom
	}
	return nil
}
func (e *errView) Render(_ map[string]any) *Rendered { return MustParse(`ok`).Render(nil) }

var errBoom = boomError("boom")

type boomError string

func (b boomError) Error() string { return string(b) }

func TestSessionErrors(t *testing.T) {
	if _, err := NewSession(&errView{mountErr: true}).Mount(nil); err == nil {
		t.Fatal("expected mount error")
	}
	sess := NewSession(&errView{eventErr: true})
	if _, err := sess.Mount(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Event("x", nil); err == nil {
		t.Fatal("expected event error")
	}
	if sess.ID() != "" {
		t.Fatal("unset id should be empty")
	}
}

// --- HTTP handler ---

func newTestHandler() *Handler {
	return NewHandler("/counter", func() View { return &Counter{Start: 0} })
}

func TestHandlerServesPage(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/counter/?label=Visits", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<!doctype html>") {
		t.Fatal("missing doctype")
	}
	if !strings.Contains(body, "<h1>Visits</h1>") {
		t.Fatalf("query param not mounted: %s", body)
	}
	if !strings.Contains(body, "data-session=") {
		t.Fatal("missing session id")
	}
	if !strings.Contains(body, "/counter/event") {
		t.Fatal("client JS should target the event route")
	}
}

func TestHandlerEventRoundTrip(t *testing.T) {
	h := newTestHandler()

	// Mount a page to create a session.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/counter/", nil))
	id := extractSession(t, rec.Body.String())
	if h.Session(id) == nil {
		t.Fatal("session not stored")
	}

	// Post an inc event.
	reqBody, _ := json.Marshal(eventRequest{Session: id, Event: "inc"})
	post := httptest.NewRequest(http.MethodPost, "/counter/event", bytes.NewReader(reqBody))
	prec := httptest.NewRecorder()
	h.ServeHTTP(prec, post)
	if prec.Code != http.StatusOK {
		t.Fatalf("event status %d: %s", prec.Code, prec.Body.String())
	}
	var diff map[string]any
	if err := json.Unmarshal(prec.Body.Bytes(), &diff); err != nil {
		t.Fatal(err)
	}
	if diff["1"] != "1" {
		t.Fatalf("event diff = %#v", diff)
	}
	if _, ok := diff["s"]; ok {
		t.Fatal("incremental event response must not include statics")
	}
}

func TestHandlerEventErrors(t *testing.T) {
	h := newTestHandler()

	// Bad JSON.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/counter/event", strings.NewReader("{")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json status %d", rec.Code)
	}

	// Unknown session.
	body, _ := json.Marshal(eventRequest{Session: "nope", Event: "inc"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/counter/event", bytes.NewReader(body)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown session status %d", rec.Code)
	}
}

func TestHandlerMountError(t *testing.T) {
	h := NewHandler("", func() View { return &errView{mountErr: true} })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandlerEventViewError(t *testing.T) {
	h := NewHandler("/", func() View { return &errView{eventErr: true} })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	id := extractSession(t, rec.Body.String())
	body, _ := json.Marshal(eventRequest{Session: id, Event: "x"})
	prec := httptest.NewRecorder()
	h.ServeHTTP(prec, httptest.NewRequest(http.MethodPost, "/event", bytes.NewReader(body)))
	if prec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", prec.Code)
	}
}

func TestNewHandlerDefaultPrefix(t *testing.T) {
	h := NewHandler("", func() View { return &Counter{} })
	if h.prefix != "/" {
		t.Fatalf("empty prefix should default to /, got %q", h.prefix)
	}
}

func extractSession(t *testing.T, body string) string {
	t.Helper()
	const marker = `data-session="`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("no session marker in page")
	}
	rest := body[i+len(marker):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		t.Fatal("malformed session marker")
	}
	return rest[:j]
}

func TestNewIDUnique(t *testing.T) {
	a, b := newID(), newID()
	if a == b || len(a) != 32 {
		t.Fatalf("ids not unique/wrong length: %q %q", a, b)
	}
}
