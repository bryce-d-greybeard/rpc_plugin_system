# Plugin API

## Purpose

This document defines the logical API contract between the rpc_plugin_system kernel and one plugin executable for v0.1.0.

## Core principles

- Plugins are separate executables.
- The kernel is the supervising authority.
- Every plugin instance belongs to one generation.
- Responses from stale generations must not be accepted.
- A plugin is not considered healthy until full startup, authentication, capability registration, and heartbeat succeed.

## Required methods

### `Auth`
Finalizes the transitional handoff from bootstrap authorization into refreshed session key material for the current generation.

Request fields:
- `Token`
- `SessionID` when bootstrap session transport is enabled
- `PluginPublicKey` when bootstrap session transport is enabled

Reports:
- plugin id
- version
- generation id
- `SessionID` when bootstrap session transport is enabled
- `TransportKeyGeneration` when bootstrap session transport is enabled
- `TransportProfile` when bootstrap session transport is enabled

Rules:
- the bootstrap token is one-time-use
- under secure bootstrap, token spend occurs when the kernel consumes the validated bootstrap session, before transitional `Auth` runs
- repeated auth attempts with the same token must not be accepted as a fresh bootstrap
- session fields are generation-scoped and must not be replayed across generations
- after secure bootstrap is active, `Auth` is a bootstrap-completion verification step, not the lasting trust anchor
- under secure bootstrap, session identity and bootstrap public-key continuity matter more than replaying the token itself
- auth no longer pretends to return the full transport key blob
- instead it confirms the transport key generation and profile that the secure channel is already using
- steady-state RPC traffic after this point uses refreshed session keys, not the bootstrap token

### `Capabilities`
Reports:
- plugin id
- version
- generation id
- capability list

Rules:
- optional methods must be advertised through capability reporting
- capability names should be stable and documented

### `Heartbeat`
Reports:
- plugin id
- version
- generation id
- uptime
- health status
- current work count
- last successful request time
- recent error count

The current v0.1.0 wire fields are:
- `UptimeSeconds`
- `Status`
- `CurrentWorkCount`
- `LastSuccessfulUnixSec`
- `RecentErrorCount`

### `Shutdown`
Requests graceful shutdown.

For the substrate itself, the stable required wire methods are still the `TestPlugin.*` methods documented here.

For public authors using `sdk/go/plugin`, the stable authoring path is:
- load config with `LoadConfigFromEnv()`
- build the minimal core with `NewTemplate(...)`
- extend optional capabilities by implementing the matching optional interfaces
- serve through `ServeWithConfig(...)`

## v0.1.0 test-plugin methods

These are required for the v0.1.0 test plugin specifically:

### `Echo`
Round-trips a message for request/response verification.

### `Sleep`
Sleeps for a duration for timeout testing.

### `Crash`
Terminates the plugin process for crash/restart testing.

## API behavior rules

- Every response is generation-scoped.
- Plugin id must match the kernel's expected plugin id.
- Generation id must match the generation that the kernel issued requests against.
- Timeout is a kernel concern; plugins should not assume infinite request duration.
- Plugins should be restart-safe and tolerate process replacement.
- Required v0.1.0 RPC service/method names remain under the `TestPlugin.*` namespace for compatibility with the frozen substrate.

## Error semantics

The kernel must treat these as failures:
- auth failure
- Linux peer credential verification failure on supported Linux runtime paths
- method timeout
- transport break
- generation mismatch
- plugin id mismatch
- malformed or missing capability data
- heartbeat failure

## Health model

Suggested health values:
- `starting`
- `healthy`
- `degraded`
- `overloaded`
- `unhealthy`

The kernel owns policy decisions based on reported health.

## Bootstrap pre-RPC exchange

Before the kernel trusts non-auth RPC methods, the plugin may be required to complete a transport-backed bootstrap exchange.

Current live shape:
- plugin loads startup config from env
- plugin reads the generation-scoped auth token file
- plugin opens the bootstrap request FIFO and reads one bootstrap record
- plugin validates plugin id, session id, and encoded token
- plugin derives bootstrap session material from the substrate public key and its own ephemeral keypair
- plugin opens the bootstrap response FIFO and writes one bootstrap response carrying plugin public key material
- plugin then proceeds to normal RPC serving and `Auth`
- kernel and plugin immediately refresh the derived session material before steady-state RPC trust is granted

The bootstrap exchange is kernel-owned substrate behavior. Public plugin authors using `sdk/go/plugin` should normally consume it through `LoadConfigFromEnv()` and `ServeWithConfig(...)`, not by reimplementing the handshake manually.

## Steady-state RPC transport

After `Auth` finalizes the transitional bootstrap handoff, the live RPC connection is framed with refreshed directional session keys derived from the bootstrap exchange.

Current live transport behavior:
- manager sends an 8-byte big-endian connection identifier before secure framing starts
- both sides derive per-connection transport keys from refreshed directional session keys plus that identifier and fixed direction labels
- each RPC write becomes one encrypted/authenticated frame with a 4-byte big-endian ciphertext length prefix followed by AES-GCM ciphertext
- each wrapped connection starts its frame sequence at zero, so per-connection key derivation is required to avoid nonce/key reuse across reconnects
- each direction is bounded by `1 << 32` frames, `32 GiB` plaintext, or `60m` connection age, whichever comes first
- when a bound is reached, the transport fails closed and rekey happens by reconnect/restart, not by in-band negotiation

Public plugin authors using the SDK should treat this as substrate-owned transport behavior rather than application-level message design. The full transport contract lives in `docs/transport-contract.md`.
