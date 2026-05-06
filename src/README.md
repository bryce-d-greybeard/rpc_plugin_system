# rpc-plugin-system source root

`src/` mirrors the workflow root-feature / feature topology. Build metadata stays at this source root; implementation, docs, examples, and tests live under feature folders.

Root features:

- `plugin-contracts/`
- `secure-bootstrap-transport/`
- `kernel-runtime-supervision/`
- `control-plane-ops/`
- `plugin-authoring-sdk/`
- `release-packaging-governance/`

Build all packages:

```bash
go build ./...
```

Build command binaries:

```bash
go build -buildvcs=false -o .tmp-bin/rpcplugind ./kernel-runtime-supervision/daemon-entrypoint/rpcplugind
go build -buildvcs=false -o .tmp-bin/rpcpluginctl ./control-plane-ops/cli-status-control/rpcpluginctl
go build -buildvcs=false -o .tmp-bin/rpcplugin-echo ./plugin-authoring-sdk/echo-example/rpcplugin-echo
go build -buildvcs=false -o .tmp-bin/rpcplugin-failure ./plugin-authoring-sdk/failure-example/rpcplugin-failure
```

Run tests:

```bash
go test ./...
```

Kernel-focused tests:

```bash
go test ./kernel-runtime-supervision/manager-lifecycle/kernel/...
```

The public Go plugin SDK lives at:

```go
import plugin "rpc_plugin_system/plugin-authoring-sdk/go-runtime/plugin"
```
