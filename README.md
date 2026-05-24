# rpc-plugin-system

`src/` is the source root. Workflow state is kept separately at repo root in `workflow.toml` and `workflow/` so code, tests, and examples are not polluted by workflow bookkeeping. Project-level documentation lives in repo-root [`docs/`](docs/). The substrate PRD, architecture, implementation spec, downstream provider guidance, compatibility notes, roadmap, and v1 freeze notes are canonical docs under [`docs/`](docs/).

Classic source layout:

- `src/cmd/rpcplugind/` — daemon entrypoint.
- `src/cmd/rpcpluginctl/` — CLI entrypoint.
- `src/internal/adminrpc/` — local admin RPC server/client.
- `src/internal/cli/` — CLI formatting helpers.
- `src/internal/eventlog/` — JSONL event log writer/reader plus provider observability schema, filters, and redaction helpers.
- `src/internal/kernel/` — plugin manager, host routing, lifecycle, monitoring.
- `src/internal/runtime/` — runtime directory, sockets, executable and peercred helpers.
- `src/internal/auth/` — startup token helpers.
- `src/pkg/plugin/` — public in-repo Go plugin SDK, including structured provider diagnostic emission.
- `src/examples/` — example plugins.
- `src/test/` — shared test fixtures/helpers.


## Substrate boundary

This repo is the executable plugin substrate. It owns local process supervision, startup authentication, Unix-socket `net/rpc` transport, generation identity, lifecycle/heartbeat/restart behavior, routing, runtime artifacts, append-only JSONL logs, provider bundle metadata, SDK logging helpers, and local admin/CLI inspection.

It does **not** own broker admission, authority issuance, credential resolution, Lua execution, provider-specific workflow semantics, provider payload handling, or client-visible provider capability emission. Those belong to the core broker, authority owners, and provider-specific repos.

Provider observability is structured and redacted:

- provider diagnostics use the SDK/helper path, not arbitrary lifecycle log fields;
- plugin id, generation id, and pid are substrate-bound facts;
- capability id, operation id, correlation id, duration, bounded status, coarse error class, degraded/unavailable reason, counts/sizes, and non-authoritative digest facts are the safe diagnostic vocabulary;
- raw prompts, payloads, upstream response bodies, credentials, tokens, bearer strings, API keys, private keys, passwords, authority refs, handles, sockets, sessions, signer material, provider-private paths, and reusable external refs are rejected before durable provider writes or redacted before operator display.

Downstream provider docs should inherit this substrate by reference and add only provider-specific deltas. See [`docs/downstream-provider-guidance.md`](docs/downstream-provider-guidance.md).

## Provider diagnostics status

Final provider diagnostics judge sweep accepted this substrate with constraints. `rpc-plugin-system` owns the provider event schema, SDK helper, admin/CLI filters, and unsafe-material rejection/redaction. It still does not own provider-specific semantics, broker admission, authority issuance, routing policy, or client-visible provider surfaces. Keep provider diagnostics on `Logger.ProviderDiagnostic`; do not smuggle them through arbitrary lifecycle `LogEvent` fields.

Build all packages:

```bash
make build
```

Or directly from the source root:

```bash
cd src && go build ./...
```

Build command binaries directly from `src/`:

```bash
cd src
go build -buildvcs=false -o .tmp-bin/rpcplugind ./cmd/rpcplugind
go build -buildvcs=false -o .tmp-bin/rpcpluginctl ./cmd/rpcpluginctl
go build -buildvcs=false -o .tmp-bin/rpcplugin-echo ./examples/rpcplugin-echo
go build -buildvcs=false -o .tmp-bin/rpcplugin-failure ./examples/rpcplugin-failure
```


Coverage policy:

- Every feature, issue fix, and regression fix must aim for 100% relevant test coverage of the behavior it touches.
- Relevant coverage means changed behavior, edge cases, regressions, and failure paths. It is not fake repository-wide line coverage theater.
- Any coverage gap must be recorded with reason, risk, and the smallest next coverage increment before the work can be considered done.
- Project coverage posture is tracked in [`workflow/artifacts/global-coverage-map.md`](workflow/artifacts/global-coverage-map.md).

Run the current package coverage report and write reusable evidence:

```bash
make coverage
```

The coverage target writes:

- `workflow/artifacts/coverage.out` — Go coverage profile for `go tool cover`/HTML inspection.
- `workflow/artifacts/coverage-summary.txt` — package coverage plus `go tool cover -func` totals, also printed to the terminal.

Run tests:

```bash
make test
```

Or directly from the source root:

```bash
cd src && go test ./...
```

Kernel-focused tests:

```bash
cd src && go test ./internal/kernel/...
```

The public Go plugin SDK lives at:

```go
import plugin "rpc_plugin_system/pkg/plugin"
```

That import path is the local Go module path used by this source release. It
is meant for packages built inside this repository checkout; it is not a
fetchable external module path and should not be used with `go get` as a
published SDK dependency.


Provider-owned diagnostics should use the structured SDK helper:

```go
err := logger.ProviderDiagnostic(plugin.ProviderDiagnostic{
    CapabilityID:  "mail.send",
    OperationID:   "send_message",
    CorrelationID: "corr-123",
    Status:        plugin.ProviderStatusDegraded,
    ErrorClass:    "upstream_unavailable",
})
```

The helper routes through the shared provider event validation/redaction contract and returns validation errors instead of writing unsafe events.

Plugin capability strings are small routing/status vocabulary emitted by the
SDK/runtime code:

- `heartbeat` — required core health check support.
- `shutdown` — required graceful shutdown support.
- `echo` — optional echo RPC support.
- `sleep` — optional sleep RPC support used by timeout/failure tests.
- `crash` — optional crash RPC support used by supervision/failure tests.

`TemplatePlugin` reports the required core capabilities (`heartbeat`,
`shutdown`) through SDK detection. Plugins that implement optional interfaces
add `echo`, `sleep`, and/or `crash`; plugins may also provide an explicit
`Capabilities() []string` override for test and compatibility scenarios.
