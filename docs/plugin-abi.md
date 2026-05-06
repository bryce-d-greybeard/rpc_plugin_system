# Plugin ABI

## Purpose

This document defines the process, wire, and runtime compatibility contract between the kernel and plugin executables.

This is not a shared-library ABI. It is a process-boundary ABI.

## Startup contract

The kernel launches a plugin executable directly.

Current v0.1.0 startup environment is intentionally minimal:
- `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
- `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
- `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`

General daemon environment variables are not inherited by the plugin process. The current in-repo test harness may additionally forward test-only `RPC_PLUGIN_SYSTEM_TEST_*` controls when those variables are set in test builds; those are not part of the public plugin ABI.

## Transport contract

- Unix domain sockets only in v0.1.0
- Go `net/rpc`
- gob encoding
- one live RPC connection per active plugin generation
- the frozen v0.1.0 service namespace remains `TestPlugin.*`

## Authentication contract

1. kernel creates a one-time auth token for the generation
2. kernel writes the token to a protected auth file
3. plugin reads the token from `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`
4. on Linux v1 paths, the kernel also verifies peer credentials through the runtime adapter before trusting the connection
5. plugin proves the token once through the `Auth` RPC method
6. kernel verifies exact token match
7. kernel removes the auth token file after successful bootstrap
8. only then does the instance qualify as trusted

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
- remove stale socket/auth artifacts
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
