package liveview

import "testing"

func TestJSCommandsKnownJSON(t *testing.T) {
	tests := []struct {
		name string
		js   *JS
		want string
	}{
		{
			name: "navigate push",
			js:   NewJS().Navigate("/next", false),
			want: `[["navigate",{"replace":false,"to":"/next"}]]`,
		},
		{
			name: "navigate replace",
			js:   NewJS().Navigate("/next", true),
			want: `[["navigate",{"replace":true,"to":"/next"}]]`,
		},
		{
			name: "patch",
			js:   NewJS().Patch("/page?tab=2", false),
			want: `[["patch",{"replace":false,"to":"/page?tab=2"}]]`,
		},
		{
			name: "remove attribute",
			js:   NewJS().RemoveAttribute("disabled", "#btn"),
			want: `[["remove_attr",{"attr":"disabled","to":"#btn"}]]`,
		},
		{
			name: "toggle attribute",
			js:   NewJS().ToggleAttribute("aria-expanded", "true", "false", "#m"),
			want: `[["toggle_attr",{"attr":"aria-expanded","to":"#m","val1":"true","val2":"false"}]]`,
		},
		{
			name: "transition",
			js:   NewJS().Transition("fade-in", "#box", 200),
			want: `[["transition",{"time":200,"to":"#box","transition":"fade-in"}]]`,
		},
		{
			name: "focus",
			js:   NewJS().Focus("#name"),
			want: `[["focus",{"to":"#name"}]]`,
		},
		{
			name: "focus first",
			js:   NewJS().FocusFirst("#form"),
			want: `[["focus_first",{"to":"#form"}]]`,
		},
		{
			name: "push focus with selector",
			js:   NewJS().PushFocus("#a"),
			want: `[["push_focus",{"to":"#a"}]]`,
		},
		{
			name: "push focus no selector",
			js:   NewJS().PushFocus(""),
			want: `[["push_focus",{}]]`,
		},
		{
			name: "pop focus",
			js:   NewJS().PopFocus(),
			want: `[["pop_focus",{}]]`,
		},
		{
			name: "exec",
			js:   NewJS().Exec("data-show", "#panel"),
			want: `[["exec",{"attr":"data-show","to":"#panel"}]]`,
		},
		{
			name: "ignore attributes",
			js:   NewJS().IgnoreAttributes([]string{"class", "style"}, "#el"),
			want: `[["ignore_attributes",{"attrs":["class","style"],"to":"#el"}]]`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.js.String(); got != tc.want {
				t.Errorf("String()\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestJSConcat(t *testing.T) {
	a := NewJS().AddClass("open", "#m")
	b := NewJS().Focus("#name").Push("shown")
	got := a.Concat(b).String()
	want := `[["add_class",{"names":"open","to":"#m"}],["focus",{"to":"#name"}],["push",{"event":"shown"}]]`
	if got != want {
		t.Errorf("Concat()\n got %s\nwant %s", got, want)
	}
	// Concat must not mutate either operand.
	if a.String() != `[["add_class",{"names":"open","to":"#m"}]]` {
		t.Errorf("Concat mutated receiver: %s", a.String())
	}
	if b.String() != `[["focus",{"to":"#name"}],["push",{"event":"shown"}]]` {
		t.Errorf("Concat mutated argument: %s", b.String())
	}
	if NewJS().Push("x").Concat(nil).String() != `[["push",{"event":"x"}]]` {
		t.Errorf("Concat(nil) should copy receiver")
	}
}

func TestJSChainPurity(t *testing.T) {
	base := NewJS().Focus("#a")
	_ = base.Navigate("/x", false)
	// base must be unchanged after deriving a new chain from it.
	if got := base.String(); got != `[["focus",{"to":"#a"}]]` {
		t.Errorf("builder step mutated receiver: %s", got)
	}
}

func BenchmarkJSChain(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewJS().
			AddClass("open", "#m").
			Transition("fade", "#m", 150).
			Focus("#first").
			Push("opened").
			String()
	}
}
