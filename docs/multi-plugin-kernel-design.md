# Multi-plugin kernel design

## Purpose

This document defines the intended v1-era kernel shape for `rpc_plugin_system` once it moves beyond the current single-plugin substrate.

The core idea is simple:
- the kernel should act like a local router and supervisor for multiple plugins
- the kernel should not absorb plugin business logic
- the control plane should remain separate from the plugin transport

## Design stance

The kernel is not just a process babysitter.

The kernel should become four things at once:
1. registry
2. router
3. supervisor
4. control plane host

Those roles should stay conceptually separate even if some code lives close together.

## What the kernel owns

The kernel should own:
- plugin process lifecycle
- generation and trust boundaries
- plugin capability discovery
- request routing to the correct plugin
- health and state tracking
- per-plugin runtime isolation
- operator-visible status and logs
- restart/isolation policy

The kernel should not own:
- memory-system retrieval logic
- storage semantics for higher-level applications
- application-specific orchestration policy beyond generic plugin routing and supervision

## Layer model

### 1. Registry

The registry answers:
- what plugins are installed or configured?
- what plugin ids exist?
- what capabilities do they expose?
- what versions are running?
- what is each plugin's current health/generation/state?

A registry entry should include at least:
- plugin id
- executable path
- runtime directory
- current generation id
- current pid
- current health
- current capabilities
- restart policy
- last error state

The registry is the kernel's source of truth for supervised plugin state.

### 2. Router

The router answers:
- which plugin should receive this request?
- should routing be by plugin id or by capability?
- what happens when multiple plugins claim the same capability?
- what happens when no healthy plugin can serve the request?

The router should support at least two modes:
- direct routing by plugin id
- capability-based routing

The router should never bypass lifecycle/trust checks.

A dead, stale, or unhealthy generation must never remain routable as if it were alive.

### 3. Supervisor

The supervisor answers:
- how is each plugin started?
- how is each plugin restarted?
- what counts as healthy startup?
- what happens on timeout, transport break, or process death?
- how are failures isolated so one plugin does not poison the whole kernel?

This extends the current single-plugin manager model into a per-plugin supervision set.

The important rule stays the same:
- one generation = one process = one connection = one rpc client

### 4. Control plane host

The control plane answers:
- how does an operator inspect all supervised plugins?
- how does an operator restart one plugin?
- how does an operator inspect logs for one plugin or the whole kernel?
- how does a future UI talk to the kernel?

This control plane should remain separate from the kernel-to-plugin transport.

That separation matters because:
- plugin transport is about kernel-to-plugin execution and trust
- control-plane transport is about operator/admin access and tooling

## Runtime isolation model

Each plugin should have isolated runtime state.

At minimum that means:
- separate socket path
- separate auth token file
- separate runtime subdirectory
- clearly attributable pid/generation state
- logs that can be filtered by plugin id

A practical shape is:
- kernel runtime root
- per-plugin subdirectory under that root

Example:
- `/tmp/rpc_plugin_system/<plugin-id>/plugin.sock`
- `/tmp/rpc_plugin_system/<plugin-id>/auth.token`
- `/tmp/rpc_plugin_system/<plugin-id>/events.jsonl`

There may also be a kernel-global log and admin socket at the top level.

## Routing model

## Direct routing by plugin id

This is the simplest and most important mode.

Examples:
- restart plugin `indexer`
- ask plugin `retriever` for status
- send a direct RPC request to plugin `echo`

## Capability-based routing

This is the next layer.

Examples:
- find a healthy plugin that serves `embed`
- find a healthy plugin that serves `store`
- find a healthy plugin that serves `retrieve`

Capability routing needs policy.

At minimum, the kernel needs a way to decide between multiple matching plugins.

Reasonable policy options:
- one primary plugin per capability
- priority order
- explicit operator-selected route table
- later, load-aware choice

For v1, keep it simple.

My recommendation:
- direct routing by plugin id is required
- capability-based routing is allowed but should begin with explicit static preference order, not clever balancing

## Failure isolation rules

The kernel must isolate plugin failures.

If plugin A fails:
- plugin B must remain routable if healthy
- plugin A's dead generation must stop being trusted immediately
- plugin A restart policy must not corrupt global registry state
- operator-visible state must show plugin A failure clearly

Mixed-plugin failure must not collapse all routing state into nonsense.

## Admin/control-plane shape

The current admin surface is too small for v1.

Today it effectively exposes:
- single-plugin status
- single-plugin restart

A v1 control plane should expose at least:
- list all plugins
- inspect one plugin state
- restart one plugin
- restart all plugins optionally
- inspect logs for one plugin
- inspect logs across all plugins
- inspect capability map
- inspect routing decisions or route table

That can still be local-only in v1.

## Proposed v1 admin concepts

### State types

A global kernel status should include:
- kernel health
- known plugins
- plugin states
- capability map
- routing policy summary

A per-plugin state should include:
- plugin id
- version
- generation id
- healthy flag
- pid
- runtime dir
- socket path
- capabilities
- last error
- last restart time

### Admin operations

Suggested control-plane operations:
- `ListPlugins`
- `GetPlugin`
- `RestartPlugin`
- `RestartAll`
- `GetCapabilityMap`
- `GetRouteTable`

The CLI can then grow commands like:
- `rpcpluginctl plugins`
- `rpcpluginctl plugin <id> status`
- `rpcpluginctl plugin <id> restart`
- `rpcpluginctl routes`
- `rpcpluginctl capabilities`

## Logging model in a multi-plugin kernel

The current logging work stays useful, but the operator model expands.

Required properties:
- each event remains attributable to plugin id where relevant
- kernel-global events remain distinguishable from plugin events
- per-plugin runtime logs stay inspectable
- aggregated log reads remain possible

For v1, do not overcomplicate this.

A sane shape is:
- per-plugin event log in the plugin runtime dir
- optional kernel-global event log at runtime root
- CLI can read one plugin log or merge multiple logs for display

## Implementation strategy

Do not try to leap directly from one manager to a magical distributed control plane.

Build this in slices.

### Slice 1: plugin registry + many managers

Add a kernel-level component that owns multiple `Manager` instances.

It should:
- load plugin configs
- create one manager per plugin
- start/stop/restart them independently
- expose a global view of all plugin states

This is the first real step.

### Slice 2: per-plugin runtime isolation cleanup

Move from one shared runtime dir assumption toward:
- runtime root
- per-plugin runtime subdirs

This makes multi-plugin state cleaner and less fragile.

### Slice 3: admin/control-plane expansion

Expand admin RPC and CLI from single-plugin operations to:
- plugin listing
- targeted per-plugin status/restart
- capability map inspection

### Slice 4: routing layer

Add direct routing and then simple capability routing.

Keep the first version boring:
- explicit route choice
- no implicit fancy balancing

Current direct-routing slice now includes:
- host-level direct routing by plugin id for `Heartbeat`
- host-level direct routing by plugin id for `Echo`
- routed restart by plugin id through the same direct-routing layer
- an explicit `RouteTarget(plugin_id)` resolution step in the host so direct routing is a deliberate primitive instead of scattered manager lookups
- a small routed-call layer in the host for supported direct operations
- admin/control-plane routing for targeted `Heartbeat`, `Echo`, and restart
- CLI commands for `plugin`, `capabilities`, `heartbeat`, `echo`, and restart
- explicit routes marked as `direct-plugin-id` so the route surface is documented as a deliberate mode, not an accident

Still needed to finish the direct-routing story:
- decide whether v1 needs routed operations beyond the current small initial set of `Heartbeat`, `Echo`, and restart
- prove routed operations remain coherent under more mixed failure churn than the current first proof slice

Current route-inspection slice now adds:
- an explicit direct-routing table in host state
- admin/control-plane access to the current routes
- a `rpcpluginctl routes` operator command

Current routing-proof slice now adds:
- a mixed-plugin test where one routed target becomes unhealthy while another remains routable
- explicit expectation that routing stays targeted and does not smear failure across unrelated healthy plugins

### Slice 5: policy refinement

Only later add richer policy such as:
- priorities
- failover policy
- load-aware routing
- richer capability negotiation

## v1 requirement level

For an honest `v1.0.0`, I think these are the minimum multi-plugin requirements:
- more than one plugin can be supervised intentionally
- each plugin has isolated runtime state
- the operator can inspect all plugin states clearly
- the operator can restart one plugin without disturbing others
- the kernel can route by plugin id
- the capability map is visible
- the direct-routing surface is documented and test-backed

What I would not require for v1:
- dynamic load balancing
- remote/distributed routing
- complex scheduling
- clever routing heuristics
- browser UI or GTK UI shipping at the same time

## First build slice recommendation

The first concrete build slices should be:
- introduce a multi-plugin kernel host that owns multiple per-plugin managers
- move runtime layout to per-plugin subdirs
- expose `list/status/restart <plugin-id>` in the admin surface and CLI
- expose the first direct routed operations by plugin id

That gets the architecture moving in the right direction without pretending the router is more advanced than it is.
