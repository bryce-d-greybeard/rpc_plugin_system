# Plugin ABI

## Purpose

This document defines the process, wire, and runtime compatibility contract between the kernel and plugin executables.

This is not a shared-library ABI. It is a process-boundary ABI.

## Startup contract

The kernel launches a plugin executable directly.

Current v0.1.0 startup environment:
- `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
- `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
- `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE` (currently used in the test harness/plugin flow)

## Transport contract

- Unix domain sockets only in v0.1.0
- Go `net/rpc`
- gob encoding
- one live RPC connection per active plugin generation

## Authentication contract

1. kernel creates challenge file
2. plugin reads challenge
3. plugin signs challenge digest with its auth token
4. plugin writes signature artifact
5. kernel verifies signature against trusted public key
6. only then does the instance qualify as trusted

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
- remove stale socket/auth/signature artifacts
- invalidate the dead generation

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

## Compatibility rule

Any change to:
- startup env vars
- wire encoding
- lifecycle expectations
- required methods
- generation semantics

must be treated as an API/ABI compatibility change and documented explicitly.
