# rpc-plugin-system global coverage map

Canonical policy lives in [`src/workflow.toml`](../workflow.toml).

## Policy

- Target: **100% relevant test coverage** for every feature, issue fix, and regression fix.
- Scope: changed behavior, edge cases, regressions, and failure paths inside the touched feature boundary.
- Completion rule: work is not done until relevant coverage evidence is recorded, or an explicit gap is recorded with reason, risk, owner, and smallest next coverage increment.
- Line coverage is a diagnostic signal, not the completion definition. Do not claim the project is safe because a global percentage is green, and do not pretend uncovered generated/example plumbing is equivalent to uncovered runtime/auth behavior.

## Current baseline snapshot

Command run from `src/` on 2026-05-06:

```bash
go test -cover ./...
```

Observed package statement coverage:

```text
ok   rpc_plugin_system/control-plane-ops/admin-rpc/adminrpc                 coverage: 71.4% of statements
ok   rpc_plugin_system/control-plane-ops/cli-status-control/cli             coverage: 70.2% of statements
ok   rpc_plugin_system/control-plane-ops/cli-status-control/rpcpluginctl    coverage: 15.5% of statements
ok   rpc_plugin_system/control-plane-ops/event-log-write/eventlog           coverage: 75.2% of statements
ok   rpc_plugin_system/kernel-runtime-supervision/daemon-entrypoint/rpcplugind coverage: 42.1% of statements
ok   rpc_plugin_system/kernel-runtime-supervision/manager-lifecycle/kernel  coverage: 79.5% of statements
ok   rpc_plugin_system/kernel-runtime-supervision/peercred-runtime/runtime  coverage: 31.7% of statements
     rpc_plugin_system/plugin-authoring-sdk/echo-example/rpcplugin-echo     coverage: 0.0% of statements
     rpc_plugin_system/plugin-authoring-sdk/failure-example/rpcplugin-failure coverage: 0.0% of statements
ok   rpc_plugin_system/plugin-authoring-sdk/go-runtime/plugin               coverage: 25.4% of statements
ok   rpc_plugin_system/plugin-authoring-sdk/test-plugin-api/testpluginapi    coverage: 0.0% of statements
     rpc_plugin_system/release-packaging-governance/go-build-deps/testroot   coverage: 0.0% of statements
     rpc_plugin_system/secure-bootstrap-transport/startup-handshake/auth     coverage: 0.0% of statements
```

This baseline is not 100% line coverage. That is acceptable only because the policy target is **100% relevant touched-behavior coverage**, not a fake global line metric. New work must not make this posture worse and must record the behavior-specific evidence it adds.

## Required harness classes by subsystem

| Subsystem | Required harness classes | Current evidence | Known gaps / next increments |
| --- | --- | --- | --- |
| `plugin-contracts` | docs/spec consistency checks, compatibility examples, negative contract tests when code is touched | existing package tests plus docs | add executable spec checks when contract docs change |
| `secure-bootstrap-transport` | token generation, bad token rejection, replay/stale artifact rejection, transport permission checks | manager lifecycle tests exercise auth path indirectly | add direct auth package tests; add stale-generation artifact regression before runtime auth work is done |
| `kernel-runtime-supervision` | lifecycle unit tests, integration start/auth/load tests, restart race tests, monitor/admin concurrency tests, cleanup ownership tests | manager lifecycle, daemon, monitor, hardening tests | high-risk gaps: generation-scoped cleanup invariant; serialized lifecycle restart invariant |
| `control-plane-ops` | admin RPC integration, CLI argument/status tests, event-log read/write tests, negative malformed input tests | admin RPC, CLI, eventlog tests | add long-line event-log read regression; add admin restart concurrency regression |
| `plugin-authoring-sdk` | SDK API unit tests, example build tests, auth-before-RPC behavior tests, public import examples | go-runtime and test-plugin-api tests; example binaries build in `make build` | resolve public import path contract; add explicit Capabilities auth-contract test/doc decision |
| `release-packaging-governance` | build wrapper tests, install docs command validation, release checklist consistency checks | root/source Makefile verification | add docs quickstart validation for absolute plugin paths |

## Gap record format

Every accepted gap must record:

- feature or issue id
- behavior not covered
- why full relevant coverage is impossible or wasteful now
- risk level
- owner/reviewer accepting the gap
- smallest next coverage increment
- expiry or review trigger

Unrecorded gaps are failures. Green tests without behavior coverage are not evidence.
