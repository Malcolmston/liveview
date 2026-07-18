package liveview

// This file extends the [JS] client-command builder toward parity with Phoenix
// LiveView's JS module. Each method returns a new JS with one command appended,
// keeping chains pure exactly like the builders in js.go. The emitted JSON is an
// array of ["op", {args}] pairs the served client interprets.

// Navigate appends a command that performs a live navigation (full remount of
// the target route) to href, mirroring Phoenix's JS.navigate. Pass replace to
// replace the current history entry instead of pushing a new one.
func (j *JS) Navigate(href string, replace bool) *JS {
	return j.with("navigate", map[string]any{"to": href, "replace": replace})
}

// Patch appends a command that performs a live patch (same view, re-runs
// HandleParams) to href, mirroring Phoenix's JS.patch. Pass replace to replace
// the current history entry instead of pushing a new one.
func (j *JS) Patch(href string, replace bool) *JS {
	return j.with("patch", map[string]any{"to": href, "replace": replace})
}

// RemoveAttribute appends a command that removes attr from the selector,
// mirroring Phoenix's JS.remove_attribute.
func (j *JS) RemoveAttribute(attr, selector string) *JS {
	return j.with("remove_attr", map[string]any{"attr": attr, "to": selector})
}

// ToggleAttribute appends a command that toggles attr between val1 and val2 on
// the selector (setting it to val1 when absent or equal to val2, otherwise
// val2), mirroring Phoenix's JS.toggle_attribute.
func (j *JS) ToggleAttribute(attr, val1, val2, selector string) *JS {
	return j.with("toggle_attr", map[string]any{
		"attr": attr, "val1": val1, "val2": val2, "to": selector,
	})
}

// Transition appends a command that applies the space-separated transition
// classes to the selector for the given duration in milliseconds, mirroring
// Phoenix's JS.transition. A time of 0 uses the client default.
func (j *JS) Transition(transition, selector string, timeMS int) *JS {
	return j.with("transition", map[string]any{
		"transition": transition, "to": selector, "time": timeMS,
	})
}

// Focus appends a command that moves keyboard focus to the selector, mirroring
// Phoenix's JS.focus.
func (j *JS) Focus(selector string) *JS {
	return j.with("focus", map[string]any{"to": selector})
}

// FocusFirst appends a command that focuses the first focusable child of the
// selector, mirroring Phoenix's JS.focus_first.
func (j *JS) FocusFirst(selector string) *JS {
	return j.with("focus_first", map[string]any{"to": selector})
}

// PushFocus appends a command that stores the currently focused element (or the
// selector's, when non-empty) on a stack so it can later be restored with
// [JS.PopFocus], mirroring Phoenix's JS.push_focus.
func (j *JS) PushFocus(selector string) *JS {
	args := map[string]any{}
	if selector != "" {
		args["to"] = selector
	}
	return j.with("push_focus", args)
}

// PopFocus appends a command that restores focus to the element saved by the
// most recent [JS.PushFocus], mirroring Phoenix's JS.pop_focus.
func (j *JS) PopFocus() *JS {
	return j.with("pop_focus", map[string]any{})
}

// Exec appends a command that executes the JS commands encoded in the named
// attribute of the selector, mirroring Phoenix's JS.exec. It lets one element
// trigger the command chain declared on another.
func (j *JS) Exec(attr, selector string) *JS {
	return j.with("exec", map[string]any{"attr": attr, "to": selector})
}

// IgnoreAttributes appends a command instructing the client DOM patcher to leave
// the named attributes of the selector untouched during updates, mirroring
// Phoenix's JS.ignore_attributes.
func (j *JS) IgnoreAttributes(attrs []string, selector string) *JS {
	names := make([]any, len(attrs))
	for i, a := range attrs {
		names[i] = a
	}
	return j.with("ignore_attributes", map[string]any{"attrs": names, "to": selector})
}

// Concat returns a new JS whose command list is this chain's commands followed
// by other's, letting reusable command fragments be composed. Neither receiver
// is mutated. A nil other yields a copy of j.
func (j *JS) Concat(other *JS) *JS {
	total := len(j.cmds)
	if other != nil {
		total += len(other.cmds)
	}
	next := make([]jsCmd, 0, total)
	next = append(next, j.cmds...)
	if other != nil {
		next = append(next, other.cmds...)
	}
	return &JS{cmds: next}
}
