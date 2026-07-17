package liveview

// Counter is a minimal example [View]: a number with increment/decrement
// events and a free-text label. It exists to exercise and demonstrate the
// framework (mount -> render -> event -> diff) and is used by the package tests
// and examples.
//
// Assigns:
//
//	count  int     the current value (event "inc" / "dec" adjust it, "set" assigns payload["value"])
//	label  string  a caption echoed into the page (escaped), settable via event "label"
type Counter struct {
	// Start is the initial count applied at Mount unless params override it.
	Start int
}

// counterTmpl is compiled once; every render shares its statics, which is what
// lets the diff omit them after the first frame.
var counterTmpl = MustParse(
	`<div class="counter"><h1>{{ label }}</h1><span class="value">{{ count }}</span></div>`,
)

// Mount seeds count from params["start"] (if an int) or the Counter's Start,
// and label from params["label"].
func (c *Counter) Mount(params map[string]any, socket *Socket) error {
	start := c.Start
	if v, ok := params["start"].(int); ok {
		start = v
	}
	socket.Assign("count", start)
	label, _ := params["label"].(string)
	if label == "" {
		label = "Counter"
	}
	socket.Assign("label", label)
	return nil
}

// HandleEvent implements the "inc", "dec", "set" and "label" events.
func (c *Counter) HandleEvent(event string, payload map[string]any, socket *Socket) error {
	switch event {
	case "inc":
		socket.Assign("count", socket.GetInt("count")+1)
	case "dec":
		socket.Assign("count", socket.GetInt("count")-1)
	case "set":
		if v, ok := payload["value"].(int); ok {
			socket.Assign("count", v)
		}
	case "label":
		if v, ok := payload["text"].(string); ok {
			socket.Assign("label", v)
		}
	}
	return nil
}

// Render draws the counter using the shared template.
func (c *Counter) Render(assigns map[string]any) *Rendered {
	return counterTmpl.Render(assigns)
}
