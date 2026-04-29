# Plugin ABI

## Purpose

This document defines the process, wire, and runtime compatibility contract between the kernel and plugin executables.

This is not a shared-library ABI. It is a process-boundary ABI.

## Startup contract

The kernel launches a plugin executable directly.

Current startup environment:
- `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
- `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
- `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`
- `RPC_PLUGIN_SYSTEM_BOOTSTRAP_SESSION_ID` when transport-backed bootstrap is enabled
- `RPC_PLUGIN_SYSTEM_BOOTSTRAP_ENDPOINT` when transport-backed bootstrap is enabled

No other daemon environment variables are inherited by the plugin process.

The auth token file is generation-scoped. The kernel now writes one auth file per plugin generation, passes that exact path to the child process, and must only remove the auth artifact that belongs to the generation being cleaned up.

## Transport contract

- Unix domain sockets for plugin RPC
- Go `net/rpc`
- gob encoding
- one live RPC connection per active plugin generation
- bootstrap transport is a separate pre-RPC channel
- on POSIX hosts the bootstrap transport uses two FIFOs, one request FIFO and one response FIFO
- the bootstrap endpoint is conveyed as `<request-path>:<response-path>`
- the frozen required RPC service namespace remains `TestPlugin.*`

## Authentication contract

1. kernel creates a one-time auth token and bootstrap session for the generation
2. kernel writes the token to a protected generation-scoped auth file
3. kernel creates a bootstrap transport endpoint for that launch
4. plugin reads the token from `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`
5. plugin reads the bootstrap session id and endpoint from the startup env when present
6. plugin completes the bootstrap record/response exchange on the bootstrap transport before normal RPC trust is established
7. on Linux v1 paths, the kernel also verifies peer credentials through the runtime adapter before trusting the connection
8. plugin proves the token once through the `Auth` RPC method
9. `AuthRequest` may carry `SessionID` and `PluginPublicKey`
10. `AuthResponse` may carry `SessionID` and `SessionKey` for the established bootstrap session
11. kernel verifies exact token match, including decode/compare of the textual bootstrap token representation on the bootstrap wire
12. kernel removes the auth token file after successful bootstrap
13. only then does the instance qualify as trusted

## Generation contract

- Every plugin start creates a new generation id.
- Generation ids are monotonic per plugin manager.
- Responses from stale generations must be rejected.
- Restart means replacement, not resume.

## Teardown contract

On plugin death/restart, the kernel must:
- close RPC client
- close underlying socket connection
- reap process state
- remove stale socket and only the auth/bootstrap artifacts owned by that generation
- invalidate the dead generation
- emit lifecycle/cleanup log events that explain what happened

## Reconnect contract

A restarted plugin must:
- start as a new process
- use a new generation id
- perform a fresh auth handshake
- establish a fresh socket/RPC connection
- re-register capabilities
- pass heartbeat before being marked healthy

## Failure contract

The kernel must survive plugin failure.

Plugin death must not:
- crash the kernel
- leave stale generations accepted
- keep stale RPC clients trusted

Timeouts and transport breaks should poison the current RPC client so the dead transport is not reused.

## Compatibility rule

Any change to:
- startup env vars
- wire encoding
- lifecycle expectations
- required methods
- generation semantics
- auth bootstrap semantics
- peer credential verification semantics on supported platforms

must be treated as an API/ABI compatibility change and documented explicitly.

## Bootstrap wire addendum

The transport-backed bootstrap is now part of the process-boundary ABI.

Required bootstrap record fields:
- protocol version
- plugin id
- session id
- token as encoded text on the bootstrap wire

Required bootstrap response fields:
- protocol version
- plugin id
- session id
- token echoed back as encoded text on the bootstrap wire
- plugin public key bytes or placeholder session bootstrap key material, depending on implementation stage

This bootstrap exchange is additive to the existing RPC `Auth` method. The token-only `Auth` step is still present for compatibility, but it is no longer the whole startup story.
