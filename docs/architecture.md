# rpc-plugin-system Architecture: executable plugin substrate boundary

`rpc-plugin-system` is the local executable plugin substrate for agent-system providers. It owns process-boundary execution, lifecycle supervision, runtime isolation, bootstrap trust, generation identity, health, routing metadata, append-only event logs, SDK authoring support, and local operator/admin inspection.

It does **not** own agent action admission, provider workflow meaning, credential authority, descriptor admission, Lua execution, client-visible broker surfaces, or generic authority-use admission. Those responsibilities live above the substrate, primarily in `agent-core-system` and the relevant authority-owner/provider repositories.

The architecture rule is simple: the substrate may prove *which executable generation said what*, and may carry inert metadata and redacted diagnostics. It must not decide that the provider is allowed to do the thing.

## Layer model

```text
provider executable process
  ↕ Unix socket net/rpc, one trusted connection per generation
rpc-plugin-system substrate
  ↕ generation-scoped capability, bundle, health, and event facts
agent-core-system broker/admission layer
  ↕ broker-emitted client surfaces and admitted authority-use contracts
authority owners / provider-specific services
```

The substrate boundary is below broker admission and above raw process execution. It supervises local executables and reports facts. It does not convert facts into permission.

## Runtime identity model

A plugin identity is not just a plugin id. A trusted runtime identity is:

```text
plugin id + generation id + authenticated connection + live process instance
```

Every plugin start creates a new generation. Restart means replacement, not resume. Stale generations, stale RPC clients, stale bundle metadata, and stale capability facts must stop being trusted.

Healthy startup requires:

1. process launch;
2. socket reachability;
3. bootstrap authentication;
4. capability fetch;
5. heartbeat success.

Only after those steps may the substrate mark a plugin generation healthy.

## Startup and transport

The v1 public startup ABI is intentionally small:

- `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
- `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
- `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`

General daemon environment is not inherited by plugins. A provider that depends on ambient service-manager state, inherited secrets, localhost trust, or magic environment has broken the substrate boundary.

The v1 transport is local Unix domain socket plus Go `net/rpc`/gob. One live trusted RPC connection belongs to one plugin generation. Alternate plugin transports are substrate-version decisions, not provider-local inventions.

## Bootstrap trust

The substrate creates a one-time token for each generation, writes it to a protected auth file, and requires the plugin to prove the token through `Auth` before non-auth RPC methods are trusted. On supported Linux paths, peer credential verification uses the runtime adapter around `SO_PEERCRED`.

The substrate hard-fails trust when plugin id, generation, token, executable/runtime assumptions, or required API behavior do not match the documented contract.

## Required API and capability reporting

Every standard plugin exposes:

- `Auth`
- `Capabilities`
- `Heartbeat`
- `Shutdown`

Responses are plugin-id and generation scoped. Optional methods are capability-driven and must be advertised through `Capabilities`.

Capability advertisement is availability metadata. It is not broker emission, not permission, and not client-visible workflow meaning. Core or another broker layer must separately admit and expose any client surface.

## Multi-plugin kernel and routing

The substrate supervises multiple local plugin processes with per-plugin runtime isolation:

- per-plugin runtime directories;
- per-plugin socket/auth artifacts;
- path-safe plugin ids;
- per-plugin state visibility;
- capability map inspection;
- route table inspection;
- direct routing by plugin id.

The v1 routed-call set is intentionally small:

- `Heartbeat`
- `Echo`
- `Restart`

Capability-based routing beyond inspection is not required v1 substrate behavior. If added later, it must still preserve lifecycle, generation, auth, and health checks.

## Admin/control plane

The local control plane exposes operator inspection and control over the substrate:

- daemon: `rpcplugind`;
- CLI: `rpcpluginctl`;
- admin RPC over local Unix socket scoped by runtime-dir filesystem access;
- host/plugin state inspection;
- plugin listing;
- capability map and route table inspection;
- targeted restart;
- targeted heartbeat/echo routing;
- kernel log inspection.

Admin/control output is operator-visible substrate state. It must not become broker admission or a provider-specific permission system.

## Bundle discovery boundary

Provider repositories may include repo-local provider bundle assets:

```text
tool-skills/manifest.json
tool-skills/lua/*.lua
```

The substrate may report these assets as plugin-owned bundle metadata attached to a concrete authenticated plugin generation. Reporting means inert path/digest/schema/provenance/availability facts.

Canonical bundle flow:

```text
plugin process starts under rpc-plugin-system
→ plugin authenticates for one generation
→ substrate reports capability and bundle metadata
→ agent-core-system validates/adopts bundle as provider advertisement input
→ core admits/emits broker surfaces separately
→ client requests broker-emitted operation
→ core admits usage and seals authority-use contract
→ provider executes only through admitted mediated authority
```

Bundle metadata may include plugin id, generation, bundle root, manifest path, Lua asset paths, digests, schema version, validation status, redacted error facts, observed time, and event correlation id.

It does not mean the bundle is admitted, executable, permission-granting, or visible to clients.

Malformed, missing, stale, unreadable, or generation-mismatched bundle metadata leaves the plugin running only if normal substrate health permits it, but the affected bundle is unavailable to core as advertisement input. It must not be silently accepted, partially admitted, or converted into a degraded permission grant.

## Observability architecture

Append-only JSONL event logging is first-class substrate behavior. The current event model covers lifecycle, RPC, auth, restart, cleanup, heartbeat, timeout poisoning, and log rotation.

Provider observability extends that model in two distinct lanes:

1. **Substrate boundary events** — what the substrate can observe at the plugin boundary.
2. **Provider-owned diagnostics** — what the provider reports about its own capability-specific behavior through the SDK logging/event sink.

Those lanes share correlation and redaction rules, but they are not the same thing. The substrate must not pretend to understand every provider's semantics. The provider may describe its own semantics, but only as diagnostics, not authority.

Provider observability events should carry safe facts such as:

- plugin id;
- plugin generation;
- capability identity;
- operation identity;
- request/correlation id;
- status;
- duration;
- error class;
- degraded or unavailable reason;
- digest/ref facts when explicitly non-authoritative.

Provider observability events must not carry raw prompts, raw provider payloads, credentials, tokens, raw authority refs, file/process/browser/socket handles, provider-private paths, raw response bodies, or reusable authority material by default.

## SDK diagnostics boundary

The public SDK may expose helpers for provider-owned structured diagnostics. Those helpers must bind events to the current startup config: plugin id and generation. They should make the safe path easy by requiring capability, operation, correlation, status, and error/degraded fields rather than encouraging arbitrary unstructured blobs.

SDK diagnostics are logs. They are not:

- broker admission;
- authority issuance;
- client-visible surface emission;
- Lua execution;
- permission grants;
- provider workflow semantics.

## Redaction and trust rules

Redaction is part of the architecture, not UI polish.

The substrate should prefer allowlisted structured event fields over free-form maps. Where free-form details are unavoidable, secret-like, payload-like, path-like, and authority-like material must be rejected or redacted before it reaches durable logs or operator output.

Correlation is allowed. Authority material is not.

Safe observability tells an operator what failed and where to look. Unsafe observability exports the thing the provider was trusted to protect. Do not confuse those.

## Responsibilities

`rpc-plugin-system` owns:

- plugin process lifecycle;
- runtime isolation;
- generation identity;
- bootstrap token authentication;
- Linux peer credential hardening path where supported;
- Unix socket `net/rpc` transport for v1;
- required plugin API contract;
- health and restart behavior;
- capability and bundle metadata transport;
- direct plugin-id routing for the v1 routed-call set;
- append-only substrate event logs;
- plugin-side SDK event/logging support;
- local operator/admin substrate state.

It does not own:

- descriptor admission;
- Lua execution;
- client-visible broker surfaces;
- action authorization;
- authority-owner routing;
- credential resolution;
- filesystem, browser, network, process, memory, MCP, SSH, gRPC, or HTTP semantic authority;
- raw authority ref creation or resolution;
- provider workflow semantics;
- payload logging policy exceptions for individual providers.

## Failure behavior

Failure must be visible and fail closed at the correct layer.

- Auth failure means the generation is untrusted.
- Generation mismatch means the response/fact is rejected.
- Timeout may poison the RPC transport.
- Poisoned transports are not reused.
- Restart invalidates capabilities, bundle metadata, and connection trust.
- Malformed bundle metadata becomes unavailable advertisement input, not permission.
- Missing provider observability support means less diagnostic detail, not fake health.
- Redaction rejection should preserve safe error facts without writing unsafe material.

## Evolution constraints

Post-v1 substrate evolution may add additional transports, broader platform credential backends, richer admin surfaces, or provider observability helpers. Those are substrate compatibility decisions.

Downstream providers must not locally redefine startup ABI, transport, auth, generation, lifecycle, logging, or authority-use rules because it is convenient. That is how userspace gets broken and trust boundaries rot.
