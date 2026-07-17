// Library content for the liveview documentation site. Mirrors the shape used by
// the malcolmston/go landing site's data.ts so the sibling sites stay in sync.
export interface Lib {
  id: string; name: string; icon: string; accent: string; pkg: string; node: string;
  repo: string; docs: string; tagline: string; blurb: string; tags: string[];
  features: string[]; node_code: string; go_code: string; integrate: string;
}

export const NODE_ACCENT = '#8cc84b';

export const LIVEVIEW: Lib = {
  id:"liveview", name:"LiveView", icon:'<i class="fa-solid fa-bolt"></i>', accent:"#fd4f00",
  pkg:"github.com/malcolmston/liveview", node:"phoenixframework/phoenix_live_view",
  repo:"https://github.com/malcolmston/liveview", docs:"https://malcolmston.github.io/liveview/",
  tagline:"Phoenix LiveView-style reactive server-rendered UI for Go.",
  blurb:"A from-scratch, standard-library-only Go take on Phoenix LiveView: server-held state drives the "+
    "UI, the browser sends events, and the server ships back a minimal diff describing only what changed. "+
    "The core trick is LiveView's static/dynamic split — a template is compiled once into the literal "+
    "fragments that never change and the interpolated values that do, so unchanged HTML never travels "+
    "twice. You implement the View interface (Mount / HandleEvent / Render), keep per-connection state in "+
    "a Socket with per-key change tracking, and let a Session run the mount → render → diff cycle; a tiny "+
    "net/http Handler adds an optional HTTP + JSON transport on top. No cgo, no third-party dependencies — "+
    "the import path and package are both liveview, and diffs marshal to the same compact JSON shape "+
    "Phoenix LiveView uses.",
  tags:["View lifecycle","Socket assigns","change tracking","static/dynamic split","minimal diffs","Rendered tree","Session runtime","net/http Handler"],
  features:[
    "The <code>View</code> lifecycle — <code>Mount</code> seeds state, <code>HandleEvent</code> reacts to a client event, <code>Render</code> returns a static/dynamic tree",
    "Server-held state in a <code>Socket</code> — <code>Assign</code>/<code>AssignAll</code> writes, <code>GetInt</code>/<code>GetString</code>/<code>Get</code> reads, with per-key <code>Changed</code> tracking",
    "Tiny <code>{{ name }}</code> templates compiled once via <code>MustParse</code> / <code>Parse</code> into a fixed static/dynamic <code>Template</code>",
    "A <code>Rendered</code> tree, not a string — <code>Statics</code> + <code>Dynamics</code> with the <code>len(Statics) == len(Dynamics)+1</code> invariant, and <code>HTML()</code> to materialise it",
    "Minimal-patch diff engine — <code>DiffRendered</code> emits only the changed dynamic slots (recursing into nested components), <code>FullDiff</code> sends the first frame with statics under <code>\"s\"</code>",
    "Auto HTML-escaping by default, with <code>Safe</code> to opt trusted markup out of escaping",
    "A per-connection <code>Session</code> (<code>NewSession</code>) that drives mount → render → diff; <code>Session.Event</code> returns just the <code>Diff</code>",
    "An optional <code>net/http</code> transport — <code>NewHandler</code> serves the initial HTML page and accepts JSON events, so the state → render → diff core stays independent of the wire",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`defmodule CounterLive do
  use Phoenix.LiveView

  def mount(_params, _session, socket) do
    {:ok, assign(socket, count: 0, label: "Clicks")}
  end

  def handle_event("inc", _params, socket) do
    {:noreply, update(socket, :count, &(&1 + 1))}
  end

  def render(assigns) do
    ~H"""
    <div><h1><%= @label %></h1><span class="value"><%= @count %></span></div>
    """
  end
end`,
  go_code:
`import "github.com/malcolmston/liveview"

var tmpl = liveview.MustParse(
	` + "`<div><h1>{{ label }}</h1><span class=\"value\">{{ count }}</span></div>`" + `)

type Counter struct{}

func (Counter) Mount(_ map[string]any, s *liveview.Socket) error {
	s.Assign("count", 0)
	s.Assign("label", "Clicks")
	return nil
}

func (Counter) HandleEvent(e string, _ map[string]any, s *liveview.Socket) error {
	if e == "inc" {
		s.Assign("count", s.GetInt("count")+1)
	}
	return nil
}

func (Counter) Render(a map[string]any) *liveview.Rendered { return tmpl.Render(a) }`,
  integrate:
`<span class="tok-c">// The state → render → diff core stands alone — no HTTP required.</span>
sess := liveview.NewSession(&liveview.Counter{Start: 0})

<span class="tok-c">// Mount runs Mount + the first Render and caches it; HTML() is the full page.</span>
initial, _ := sess.Mount(map[string]any{"label": "Clicks"})
_ = initial.HTML() <span class="tok-c">// <div class="counter"><h1>Clicks</h1><span class="value">0</span></div></span>

<span class="tok-c">// An event re-renders, diffs against the previous frame, and returns only</span>
<span class="tok-c">// the changed dynamic slots. Here just the count slot moves.</span>
diff, _ := sess.Event("inc", nil)
<span class="tok-c">// diff marshals to {"1":"1"} — statics and the unchanged label are omitted.</span>

<span class="tok-c">// The very first frame is a FullDiff: every dynamic plus the statics under "s",</span>
<span class="tok-c">// so a fresh client can rebuild the document: {"s":[...],"0":"Clicks","1":"0"}.</span>
full := liveview.FullDiff(sess.Render())

<span class="tok-c">// Mount the same View over plain HTTP + JSON when you want a transport.</span>
h := liveview.NewHandler("/", func() liveview.View { return &liveview.Counter{} })
_ = diff
_ = full
_ = h`
};
