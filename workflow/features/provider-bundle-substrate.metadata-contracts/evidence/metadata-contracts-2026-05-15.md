# Provider bundle metadata contracts evidence - 2026-05-15

## Changed files

- `src/internal/providerbundle/metadata.go`
- `src/internal/providerbundle/metadata_test.go`
- `workflow/features/provider-bundle-substrate.metadata-contracts/evidence/metadata-contracts-2026-05-15.md`

## Implementation summary

- Added inert provider bundle metadata primitives:
  - `ProviderBundleMetadata`
  - `ProviderBundleAsset`
  - `ProviderBundleDigest`
  - `ProviderBundleStatus`
- Added declared/core and admin-safe DTO projections:
  - `DeclaredProviderBundleCoreDTO` uses `declared_provider_bundle_metadata` naming to avoid broker/admission confusion.
  - `ProviderBundleAdminDTO` supports requested path redaction and secret-like string redaction.
- Added validation for:
  - missing/malformed plugin id;
  - non-positive plugin generation;
  - optional expected-generation mismatch/stale metadata;
  - missing bundle root path and manifest path;
  - malformed sha256 digests;
  - malformed validation status;
  - missing Lua asset paths and malformed per-asset digests;
  - missing schema version;
  - missing observation/correlation facts;
  - secret-like material in redacted error fields.
- Added slice-copying snapshot/projection helpers so clone/DTO/path projections do not expose mutable internal slices.
- Kept the package inert: no filesystem loading, no Lua execution, no credential/authority owner/network calls, no authority refs, no broker-visible/admitted surfaces.

## Verification

```text
$ cd src && go test ./internal/providerbundle -cover
ok  	rpc_plugin_system/internal/providerbundle	0.007s	coverage: 93.0% of statements

$ make test
scripts/check-protocol-doc-honesty.sh
scripts/check-workflow-layout.sh
make -C src test
make[1]: Entering directory '/tank/development/linus/rpc-plugin-system-worktrees/features/provider-bundle-substrate.metadata-contracts/src'
go test ./...
ok  	rpc_plugin_system/cmd/rpcpluginctl	2.143s
ok  	rpc_plugin_system/cmd/rpcplugind	2.738s
ok  	rpc_plugin_system/examples/rpcplugin-echo	0.068s
ok  	rpc_plugin_system/examples/rpcplugin-failure	0.060s
ok  	rpc_plugin_system/internal/adminrpc	0.979s
ok  	rpc_plugin_system/internal/auth	0.023s
ok  	rpc_plugin_system/internal/cli	0.030s
ok  	rpc_plugin_system/internal/eventlog	0.033s
ok  	rpc_plugin_system/internal/kernel	17.565s
ok  	rpc_plugin_system/internal/providerbundle	0.023s
ok  	rpc_plugin_system/internal/runtime	0.023s
ok  	rpc_plugin_system/pkg/plugin	0.023s
ok  	rpc_plugin_system/test/testpluginapi	0.023s
ok  	rpc_plugin_system/test/testroot	0.063s
make[1]: Leaving directory '/tank/development/linus/rpc-plugin-system-worktrees/features/provider-bundle-substrate.metadata-contracts/src'

$ cd src && go vet ./...
# no output; passed

$ scripts/check-workflow-layout.sh
# no output; passed

$ git diff --check
# no output; passed
```

## Coverage

- Focused package coverage: `93.0%` statements after orchestrator-added coverage for observation/correlation and pre-redacted error fields.
- Relevant issue-surface coverage includes valid metadata, missing plugin id, non-positive generation, missing bundle/manifest identity, malformed digest/status, stale generation mismatch, missing Lua asset path, clone/DTO slice isolation, admin redaction, core DTO declared naming, and relative/secret path redaction behavior.
- Coverage gap: package is intentionally primitives-only. There is no filesystem loader, restart invalidation, kernel lifecycle integration, or admin CLI wiring to cover in this slice.

## Risks / gaps

- Digest validation is intentionally narrow (`sha256`, 64 hex chars). If a later spec admits more algorithms, this contract must change explicitly instead of accepting vague digest facts now.
- `ProviderBundleMetadata` has exported fields for simple DTO-style construction; callers can mutate their own instance. Snapshot/projection helpers copy slices, satisfying the no-leak requirement for clones/DTOs, but this is not an immutable builder API.
- Restart invalidation and generation collection are not implemented here by design; this slice only provides the metadata contract primitives needed by later kernel/admin integration.

## Next-step recommendation

Accepted by orchestrator after tightening validation for observation/correlation and redacted error fields. Proceed to the bundle-loading slice by consuming these primitives without changing their boundary: loader may produce declared metadata, but admission, broker-visible surfaces, authority refs, Lua execution, and credential/network behavior must remain outside this substrate package.
