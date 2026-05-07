# rpc-plugin-system

`src/` is the source root. Workflow state is kept separately at repo root in `workflow.toml` and `workflow/` so code, tests, and examples are not polluted by workflow bookkeeping. Project-level documentation lives in repo-root [`docs/`](docs/).

Root feature source directories:

- `src/plugin-contracts/`
- `src/secure-bootstrap-transport/`
- `src/kernel-runtime-supervision/`
- `src/control-plane-ops/`
- `src/plugin-authoring-sdk/`
- `src/release-packaging-governance/`

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
go build -buildvcs=false -o .tmp-bin/rpcplugind ./kernel-runtime-supervision/daemon-entrypoint/rpcplugind
go build -buildvcs=false -o .tmp-bin/rpcpluginctl ./control-plane-ops/cli-status-control/rpcpluginctl
go build -buildvcs=false -o .tmp-bin/rpcplugin-echo ./plugin-authoring-sdk/echo-example/rpcplugin-echo
go build -buildvcs=false -o .tmp-bin/rpcplugin-failure ./plugin-authoring-sdk/failure-example/rpcplugin-failure
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
cd src && go test ./kernel-runtime-supervision/manager-lifecycle/kernel/...
```

The public Go plugin SDK lives at:

```go
import plugin "rpc_plugin_system/plugin-authoring-sdk/go-runtime/plugin"
```

That import path is the local Go module path used by this source release. It
is meant for packages built inside this repository checkout; it is not a
fetchable external module path and should not be used with `go get` as a
published SDK dependency.

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
