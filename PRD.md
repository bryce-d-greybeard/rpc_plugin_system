# PRD: rpc-plugin-system executable plugin substrate

## Problem

Agent-system providers need a stable executable substrate that can supervise provider processes, bind runtime state to a concrete plugin generation, expose operator-visible health and logs, and carry inert provider bundle metadata without becoming an authority broker itself.

Without a clear product boundary, downstream provider repos will copy, weaken, or reinterpret lifecycle, transport, authentication, generation, logging, and authority-use rules. That turns the substrate into permission smear: bundle presence, local process state, environment variables, or plugin-private descriptors start looking like authorization. That is not admissible.

## Product goal

`rpc-plugin-system` is the local executable plugin substrate for agent-system providers.

It owns process-boundary plugin execution, lifecycle supervision, runtime isolation, bootstrap authentication, generation identity, health, routing metadata, append-only substrate logs, plugin SDK authoring support, and local operator/admin inspection.

It does **not** own agent action admission, provider workflow meaning, credential authority, filesystem/browser/network/process/memory semantics, Lua execution, or generic authority-use admission. Those belong above the substrate, primarily in `agent-core-system` and the relevant authority-owner/provider repos.

## Current shipped substrate boundary

The current project state is a v1 local plugin kernel, not a distributed provider platform.

### Process and lifecycle

The substrate owns:

- launching plugins as separate executables, never in-process libraries;
- one active process identity per plugin generation;
- generation creation and monotonic generation advancement;
- graceful shutdown requests;
- restart as replacement, not resume;
- process death handling and reaping;
- stale generation invalidation;
- timeout poisoning and stale RPC client teardown;
- runtime artifact cleanup under churn.

Healthy startup requires all of:

1. process starts;
2. plugin socket becomes reachable;
3. bootstrap authentication succeeds;
4. capabilities are fetched successfully;
5. heartbeat succeeds.

Only then may the plugin be treated as healthy.

### Startup ABI

The public v1 startup environment is minimal and stable:

- `RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET`
- `RPC_PLUGIN_SYSTEM_PLUGIN_ID`
- `RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION`
- `RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE`

General daemon environment is not inherited by plugins and must not be treated as provider configuration. Test-only `RPC_PLUGIN_SYSTEM_TEST_*` controls are not public substrate ABI.

### Transport

The v1 transport is:

- local Unix domain sockets;
- Go `net/rpc` with gob encoding;
- frozen required wire namespace under `TestPlugin.*`;
- one live trusted RPC connection per plugin generation.

Alternate plugin transports such as gRPC, Connect, Twirp, JSON-RPC, dRPC, Thrift, browser-facing transports, or native UI transports are post-v1 substrate evolution. They are not provider-local extensions.

### Bootstrap trust

The substrate owns bootstrap trust:

- per-generation one-time auth token creation;
- protected auth token file delivery;
- `Auth` proof before non-auth RPC methods are trusted;
- exact token match verification;
- token file removal after successful bootstrap;
- Linux `SO_PEERCRED` verification through the runtime adapter path on supported Linux hosts;
- rejection of plugin id and generation mismatches.

Unverified plugin instances are not trusted.

### Required plugin API

Every standard plugin exposes:

- `Auth`
- `Capabilities`
- `Heartbeat`
- `Shutdown`

Required responses are plugin-id and generation scoped. Optional response fields must be additive and safe to ignore.

Optional methods are capability-driven. A plugin must advertise optional methods through `Capabilities`; optional provider capabilities do not bypass lifecycle, trust, routing, or authority-use admission.

### Multi-plugin kernel and routing

The substrate owns local multi-plugin supervision:

- multiple supervised plugin processes;
- per-plugin runtime directories;
- per-plugin sockets and auth artifacts;
- path-safe plugin ids;
- per-plugin state visibility;
- capability map inspection;
- route table inspection;
- direct routing by plugin id.

The intentionally small v1 routed-call set is:

- `Heartbeat`
- `Echo`
- `Restart`

Capability-based routing beyond inspection is not a required v1 feature unless explicitly added by a later substrate version.

### Admin and operator surface

The substrate ships:

- daemon: `rpcplugind`;
- control CLI: `rpcpluginctl`;
- local admin/control plane over Unix socket RPC scoped by runtime-dir filesystem access;
- host status inspection;
- plugin listing and per-plugin inspection;
- capability map inspection;
- route table inspection;
- targeted per-plugin restart;
- targeted heartbeat routing;
- targeted echo routing;
- kernel log inspection through `rpcpluginctl logs`.

Operator surfaces expose substrate state. They must not turn plugin-private metadata into authority, broker-emitted client surfaces, or provider workflow semantics.

### Logging and observability

Append-only JSONL event logging is first-class substrate behavior.

The substrate currently covers lifecycle/RPC/auth/restart/cleanup observability, including events for manager initialization, plugin start/dial/auth/capabilities, heartbeat health, RPC start/success/failure/timeout/poisoning, restart/shutdown/kill/stop, runtime cleanup, and log rotation.

Kernel and plugin-side SDK logs use the same logging subsystem and aligned event schema. Plugin-side SDK event logs live under sibling `plugin-events.jsonl` files as an intentional shell-first inspection surface.

The current substrate does **not yet** fully cover provider-specific semantic diagnostics, external authority/provider-boundary logging, payload logging policy, or a clean provider-owned SDK event-emission surface for capability-specific diagnostics. The acceptable direction is structured, redacted provider-boundary and provider-owned events using shared correlation fields, not raw payload logging.

### Public SDK authoring path

The public Go authoring entrypoint is:

```go
import plugin "rpc_plugin_system/pkg/plugin"
```

The stable authoring path is:

1. `LoadConfigFromEnv()`;
2. `NewTemplate(...)`;
3. optional capability interfaces where needed;
4. `ServeWithConfig(...)`.

Plugin authors should not need internal packages for a basic plugin.

## Provider bundle substrate support

Provider repos may ship repo-local `tool-skills/manifest.json` descriptors and Lua proposal machines. The substrate may carry and expose inert bundle metadata for the authenticated plugin generation.

The substrate may surface:

- plugin id;
- plugin generation;
- bundle root path;
- manifest path;
- Lua asset paths;
- manifest digest;
- per-asset digests;
- schema version;
- validation/status facts;
- redacted error facts;
- observed time;
- substrate event correlation id.

Bundle metadata is discovery, transport, provenance, and integrity input only. It is not permission, not broker emission, not executable authority, and not client-visible workflow meaning.

Canonical flow:

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

Malformed, missing, stale, unreadable, or generation-mismatched bundle metadata must fail closed as bundle-unavailable advertisement input. It must not be silently accepted, partially admitted, or converted into a degraded permission grant.

## Non-goals

`rpc-plugin-system` does not provide:

- provider workflow semantics;
- descriptor admission;
- Lua execution;
- client-visible broker surfaces;
- action authorization;
- authority-owner routing;
- credential resolution;
- filesystem, browser, network, process, memory, MCP, SSH, gRPC, or HTTP semantic authority;
- raw secret, file handle, browser session, process handle, socket, signer, credential, or reusable authority ref export;
- ambient environment fallback;
- service-manager-state fallback;
- localhost trust fallback;
- inherited descriptor fallback;
- trust in stale plugin generations;
- distributed scheduling or remote plugin execution;
- alternate plugin transports in v1;
- browser/UI-facing control-plane transports in v1;
- distro-native packaging or prebuilt binary release requirements in v1.

## Required behavior

### Lifecycle and trust

- Required startup env vars must be present and parseable.
- Non-auth RPC methods require successful bootstrap auth first.
- Plugin id mismatch is a hard failure.
- Generation mismatch is a hard failure.
- Stale generation responses are rejected.
- Timeout can poison the transport.
- Poisoned transports are not reused.
- Restart creates a fresh process, fresh generation, fresh auth handshake, fresh connection, fresh capabilities fetch, and fresh heartbeat before healthy state.

### Capability and routing state

- Capabilities are generation-scoped.
- Capabilities are refreshed after restart.
- Optional capability absence is visible and additive, not a lifecycle failure by itself.
- Dead, stale, unauthenticated, or unhealthy generations must not remain routable as live trusted providers.
- Direct routing by plugin id must preserve lifecycle and trust checks.

### Bundle metadata

- Bundle metadata is collected only for the authenticated plugin generation.
- Restart invalidates prior bundle metadata.
- Bundle APIs return inert facts only.
- Core-facing DTOs distinguish declared substrate metadata from broker-admitted/emitted surfaces.
- Admin output redacts host-private or secret-like material and never displays secrets.
- The substrate never runs Lua, never interprets descriptor permission, and never calls authority owners.

### Logging and diagnostics

- Kernel lifecycle and RPC events are append-only JSONL.
- Log schema removals or semantic breakage are compatibility breaks.
- Additive event fields are allowed when they preserve operator tooling.
- Provider-specific diagnostics must be structured and redacted.
- Provider-boundary events should carry correlation id, plugin id, generation, capability/operation identity, duration/status/error class, and degraded/unavailable reason where applicable.
- Logs must not record raw prompts, raw provider payloads, credentials, authority handles, raw response bodies, or provider-private paths by default.

## Acceptance criteria

The substrate is acceptable when tests and docs prove:

- plugin lifecycle, auth, generation, heartbeat, timeout poisoning, restart, and stale-client behavior match the documented contract;
- Linux peer credential verification works on supported Linux runtime paths;
- repeated crash/restart, timeout, transport-break, and monitor-loop failure sequences recover without stale trust or artifact leaks;
- multi-plugin state remains attributable by plugin id and generation;
- admin/CLI surfaces expose host/plugin/capability/route/log state without changing authority semantics;
- public SDK plugins can be authored through the supported `pkg/plugin` path without importing internals;
- provider bundle metadata is tied to plugin id and generation;
- restart invalidates stale bundle/capability state;
- core can distinguish substrate discovery metadata from broker-admitted surfaces;
- malformed, missing, unreadable, stale, or generation-mismatched bundle metadata never becomes client-visible authority;
- provider diagnostics/logging remains redacted and correlation-based;
- documentation tells downstream providers to inherit the substrate contract instead of redefining it.

## Compatibility stance

After v1.0.0, backward compatibility is the default.

Breaking changes include casual changes to:

- required startup env names;
- required wire method names;
- startup/auth flow;
- generation semantics;
- required lifecycle behavior;
- required response field meanings;
- peer credential verification semantics on supported platforms;
- core logging schema fields used by operator tooling.

Any such change requires explicit standard/spec/test updates and an intentional major-version decision.

## Documentation map

Implementation and contract detail lives in:

- `docs/plugin-standard-v0.md` — stable plugin standard;
- `docs/plugin-api.md` — logical API contract;
- `docs/plugin-abi.md` — process, wire, startup, transport, auth, generation, teardown contract;
- `docs/compatibility.md` — compatibility matrix and hard/soft failure behavior;
- `docs/v1-freeze.md` — v1 included/excluded scope;
- `docs/architecture.md` — provider bundle substrate boundary;
- `docs/implementation-spec.md` — provider bundle metadata implementation spec;
- `docs/downstream-provider-guidance.md` — rules for provider PRDs inheriting the substrate;
- `docs/roadmap.md` — post-v1 transport, hardening, UI/control-plane evolution.
