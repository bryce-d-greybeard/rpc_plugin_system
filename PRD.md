# PRD: rpc-plugin-system provider bundle substrate support

## Problem

Provider plugins now ship repo-local `tool-skills/manifest.json` descriptors and Lua proposal machines. The executable substrate must carry, expose, and supervise those assets without pretending they are permission or executable authority.

If the substrate treats bundle presence as authorization, the plugin system becomes a permission smear. That is not admissible.

## Product goal

`rpc-plugin-system` shall remain the executable plugin substrate: process lifecycle, runtime isolation, bootstrap authentication, generation identity, health, routing, logs, and operator-visible runtime state. For provider bundles, it shall provide discovery/transport metadata and integrity facts only. `agent-core-system` remains the broker that admits advertisement and usage.

## Required behavior

- Supervised plugins may declare repo-local provider bundles containing descriptors and Lua proposal machines.
- The substrate may surface bundle path, digest, plugin id, plugin generation, and availability metadata to core.
- Bundle assets remain inert until core validates and admits advertisement.
- Plugin capabilities remain generation-scoped and must be refreshed after restart.
- The substrate must not grant permission, emit broker-visible surfaces, mint authority refs, resolve credentials, open sockets on behalf of descriptors, or execute Lua.
- The substrate must fail closed on missing, unreadable, malformed, stale, or generation-mismatched bundle metadata.
- Operator/admin surfaces may inspect bundle presence and digest status, not raw secrets or executable authority.

## Non-goals

- No provider workflow semantics in the substrate.
- No direct authority-owner calls.
- No raw secret, file handle, browser session, process handle, socket, or credential export.
- No fallback to ambient environment, service-manager state, localhost trust, inherited descriptors, or stale plugin generations.

## Acceptance criteria

- Provider bundle metadata is tied to plugin id and generation.
- Restart invalidates stale bundle/capability state.
- Core can distinguish substrate discovery metadata from broker-admitted surfaces.
- Tests prove malformed or stale bundle metadata does not become client-visible authority.
- Documentation tells downstream providers that descriptor/Lua assets are proposal inputs only.
