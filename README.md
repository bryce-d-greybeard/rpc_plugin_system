# rpc_plugin_system

`rpc_plugin_system` is a local Go project for supervising executable plugins over Unix domain sockets using Go `net/rpc`.

The current v0.1.0 slice is a kernel-first substrate that proves:
- one supervised plugin process at a time
- Unix socket RPC transport
- one-time bootstrap token trust on startup
- heartbeat and health reporting
- timeout handling and poisoned-client teardown
- restart supervision with generation tracking
- verbose first-class append-only event logging
- a small admin/control CLI

Current project direction:
- v0.1.0 is the proven kernel substrate
- the next milestone is plugin system v1.0
- memory-system work should begin only after the plugin substrate reaches v1.0

## Project layout

Main entrypoints:
- daemon: `cmd/rpcplugind/main.go`
- control CLI: `cmd/rpcpluginctl/main.go`
- example plugin: `cmd/rpcplugin-echo/main.go`
- failure-behavior plugin: `cmd/rpcplugin-failure/main.go`

Important internals:
- `internal/kernel` - supervisor and monitor loop
- `internal/adminrpc` - local admin socket API
- `internal/auth` - one-time token bootstrap helpers
- `internal/runtime` - runtime-dir and Unix socket helpers
- `sdk/go/plugin` - public Go plugin authoring SDK
- `internal/testpluginapi` - test-only env helpers and compatibility shims

## Build

From the project root:

```bash
go build ./...
```

Build the main binaries explicitly:

```bash
go build -o .tmp-bin/rpcplugind ./cmd/rpcplugind
go build -o .tmp-bin/rpcpluginctl ./cmd/rpcpluginctl
go build -o .tmp-bin/rpcplugin-echo ./cmd/rpcplugin-echo
go build -o .tmp-bin/rpcplugin-failure ./cmd/rpcplugin-failure
```

Or use the Makefile:

```bash
make build
```

## Test

Run the full test suite:

```bash
go test ./...
```

Run the kernel-focused suite:

```bash
go test ./internal/kernel/...
```

Or use:

```bash
make test
```

## Plugin authoring entrypoint

Plugin authors should start from:

```go
import plugin "rpc_plugin_system/sdk/go/plugin"
```

That SDK is the intended public Go authoring surface. It provides:
- bootstrap config loading from env
- one-time token auth handling
- RPC service/method constants
- request/response types
- adapter-based optional capability registration
- `TemplatePlugin` as the stable minimal skeleton
- `Serve` / `ServeWithConfig` helpers

For the supported authoring path, start with:
- `sdk/go/plugin/template.go`
- `sdk/go/plugin/example_minimal.go`
- `cmd/rpcplugin-echo/main.go` for the minimal template extended with optional capabilities

The intended rule is simple:
- plugin authors should not need to read internal packages to get a basic plugin running

## Minimal plugin shape

A minimal Go plugin needs to implement only the required core methods:
- `Version() string`
- `Heartbeat(plugin.Empty, *plugin.HeartbeatResponse) error`
- `Shutdown(plugin.Empty, *plugin.Empty) error`

Optional capabilities like `Echo`, `Sleep`, and `Crash` are added by implementing the matching optional interfaces.

The stable minimal authoring path is:

```go
cfg, err := plugin.LoadConfigFromEnv()
if err != nil {
	panic(err)
}

p := plugin.NewTemplate(cfg, "0.1.0")

if err := plugin.ServeWithConfig(cfg, p); err != nil {
	panic(err)
}
```

If you need custom heartbeat data or optional capabilities, copy the shape from `sdk/go/plugin/template.go` into your own `main` package and extend it there. The public echo plugin now does exactly that by embedding `TemplatePlugin` and adding `Echo`, `Sleep`, and `Crash`.

### First plugin in one file

A minimal standalone plugin `main.go` can look like this:

```go
package main

import plugin "rpc_plugin_system/sdk/go/plugin"

func main() {
	cfg, err := plugin.LoadConfigFromEnv()
	if err != nil {
		panic(err)
	}

	p := plugin.NewTemplate(cfg, "0.1.0")

	if err := plugin.ServeWithConfig(cfg, p); err != nil {
		panic(err)
	}
}
```

That is the shortest supported public authoring path.

## Tutorial: run the system locally

This walkthrough shows the normal happy-path flow using the example echo plugin.

### Step 1: build the binaries

```bash
cd /tank/development/rpc_plugin_system
make build
```

That produces:
- `.tmp-bin/rpcplugind`
- `.tmp-bin/rpcpluginctl`
- `.tmp-bin/rpcplugin-echo`
- `.tmp-bin/rpcplugin-failure`

### Step 1b: understand the public plugin startup contract

A plugin launched by the kernel receives these required environment variables:
- `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
- `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
- `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`

If one is missing or malformed, `plugin.LoadConfigFromEnv()` now fails with an explicit author-facing startup error naming the missing variable.

Examples:
- missing socket env -> `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET is required for plugin startup`
- bad generation env -> parse error naming `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- unreadable auth token file -> read error naming `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`

### Step 2: start the daemon with the example plugin

In one terminal:

```bash
cd /tank/development/rpc_plugin_system
.tmp-bin/rpcplugind \
  -runtime-dir /tmp/rpc_plugin_system-demo \
  -plugin ./.tmp-bin/rpcplugin-echo \
  -plugin-id echo
```

What this does:
- creates a runtime directory under `/tmp/rpc_plugin_system-demo`
- creates a one-time bootstrap token for this generation
- launches the plugin executable
- authenticates the plugin through the one-time token bootstrap
- opens the admin socket for local control
- starts the monitor loop

If startup succeeds, the daemon stays running in the foreground.

### Step 3: inspect status from a second terminal

Open another terminal and run:

```bash
cd /tank/development/rpc_plugin_system
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo status
```

You should get JSON showing fields like:
- plugin id
- generation id
- health
- socket path
- pid

Healthy output means:
- the plugin started
- auth succeeded
- capabilities were read
- the kernel currently trusts that generation

### Step 4: restart the plugin

From the second terminal:

```bash
cd /tank/development/rpc_plugin_system
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo restart
```

That should:
- stop the current plugin process
- start a new process
- create a new generation
- perform fresh bootstrap auth
- return updated state

The important thing to verify is:
- the `generation_id` increases
- the plugin becomes healthy again

### Step 4b: inspect one plugin and route direct requests by plugin id

The multi-plugin control surface is growing toward v1. Even in single-plugin mode, the control CLI now exposes direct plugin-id targeting for inspection and routed operations.

Examples:

```bash
cd /tank/development/rpc_plugin_system
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo plugin -plugin-id echo
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo capabilities
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo routes
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo heartbeat -plugin-id echo
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo echo -plugin-id echo -message hello
```

Current routing note:
- direct routing by plugin id exists now through a small routed-call layer for targeted operations such as `Heartbeat`, `Echo`, and restart
- the host resolves direct targets explicitly before routed calls execute
- `routes` shows the current explicit direct-routing table
- current routes are explicitly marked as `direct-plugin-id`
- broader capability-based routing is still later work

### Step 5: inspect runtime artifacts

While the daemon is running, inspect the runtime directory:

```bash
ls -la /tmp/rpc_plugin_system-demo
```

You should typically see artifacts like:
- `admin.sock`
- `echo.sock`
- `events.jsonl`

The event log is the canonical operator log for v0.1.0. It is append-only JSONL with verbose lifecycle, auth, RPC, restart, and cleanup events.

The one-time auth token file is bootstrap-only and should be removed after successful auth.

### Step 6: stop the daemon

Go back to the terminal running `rpcplugind` and press `Ctrl+C`.

After shutdown, you can check whether runtime artifacts were cleaned up:

```bash
ls -la /tmp/rpc_plugin_system-demo
```

## Tutorial: run the failure plugin

The failure plugin exists so the kernel can be tested against controlled bad behavior.

### Example: force auth failure

```bash
cd /tank/development/rpc_plugin_system
RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH=true \
.tmp-bin/rpcplugind \
  -runtime-dir /tmp/rpc_plugin_system-failure \
  -plugin ./.tmp-bin/rpcplugin-failure \
  -plugin-id failure
```

Expected result:
- daemon startup should fail
- the kernel should reject the plugin during bootstrap

### Example: close transport on echo

This path is mainly exercised in tests, but the behavior is controlled through env vars such as:
- `RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ECHO`
- `RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CRASH_ON_ECHO`
- `RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_STATUS`
- `RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_ERRORS`
- `RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SLEEP_SCALE`
- `RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ACCEPT`

See:
- `cmd/rpcplugin-failure/main.go`
- `internal/testpluginapi/env.go`
- `internal/kernel/failure_suite_test.go`

## How the example plugin is configured

The supervisor passes runtime information to plugins through environment variables, including:
- `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
- `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
- `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`

The auth token file is one-time-use per generation and is removed after successful bootstrap.

## Failure testing

The repo includes a configurable failure plugin used by the kernel tests:
- `cmd/rpcplugin-failure`

Behavior is driven by environment variables and exercised through:
- `internal/kernel/failure_suite_test.go`

That suite currently covers startup/auth/identity/generation/capabilities/heartbeat/timeout/crash/transport/restart-churn behavior.

## Logging

v0.1.0 logging is intentionally first-class.

Current logging design:
- canonical log format is append-only JSONL
- kernel and SDK-backed plugins share the same event schema and severity model
- logs are written to `events.jsonl` in the runtime dir
- every write is flushed with `fsync` so logs are durable and human-inspectable during failures
- entries are verbose and include level, component, event, plugin id, generation id, pid, socket path, method, message, error/reason, and optional details

The log is designed to be both:
- machine-parseable for later tooling
- human-readable enough to inspect directly with normal shell tools

The kernel logs at least these categories:
- manager initialization
- plugin start request/success/failure
- socket dial request/success/failure
- auth start/success/failure
- capability load start/success/failure
- heartbeat unhealthy/failure transitions
- RPC start/success/failure/timeout
- RPC poison events
- restart request/success/failure
- shutdown request/success/failure
- forced kill
- runtime cleanup success/failure

SDK-backed plugins log at least these categories:
- boot/config load
- listener start/stop
- auth attempt/accept/reject
- capability reporting
- RPC handler success/failure
- shutdown handling

The control CLI now supports log inspection:

```bash
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo logs
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo -component rpc -level warn logs
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo -event plugin_auth_failed -format json logs
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo -since 15m -summary logs
```

Logging final-form notes for v0.1.0:
- event logs rotate automatically when they grow too large
- rotated backups are retained and included in CLI log reads
- CLI supports text/json output, filtering, recency windows, reverse order, and summary mode

## Current limitations

This is still a pre-v1 standalone substrate, not a polished general release.

Current constraints include:
- one supervised plugin instance at a time
- local Unix-socket operation only
- Go `net/rpc` transport only
- plugin auth/bootstrap UX is still developer-oriented
- some naming and bootstrap rough edges remain

## Road to v1.0

The current intent is to finish `rpc_plugin_system` as a real v1.0 standalone substrate before starting memory-system implementation on top of it.

That means the near-term focus stays on:
- hardening and stress coverage
- SDK/plugin authoring polish
- compatibility and packaging discipline
- stronger public docs/examples
- branch/release promotion discipline up to v1.0

## Key docs

- `docs/v0.1.0.md`
- `docs/plugin-api.md`
- `docs/plugin-abi.md`
- `docs/compatibility.md`
- `docs/plugin-standard-v0.md`

## Status

The kernel slice is real, tested, and being hardened through failure-point and stress-oriented validation before the standard is considered fully earned.
