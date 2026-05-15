# Restart Invalidation Evidence - 2026-05-15

Feature: `provider-bundle-substrate.restart-invalidation`

## Changed files

- `src/internal/kernel/manager.go`
- `src/internal/kernel/host.go`
- `src/internal/kernel/host_test.go`

## Summary

- Kept provider bundle projection bound to plugin id and current authenticated plugin generation.
- Added `ProviderBundleMetadataRefreshConfig` as a bounded inert host/manager config refresh path keyed by plugin generation.
- Preserved fail-closed nil projection when static metadata is stale after restart.
- Preserved no-bundle restart behavior: admin/core projections remain nil before and after restart.
- Added restart tests covering stale invalidation, explicit generation refresh, config clone isolation, redacted/inert admin/core projections, and no-bundle behavior.

## Coverage

Covered in `src/internal/kernel/host_test.go`:

- `TestProviderBundleProjectionRequiresCurrentPluginGeneration`
- `TestProviderBundleRestartInvalidatesStaleStaticMetadata`
- `TestProviderBundleRestartProjectsExplicitGenerationRefreshOnly`
- `TestProviderBundleRestartWithoutBundleMetadataRemainsAbsent`

The tests verify:

- current generation metadata projects on initial start;
- restart/generation increment prevents old-generation metadata from projecting;
- generation-2 metadata projects only when explicitly supplied through inert refresh config;
- admin/core views remain redacted and path-free after restart;
- no-bundle restart behavior remains unchanged;
- host construction clones static and refresh metadata so later caller mutation cannot rewrite projected generation facts.

## Verification

From `/tank/development/linus/rpc-plugin-system-worktrees/features/provider-bundle-substrate.restart-invalidation/src`:

```text
$ go test ./internal/kernel ./internal/adminrpc ./internal/providerbundle
ok  	rpc_plugin_system/internal/kernel	18.397s
ok  	rpc_plugin_system/internal/adminrpc	1.155s
ok  	rpc_plugin_system/internal/providerbundle	(cached)
```

From `/tank/development/linus/rpc-plugin-system-worktrees/features/provider-bundle-substrate.restart-invalidation`:

```text
$ make test
scripts/check-protocol-doc-honesty.sh
scripts/check-workflow-layout.sh
make -C src test
make[1]: Entering directory '/tank/development/linus/rpc-plugin-system-worktrees/features/provider-bundle-substrate.restart-invalidation/src'
go test ./...
ok  	rpc_plugin_system/cmd/rpcpluginctl	(cached)
ok  	rpc_plugin_system/cmd/rpcplugind	2.814s
ok  	rpc_plugin_system/examples/rpcplugin-echo	(cached)
ok  	rpc_plugin_system/examples/rpcplugin-failure	(cached)
ok  	rpc_plugin_system/internal/adminrpc	1.116s
ok  	rpc_plugin_system/internal/auth	(cached)
ok  	rpc_plugin_system/internal/cli	(cached)
ok  	rpc_plugin_system/internal/eventlog	(cached)
ok  	rpc_plugin_system/internal/kernel	18.346s
ok  	rpc_plugin_system/internal/providerbundle	(cached)
ok  	rpc_plugin_system/internal/runtime	(cached)
ok  	rpc_plugin_system/pkg/plugin	(cached)
ok  	rpc_plugin_system/test/testpluginapi	(cached)
ok  	rpc_plugin_system/test/testroot	(cached)
make[1]: Leaving directory '/tank/development/linus/rpc-plugin-system-worktrees/features/provider-bundle-substrate.restart-invalidation/src'
```

From `/tank/development/linus/rpc-plugin-system-worktrees/features/provider-bundle-substrate.restart-invalidation/src`:

```text
$ go vet ./...
# no output; success
```

From `/tank/development/linus/rpc-plugin-system-worktrees/features/provider-bundle-substrate.restart-invalidation`:

```text
$ scripts/check-workflow-layout.sh
# no output; success

$ git diff --check
# no output; success
```

## Risks / gaps

- Refresh is intentionally config-supplied inert metadata only. There is no filesystem reload on restart in this patch.
- The refresh config is not a broker admission path and does not execute Lua or contact authority owners.
- Invalid or wrong-generation refresh entries fail closed by not projecting unless plugin id and generation match the current state.
- Accepted by orchestrator after adding config clone isolation for static/refresh metadata and re-running verification.

## Next-step recommendation

Orchestrator review should focus on whether the config refresh shape is acceptable public API surface for this feature, or whether the project wants only fail-closed stale invalidation until a later filesystem-loader integration feature.
