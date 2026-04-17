# rpc_plugin_system roadmap

This document lays out what the plugin system should become over time.

## v0.1.0 - kernel proof

Goal:
- prove the kernel can supervise one executable plugin safely over Unix socket `net/rpc`

Must have:
- one supervised plugin generation at a time
- one-time bootstrap token auth
- generation tracking
- capability fetch
- heartbeat
- graceful shutdown
- restart supervision
- timeout poisoning
- stale-generation rejection
- local admin CLI
- append-only event log
- green local test suite

Not the goal yet:
- distributed operation
- multi-plugin orchestration
- public SDK polish
- multi-language plugin authoring story
- wire-stable external protocol

## v0.2.0 - plugin authoring baseline

Goal:
- make it reasonable for someone to start building a plugin without understanding the whole repo

Should have:
- one obvious package/include for plugin authors to start from
- cleaner public plugin API surface
- clearer plugin lifecycle docs
- plugin skeleton example
- better error messages during bootstrap and incompatibility
- better public README/tutorial

## v0.3.0 - local hardening

Goal:
- make the local trust and failure model much harder to break after the v0.1.0 substrate is frozen

Should have:
- deeper stress/endurance coverage
- more exhaustive failure-point coverage
- peer credential verification (`SO_PEERCRED`) where supported
- stronger operator-visible state and event reporting
- cleaner runtime artifact handling under churn

## v0.4.0 - multi-plugin kernel

Goal:
- move from single-plugin proof to real local plugin management

Should have:
- multiple supervised plugins
- per-plugin runtime isolation
- per-plugin state visibility
- per-plugin restart policy
- better CLI/admin operations

## v0.5.0 - compatibility and packaging

Goal:
- make the plugin system easier to consume as a standalone project

Should have:
- stronger version negotiation story
- explicit compatibility matrix
- packaging guidance
- clearer release policy
- more stable public contracts

## v1.0.0 - stable standalone plugin substrate

Goal:
- provide a standalone local plugin kernel with a stable contract and a credible test/hardening story

Project rule:
- plugin system v1.0 comes before memory-system implementation on top of it

Must have:
- stable documented API/ABI contract
- stable plugin authoring entrypoint
- strong restart/failure semantics
- hardening and stress evidence that feels earned
- clear branch/release discipline
- public-safe docs and examples

## post-v1 plugin transport adapters

Goal:
- expand the standalone plugin system beyond the initial local `net/rpc` transport without replacing the core plugin adapter model

Direction:
- keep the plugin capability adapter pattern in place
- add transport selection above it
- treat transport adapters as a separate concern from plugin capability adapters

Likely candidates:
- gRPC
- Connect
- Twirp
- JSON-RPC
- possibly dRPC later
- possibly Thrift later

Non-goal for v1:
- shipping multiple transport stacks before the base local kernel contract is fully earned

## post-v1 UI / control-plane transports

Goal:
- add cleaner external/operator-facing transport options and native operator UI surfaces for dashboards, admin surfaces, desktop tooling, and browser-oriented tooling without confusing them with the plugin transport itself

Direction:
- keep UI/control-plane transport separate from plugin transport
- use UI-facing transports for operator tools, dashboards, browser integration, and native desktop control surfaces
- avoid forcing browser/UI concerns into the kernel-to-plugin transport too early
- keep native desktop UI frameworks above the control-plane API layer rather than treating them as plugin transports

Likely candidates:
- WebRPC
- JSON-RPC
- Connect
- possibly gRPC-backed admin surfaces where appropriate
- GTK4/libadwaita as a native Linux operator UI built on top of the control-plane API
