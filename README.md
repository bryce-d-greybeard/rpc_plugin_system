# rpc-plugin-system

`src/` is the workflow root and the source root. Implementation, examples, tests, and workflow records live in the same root-feature / feature folders. Project-level documentation lives in repo-root [`docs/`](docs/).

Root features / source-workflow roots:

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
- Project coverage posture is tracked in [`src/artifacts/global-coverage-map.md`](src/artifacts/global-coverage-map.md).

Run the current package coverage report:

```bash
make coverage
```

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
