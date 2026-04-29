# Plugin Standard v0

## Purpose

This document defines the first stable plugin standard shipped with `rpc_plugin_system v1.0.0`.

This standard is intended to be generic and standalone. It is not tied to any higher-level application domain.

## Standard layers

The plugin standard has four parts:

1. API: logical RPC method contract
2. ABI: executable, process, wire, and lifecycle contract
3. Compatibility: versioning and feature expectations
4. Lifecycle: health, teardown, restart, and generation rules

## Core design principles

- Plugins are separate executables, never in-process libraries.
- The kernel is the supervising authority.
- Plugin instances are disposable.
- Process death means RPC identity death.
- Restart means replacement, not resume.
- Each plugin start creates a new generation.
- Stale generations must not remain trusted.
- The kernel must survive plugin failure.
- Logging is first-class, verbose, and operator-readable.
- Plugins and kernel share one logging subsystem and event schema.

## Required API surface

Every standard plugin must expose these required RPC methods:

### `Auth`
Returns:
- plugin id
- plugin version
- protocol version where applicable later
- generation id

Rules:
- `Auth` remains the required startup continuity-verification method in the frozen v1 wire contract
- bootstrap token auth is one-time-use per generation
- in the historical v0 token-only model, successful auth spent the token for that generation
- in the newer secure bootstrap path, token spend occurs earlier at bootstrap session consume time
- a spent token must not be accepted as a fresh bootstrap
- under secure bootstrap, `Auth` is no longer the primary trust root; it confirms continuity for the already-established secure session

### `Capabilities`
Returns:
- plugin id
- plugin version
- protocol version
- generation id
- capability list
- optional capability metadata

### `Heartbeat`
Returns:
- plugin id
- plugin version
- protocol version
- generation id
- health status
- uptime
- inflight work count
- recent error count
- optional detail fields

### `Shutdown`
Requests graceful shutdown.

## Optional API surface

Optional methods are capability-driven.

Examples for future plugins:
- store methods
- retrieval methods
- importer methods
- ranker methods
- policy methods

A plugin must advertise optional methods through capability reporting.

## API rules

- Every response is generation-scoped.
- Plugin id must match expected plugin id.
- Protocol version must be compatible.
- Capability names should be stable and documented.
- Required fields must always be present.
- Optional fields must be additive and safe to ignore.

## ABI rules

### Executable model
- plugin is launched by the kernel as an executable
- plugin is not linked into the kernel process
- kernel owns lifecycle and restart policy

### Logging model
- kernel and plugins use the same logging subsystem
- canonical log format is append-only JSONL
- event schema, severity levels, and core correlation fields stay aligned across kernel and plugins
- logs must remain human-readable enough for direct operator inspection

### Startup environment
The kernel may provide environment variables such as:
- `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
- `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
- `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`
- future protocol/version variables

### Transport
- v0 transport is Unix domain sockets plus Go `net/rpc`
- one live connection belongs to one generation
- reconnect always creates a fresh connection and fresh client

### Authentication
- kernel creates a one-time bootstrap token for the generation
- kernel writes the token to a protected auth file
- plugin reads the token through `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`
- in the historical token-only path, plugin proves the token once through `Auth`
- in the newer secure bootstrap path, the token is validated and consumed during bootstrap session completion before `Auth` continuity verification
- token file is removed after successful bootstrap
- unverified plugin instances are not trusted
- the runtime hardening path may also verify kernel-reported peer credentials through a platform adapter
- v1 ships Linux peer credential verification first through `SO_PEERCRED`
- post-v1 may add BSD and other Unix backends behind the same adapter pattern

## Lifecycle rules

### Healthy startup requires all of:
1. process starts
2. socket becomes reachable
3. transport-backed bootstrap succeeds
4. `Auth` continuity verification succeeds
5. capabilities call succeeds
6. heartbeat succeeds

Only then may the kernel mark the plugin healthy.

### Teardown rules
On plugin death, timeout, or poisoned transport, the kernel must:
- invalidate the current generation
- close rpc client
- close underlying connection
- stop trusting the dead process instance
- reap process state
- remove stale runtime artifacts when applicable
- emit explicit lifecycle and cleanup log events explaining what happened

### Restart rules
Restart means:
- new process
- new generation id
- fresh auth handshake
- fresh connection
- fresh capabilities fetch
- fresh heartbeat success before healthy state

### Timeout rules
- timeout is not merely a failed call
- timeout may poison the transport
- a poisoned transport must not be reused

## Compatibility rules

### Required compatibility checks
The kernel should hard-fail startup when any of these occur:
- plugin id mismatch
- auth failure
- protocol version incompatibility
- required API missing
- generation mismatch during startup
- invalid or missing executable

### Additive changes allowed
- optional capability additions
- optional response fields
- new optional methods behind declared capabilities

### Breaking changes
Any change to:
- required method set
- required fields
- startup contract
- transport assumptions
- auth flow
- generation semantics
- lifecycle semantics

requires a new protocol/plugin-standard version.

## Versioning model

For now, standardize these version concepts:
- plugin implementation version
- plugin standard / protocol version
- capability set versioning only where needed later

The kernel should compare protocol version compatibility during startup.

## Acceptance expectations for the kernel

The kernel is not considered ready for this standard until it proves:
- dead plugin cannot keep speaking as a live generation
- stale rpc state is rejected
- timeout poisoning works
- restart creates a fresh trusted instance
- monitor loop survives plugin failure
- operator-visible state reflects health accurately
- event logs clearly explain lifecycle/auth/rpc/restart/cleanup behavior
- plugin logs follow the same logging subsystem and are consistent with kernel logs

## Stress and endurance expectations

The kernel standard is not earned by happy-path correctness alone.

Before freezing the standard, the kernel should also prove:
- repeated start/stop cycles remain stable
- repeated crash/restart churn remains stable
- repeated timeout poisoning does not leave stale clients trusted
- repeated transport breaks recover cleanly
- heartbeat polling under failure does not wedge the supervisor
- runtime artifacts remain clean across many cycles
- generation ids remain monotonic and are not reused incorrectly
- mixed-failure sequences do not corrupt operator-visible state
- long-run monitor behavior does not leak dead processes or stale sockets

Suggested validation includes:
- restart storm tests
- timeout storm tests
- transport break storm tests
- long-run monitor tests
- artifact leak checks after repeated cycles
- generation monotonicity checks under churn
- mixed-failure stress runs
- log assertions for key lifecycle, auth, timeout, restart, and cleanup events

Current earned validation now includes explicit local coverage for:
- restart storm monotonicity
- timeout storm poisoning and recovery
- repeated transport-break recovery
- repeated monitor-loop failure recovery
- mixed-failure sequence recovery with cross-plugin isolation checks
- runtime artifact cleanup assertions across restart churn
- Linux peer credential verification during startup

## Intended next users of the standard

After kernel stabilization, this standard should support plugins such as:
- storage plugins
- indexing plugins
- importer plugins
- retriever plugins
- ranker plugins
- policy plugins
- service-integration plugins

## Freeze rule

Do not casually change this standard.

For frozen v1 scope and non-goals, `docs/v1-freeze.md` is the source of truth for what shipped in `v1.0.0`.

When changing this standard:
1. update the standard docs
2. update acceptance tests
3. update kernel implementation
4. rerun stability and compatibility coverage
