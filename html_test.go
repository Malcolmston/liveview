package liveview

import "testing"

func TestClassList(t *testing.T) {
	tests := []struct {
		name    string
		classes map[string]bool
		want    string
	}{
		{"none", map[string]bool{"a": false}, ""},
		{"sorted", map[string]bool{"btn": true, "active": true}, "active btn"},
		{"filtered", map[string]bool{"on": true, "off": false, "": true}, "on"},
		{"empty map", map[string]bool{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassList(tc.classes); got != tc.want {
				t.Errorf("ClassList=%q want %q", got, tc.want)
			}
		})
	}
}

func TestAttrList(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]any
		want  string
	}{
		{"empty", map[string]any{}, ""},
		{"sorted values", map[string]any{"id": "x", "class": "c"}, ` class="c" id="x"`},
		{"bool true", map[string]any{"disabled": true}, ` disabled`},
		{"bool false omitted", map[string]any{"disabled": false, "id": "x"}, ` id="x"`},
		{"escaped value", map[string]any{"title": `a"b`}, ` title="a&#34;b"`},
		{"int value", map[string]any{"tabindex": 2}, ` tabindex="2"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(AttrList(tc.attrs)); got != tc.want {
				t.Errorf("AttrList=%q want %q", got, tc.want)
			}
		})
	}
}

func TestHiddenInputs(t *testing.T) {
	got := string(HiddenInputs(map[string]string{"_csrf": "tok", "id": "7"}))
	want := `<input type="hidden" name="_csrf" value="tok"><input type="hidden" name="id" value="7">`
	if got != want {
		t.Errorf("HiddenInputs\n got %s\nwant %s", got, want)
	}
	if string(HiddenInputs(map[string]string{})) != "" {
		t.Error("empty HiddenInputs should be empty")
	}
	esc := string(HiddenInputs(map[string]string{"x": `"><script>`}))
	want2 := `<input type="hidden" name="x" value="&#34;&gt;&lt;script&gt;">`
	if esc != want2 {
		t.Errorf("HiddenInputs escaping\n got %s\nwant %s", esc, want2)
	}
}

func TestLiveLinks(t *testing.T) {
	if got := string(LivePatch("Next", "/p?tab=2")); got != `<a href="/p?tab=2" data-phx-link="patch" data-phx-link-state="push">Next</a>` {
		t.Errorf("LivePatch=%s", got)
	}
	if got := string(LiveNavigate("Home", "/")); got != `<a href="/" data-phx-link="redirect" data-phx-link-state="push">Home</a>` {
		t.Errorf("LiveNavigate=%s", got)
	}
	// Escaping of href and text.
	got := string(LivePatch(`<b>`, `/a?x="1"&y=2`))
	want := `<a href="/a?x=&#34;1&#34;&amp;y=2" data-phx-link="patch" data-phx-link-state="push">&lt;b&gt;</a>`
	if got != want {
		t.Errorf("LivePatch escaping\n got %s\nwant %s", got, want)
	}
}
