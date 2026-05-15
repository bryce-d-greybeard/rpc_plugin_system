# rpc-plugin-system Architecture: provider bundle substrate boundary

`rpc-plugin-system` owns executable plugin substrate mechanics. It does not own agent action admission, provider workflow meaning, credential authority, or generic authority-use admission.

## Bundle discovery boundary

Provider repositories may include:

```text
tool-skills/manifest.json
tool-skills/lua/*.lua
```

The substrate may report these assets as plugin-owned bundle metadata attached to a concrete plugin generation. Reporting means path/digest/schema/provenance/availability facts. It does not mean the bundle is admitted, executable, or visible to clients.

Canonical flow:

```text
plugin process starts under rpc-plugin-system
→ plugin authenticates for one generation
→ substrate reports capability and bundle metadata
→ agent-core-system validates/adopts bundle as provider advertisement input
→ core admits/emits broker surfaces separately
→ client requests broker-emitted operation
→ core admits usage and seals authority-use contract
```

## Responsibilities

`rpc-plugin-system` owns:

- plugin process lifecycle;
- generation identity;
- bootstrap token authentication;
- Unix socket transport;
- plugin health and restart;
- capability and bundle metadata transport;
- event logs and operator-visible substrate state.

It does not own:

- descriptor admission;
- Lua execution;
- client-visible broker surfaces;
- action authorization;
- authority-owner routing;
- credential, filesystem, browser, network, process, memory, or MCP semantics.

## Failure behavior

Malformed, missing, stale, unreadable, or generation-mismatched bundle metadata leaves the plugin running only if normal substrate health permits it, but the affected bundle is unavailable to core as advertisement input. It must not be silently accepted, partially admitted, or converted into a degraded permission grant.
