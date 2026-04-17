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
Proves possession of the one-time bootstrap token for the current generation.

Reports:
- plugin id
- version
- generation id

Rules:
- the bootstrap token is one-time-use
- successful auth spends the token for that generation
- repeated auth attempts with the same token must not be accepted as a fresh bootstrap

### `Capabilities`
Reports:
- plugin id
- version
- generation id
- capability list

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

### `Shutdown`
Requests graceful shutdown.

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

## Error semantics

The kernel must treat these as failures:
- auth failure
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
