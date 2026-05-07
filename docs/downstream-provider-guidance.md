# Downstream provider guidance

## Purpose

This document tells downstream provider projects how to reference `rpc_plugin_system` without re-specifying the substrate.

Provider PRDs should inherit the executable-plugin contract by reference. They should not copy, weaken, or reinterpret lifecycle, transport, authentication, generation, health, logging, or supervision rules.

## Rule

Downstream provider documents must treat `rpc_plugin_system` as the source of truth for the executable plugin substrate.

Provider documents may restate consequences that matter to the provider, but must not redefine the substrate contract.

## What provider PRDs should say

A provider PRD should include an execution-model section with this shape:

```text
<provider> runs as a separate executable under rpc_plugin_system.
The executable-plugin substrate is specified by rpc_plugin_system;
this PRD only states provider-specific obligations.
```

Provider PRDs may then list inherited substrate facts:

- plugin is a process-boundary executable, never an in-process library
- kernel/supervisor owns lifecycle, restart, generation, health, routing, logs, and runtime artifacts
- startup uses the minimal substrate environment:
  - `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
  - `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
  - `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
  - `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`
- general daemon environment is not inherited and must not be treated as provider configuration
- plugin authenticates with one-time bootstrap token through `Auth` before any trusted capability use
- responses are scoped to plugin id and generation
- stale generations are not trusted
- healthy startup requires process start, socket reachability, auth, capabilities, and heartbeat
- restart means replacement, not resume
- v1 transport is Unix domain socket plus Go `net/rpc`
- alternate transports are substrate-version decisions, not provider-local decisions
- durable append-only JSONL event logging is part of the substrate observability model
- systemd, launchd, Windows services, containers, or sidecars may wrap deployment, but provider correctness must not depend on any OS service manager

## What provider PRDs should add

Provider PRDs should state only provider-specific consequences, for example:

- capability names and capability metadata
- credential lease requirements
- provider-specific side-effect classes
- provider-specific retry/idempotency rules
- provider-specific timeout/cancellation requirements
- provider-specific redaction and audit fields
- provider-specific storage boundaries
- provider-specific teardown requirements

Examples:

- a Telegram provider should describe duplicate-send and chat-scope rules
- a filesystem provider should describe path-scope, symlink, special-file, and bounds rules
- an LLM provider should describe model output as untrusted proposer data
- a browser provider should describe browser process/profile teardown
- a keyring provider should bind leases to plugin id and generation

## What provider PRDs must not do

Provider PRDs must not:

- redefine plugin authentication
- redefine generation semantics
- redefine transport
- redefine lifecycle or restart behavior
- assume inherited ambient environment variables
- require systemd, launchd, Windows services, Kubernetes, or containers for correctness
- allow in-process loading into the kernel or higher-level host
- treat a restarted plugin as a resumed trusted identity
- let optional provider methods bypass capability reporting

## Portability stance

The portable baseline is:

```text
separate executable + rpc_plugin_system contract
```

OS service managers are deployment adapters only:

- systemd on Linux
- launchd on macOS
- Windows service manager on Windows
- container or sidecar supervisors in container deployments

A provider that only works because systemd injected magic environment or filesystem state has broken the substrate boundary.

## Compatibility stance

If a provider needs a different startup environment, transport, auth flow, lifecycle rule, or generation rule, that is not a provider-local extension. It is a substrate compatibility change and must be handled in `rpc_plugin_system` standard docs and tests first.
