# plugin-contracts.api-surface

Source feature folder for `plugin-contracts.api-surface`.

## SDK import path

The Go SDK package for this source release is imported from the repository's
local module path:

```go
import plugin "rpc_plugin_system/plugin-authoring-sdk/go-runtime/plugin"
```

This is an in-repo module path for builds run from `src/`. It is not a
published external module path and is not intended to be fetched with `go get`.

## Capability vocabulary

The current code emits these capability strings:

- `heartbeat` — required core health check RPC support.
- `shutdown` — required graceful shutdown RPC support.
- `echo` — optional echo RPC support.
- `sleep` — optional sleep RPC support for timeout/failure exercising.
- `crash` — optional crash RPC support for supervision/failure exercising.

The SDK detects `heartbeat` and `shutdown` from the required core plugin
interface. It appends `echo`, `sleep`, and `crash` when the plugin implements
the corresponding optional interfaces. A plugin can override detection by
implementing `Capabilities() []string`; the failure-test plugin uses that hook
to expose normal, empty, or invalid capability responses for kernel tests.
