# rpc_plugin_system

`rpc_plugin_system` is a local Go project for supervising executable plugins over Unix domain sockets using Go `net/rpc`.

The current v0.1.0 slice is a kernel-first substrate that proves:
- one supervised plugin process at a time
- Unix socket RPC transport
- keypair challenge/response trust on startup
- heartbeat and health reporting
- timeout handling and poisoned-client teardown
- restart supervision with generation tracking
- a small admin/control CLI

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
- `internal/testpluginapi` - RPC contract used by the example/failure plugins

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

## Quick start

### 1. Generate a plugin keypair

The daemon expects the plugin auth token in `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`.

Right now the easiest way to generate a compatible keypair is with a tiny Go helper:

```bash
cat > /tmp/gen_rpc_plugin_key.go <<'EOF'
package main

import (
  "fmt"
  "rpc_plugin_system/internal/auth"
)

func main() {
  kp, err := auth.NewToken()
  if err != nil {
    panic(err)
  }
  fmt.Println("PRIVATE=" + auth.Encode(kp.Private))
  fmt.Println("PUBLIC=" + auth.Encode(kp.Public))
}
EOF

go run /tmp/gen_rpc_plugin_key.go
```

Save the printed values.

### 2. Export the auth token for the daemon

```bash
export RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE='...private-from-generator...'
```

### 3. Start the daemon with the example plugin

```bash
mkdir -p .tmp-bin
go build -o .tmp-bin/rpcplugind ./cmd/rpcplugind
go build -o .tmp-bin/rpcplugin-echo ./cmd/rpcplugin-echo

.tmp-bin/rpcplugind \
  -runtime-dir /tmp/rpc_plugin_system-demo \
  -plugin ./.tmp-bin/rpcplugin-echo \
  -plugin-id echo
```

That starts the supervisor, launches the plugin, and serves the local admin socket at:

- `/tmp/rpc_plugin_system-demo/admin.sock`

It also writes the event log to:

- `/tmp/rpc_plugin_system-demo/events.jsonl`

### 4. Query status from another shell

```bash
go build -o .tmp-bin/rpcpluginctl ./cmd/rpcpluginctl

.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo status
```

### 5. Restart the supervised plugin

```bash
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo restart
```

## How the example plugin is configured

The supervisor passes runtime information to plugins through environment variables, including:
- `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
- `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
- `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`

The example plugin also expects the auth challenge path via:
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`

That legacy auth env name still exists in the current code path and should likely be renamed later.

## Failure testing

The repo includes a configurable failure plugin used by the kernel tests:
- `cmd/rpcplugin-failure`

Behavior is driven by environment variables and exercised through:
- `internal/kernel/failure_suite_test.go`

That suite currently covers startup/auth/identity/generation/capabilities/heartbeat/timeout/crash/transport/restart-churn behavior.

## Current limitations

This is still a frozen early substrate, not a polished general release.

Current constraints include:
- one supervised plugin instance at a time
- local Unix-socket operation only
- Go `net/rpc` transport only
- plugin auth/bootstrap UX is still developer-oriented
- some naming and bootstrap rough edges remain

## Key docs

- `docs/v0.1.0.md`
- `docs/plugin-api.md`
- `docs/plugin-abi.md`
- `docs/compatibility.md`
- `docs/plugin-standard-v0.md`
- `POSTMORTEM-v0.1.0-rpc-restart.md`

## Status

The kernel slice is real, tested, and being hardened through failure-point and stress-oriented validation before the standard is considered fully earned.
