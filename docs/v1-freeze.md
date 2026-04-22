# v1 freeze declaration

This document states what `rpc_plugin_system v1.0.0` includes and what it does not.

It exists to stop drift during the final audit and promotion work.

## v1.0.0 includes

### Stable contract
- stable documented process-boundary plugin contract
- stable documented API, ABI, compatibility, and plugin-standard docs
- stable required startup environment contract
- stable required wire namespace under `TestPlugin.*`
- stable generation and restart semantics

### Stable authoring path
- `sdk/go/plugin` is the public Go authoring entrypoint
- `LoadConfigFromEnv()` is the supported config/bootstrap entrypoint
- `NewTemplate(...)` is the supported minimal skeleton path
- optional capabilities are added by implementing supported capability interfaces
- `ServeWithConfig(...)` is the supported serve path

### Stable local kernel behavior
- executable plugin supervision over Unix domain sockets
- one process and one trusted RPC connection per generation
- one-time token bootstrap auth enforced before non-auth RPC methods are served
- Linux peer credential verification through the runtime adapter path
- timeout poisoning and stale-client rejection
- restart supervision with generation advancement
- runtime artifact cleanup under churn
- durable append-only JSONL event logging

### Stable operator/control surface
- daemon: `rpcplugind`
- control CLI: `rpcpluginctl`
- admin/control plane over local Unix socket RPC scoped by runtime-dir filesystem access
- host state inspection
- plugin listing and per-plugin inspection
- capability map inspection
- route table inspection
- targeted per-plugin restart
- targeted `Heartbeat` routing
- targeted `Echo` routing
- kernel log inspection through `rpcpluginctl logs`
- sibling plugin-side SDK event logs under `plugin-events.jsonl`

### Stable multi-plugin scope for v1
- multiple supervised plugins
- per-plugin runtime isolation
- per-plugin state visibility
- explicit direct routing by plugin id
- the initial routed-call set is intentionally small for v1:
  - `Heartbeat`
  - `Echo`
  - `Restart`

## v1.0.0 does not include

- capability-based routing as a required v1 feature
- non-Linux peer credential backends
- alternate plugin transports such as gRPC, Connect, Twirp, JSON-RPC, dRPC, or Thrift
- browser/UI-facing control-plane transport expansion
- native desktop UI
- distro-native packaging
- prebuilt binary releases
- higher-level memory-system implementation

## Release stance

- release source only
- release only in a stable format
- after `v1.0.0`, backward compatibility is the default rule unless a major-version break is explicitly justified

## Freeze rule

During final audit and promotion work:
- do not add new feature lanes casually
- do not expand v1 scope just because something sounds nice
- only fix real blockers, coherence issues, validation failures, or release-readiness gaps
