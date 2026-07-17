package liveview_test

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/malcolmston/liveview"
)

// Example walks the full lifecycle: mount a counter, render it, dispatch an
// event, and print the minimal diff — showing that only the changed dynamic
// slot travels, not the surrounding statics.
func Example() {
	sess := liveview.NewSession(&liveview.Counter{Start: 0})

	initial, _ := sess.Mount(map[string]any{"label": "Clicks"})
	fmt.Println("initial:", initial.HTML())

	diff, _ := sess.Event("inc", nil)
	out, _ := json.Marshal(diff)
	fmt.Println("diff:", string(out))
	fmt.Println("now:", sess.Render().HTML())

	// Output:
	// initial: <div class="counter"><h1>Clicks</h1><span class="value">0</span></div>
	// diff: {"1":"1"}
	// now: <div class="counter"><h1>Clicks</h1><span class="value">1</span></div>
}

// ExampleDiffRendered shows the static/dynamic split directly: identical
// templates rendered with one differing value diff down to a single slot.
func ExampleDiffRendered() {
	tmpl := liveview.MustParse(`<span>{{ count }}</span>`)
	prev := tmpl.Render(map[string]any{"count": 41})
	next := tmpl.Render(map[string]any{"count": 42})

	diff := liveview.DiffRendered(prev, next)
	out, _ := json.Marshal(diff)
	fmt.Println(string(out))

	// Output:
	// {"0":"42"}
}

// ExampleFullDiff shows the initial patch: it carries the statics (under "s")
// so a fresh client can build the document, plus every dynamic value.
func ExampleFullDiff() {
	r := liveview.MustParse(`<b>{{ x }}</b>`).Render(map[string]any{"x": "hi"})

	// Encode with HTML escaping off so the statics read naturally in docs; the
	// wire format is ordinary JSON either way.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(liveview.FullDiff(r))
	fmt.Print(buf.String())

	// Output:
	// {"0":"hi","s":["<b>","</b>"]}
}

// ExampleSafe demonstrates HTML escaping and the Safe opt-out.
func ExampleSafe() {
	tmpl := liveview.MustParse(`<p>{{ a }}</p><p>{{ b }}</p>`)
	r := tmpl.Render(map[string]any{
		"a": `<script>alert(1)</script>`, // escaped
		"b": liveview.Safe(`<em>trusted</em>`),
	})
	fmt.Println(r.HTML())

	// Output:
	// <p>&lt;script&gt;alert(1)&lt;/script&gt;</p><p><em>trusted</em></p>
}
