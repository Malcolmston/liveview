# Changelog

All notable changes to this project are documented here.

## 0.3.0

Adds standard-library-only functionality moving the port closer to Phoenix
LiveView parity. All additions are pure Go stdlib, deterministic, and covered by
known-answer table tests.

### Added

- **JS client commands** (`js_commands.go`) toward parity with Phoenix's `JS`
  module: `JS.Navigate`, `JS.Patch`, `JS.RemoveAttribute`, `JS.ToggleAttribute`,
  `JS.Transition`, `JS.Focus`, `JS.FocusFirst`, `JS.PushFocus`, `JS.PopFocus`,
  `JS.Exec`, `JS.IgnoreAttributes`, and `JS.Concat` for composing command
  fragments. Each returns a new immutable chain and marshals to the same
  `["op", {args}]` JSON shape as the existing commands.
- **Flash messages** (`flash.go`), the analog of Phoenix's flash: the `Flash`
  type (`NewFlash`, `Put`, `Get`, `Has`, `Delete`, `Clear`, `Kinds`, `Merge`)
  plus socket helpers `PutFlash`, `GetFlash`, `ClearFlash`, and `SocketFlash`.
  Flash rides along in assigns under a reserved key and is diff-tracked.
- **Form handling** (`form.go`): `DecodeForm` / `DecodeFormString` decode
  bracket-encoded submissions (`user[address][city]`, `tags[]`) into nested
  maps, matching Phoenix's form parameter decoding. The `Form` type
  (`NewForm`, `Get`, `GetString`, `AddError`, `Errors`, `HasErrors`, `Valid`,
  `InputName`) pairs params with per-field validation errors for phx-change
  style validation.
- **HTML helpers** (`html.go`) mirroring Phoenix.Component helpers: `ClassList`
  (conditional, deterministically sorted class strings), `AttrList`
  (escaped attribute rendering with boolean attributes), `HiddenInputs`, and the
  `LivePatch` / `LiveNavigate` link builders.

### Notes

Exported API surface grew from 123 to 164 symbols (+41). Remaining gaps toward
full LiveView parity include async assigns (`assign_async`/`start_async`),
temporary assigns, and nested `live_render`.
