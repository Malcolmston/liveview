# liveview

Phoenix LiveView-style reactive server-rendered UI for Go — **standard library only**, zero dependencies.

Server-held state drives the UI. The browser sends events, the server updates
state, re-renders, and ships back a **minimal diff** describing only what
changed. The trick is the classic LiveView *static/dynamic split*: a template is
cut into the literal parts that never change (statics) and the interpolated
values (dynamics), so unchanged HTML never travels twice.

## Install

```sh
go get github.com/malcolmston/liveview
```

Requires Go 1.24+.

## Quick start

Implement the `View` interface (`Mount`, `HandleEvent`, `Render`):

```go
package main

import (
	"log"
	"net/http"

	"github.com/malcolmston/liveview"
)

var tmpl = liveview.MustParse(
	`<div><h1>{{ label }}</h1><span class="value">{{ count }}</span></div>`,
)

type Counter struct{}

func (Counter) Mount(params map[string]any, s *liveview.Socket) error {
	s.Assign("count", 0)
	s.Assign("label", "Clicks")
	return nil
}

func (Counter) HandleEvent(event string, _ map[string]any, s *liveview.Socket) error {
	switch event {
	case "inc":
		s.Assign("count", s.GetInt("count")+1)
	case "dec":
		s.Assign("count", s.GetInt("count")-1)
	}
	return nil
}

func (Counter) Render(a map[string]any) *liveview.Rendered { return tmpl.Render(a) }

func main() {
	h := liveview.NewHandler("/", func() liveview.View { return Counter{} })
	log.Fatal(http.ListenAndServe(":8080", h))
}
```

`GET /` serves the full initial HTML page (plus a tiny inline JS stub);
`POST /event` accepts `{"session","event","payload"}` JSON and returns the diff.

## Driving the engine directly

The transport is optional — the state → render → diff core stands alone:

```go
sess := liveview.NewSession(&liveview.Counter{Start: 0})

initial, _ := sess.Mount(map[string]any{"label": "Clicks"})
// initial.HTML() == <div class="counter"><h1>Clicks</h1><span class="value">0</span></div>

diff, _ := sess.Event("inc", nil)
// diff marshals to {"1":"1"} — only the changed count slot, no statics, no label.
```

## How the diff works

Rendering yields a `*Rendered`, not a string:

```
template:  <span class="value">{{ count }}</span>
Statics:   ["<span class=\"value\">", "</span>"]   // len == len(Dynamics)+1
Dynamics:  ["0"]                                    // count == 0
```

- `FullDiff` (initial frame) emits every dynamic plus the statics under `"s"`,
  so a fresh client can build the document: `{"s":[...],"0":"0"}`.
- `DiffRendered(prev, next)` emits **only** the dynamic slots whose value
  changed. Unchanged slots and the statics are omitted. Dynamics can be nested
  `*Rendered` components; the diff recurses, so an unchanged component adds
  nothing. If the statics themselves differ (a template structure change), the
  subtree is re-sent in full.

Values are HTML-escaped by default; wrap trusted markup in `liveview.Safe`.

## API surface

| Symbol | Purpose |
| --- | --- |
| `View` | interface: `Mount` / `HandleEvent` / `Render` |
| `Socket`, `NewSocket` | server-side assigns + change tracking |
| `Template`, `Parse`, `MustParse` | `{{ name }}` templates compiled to a static/dynamic split |
| `Rendered`, `Safe` | render tree; escape opt-out |
| `Diff`, `FullDiff`, `DiffRendered` | the diff engine |
| `Session`, `NewSession` | per-connection runtime (mount → render → diff) |
| `Handler`, `NewHandler` | `http.Handler` transport (HTML page + JSON events) |
| `Counter` | example view used in tests and docs |

See `go doc github.com/malcolmston/liveview` for the full godoc.

## License

See repository.
