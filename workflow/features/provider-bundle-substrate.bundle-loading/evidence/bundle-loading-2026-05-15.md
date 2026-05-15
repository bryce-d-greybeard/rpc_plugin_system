# Bundle loading evidence - 2026-05-15

## Changed files

- `src/internal/providerbundle/loader.go`
- `src/internal/providerbundle/loader_test.go`

## Summary

Implemented filesystem provider bundle metadata loading for `src/internal/providerbundle`.

The loader accepts plugin identity/generation, expected generation, bundle root, manifest path or filename, observed time, correlation id, and expected Lua asset paths. It returns inert `ProviderBundleMetadata` only:

- computes sha256 digest facts for the manifest and expected Lua assets;
- binds plugin id, plugin generation, observed time, and substrate correlation id;
- parses only minimal manifest identity/schema fields;
- fails closed as `declared_unavailable` or `declared_invalid` with redacted error code/message for malformed load identity, missing root, missing/unreadable manifest, malformed manifest identity, missing/unreadable Lua asset, stale generation, and root escape attempts;
- rejects lexical traversal, absolute paths outside the bundle root, and symlink escape for existing assets;
- does not execute Lua, interpret permissions as authority, emit broker/admitted surfaces, mint refs, or call authority/keyring/network owners.

## Verification

```text
cd src && go test ./internal/providerbundle -cover
ok  	rpc_plugin_system/internal/providerbundle	0.016s	coverage: 90.8% of statements
PASS

make test
scripts/check-protocol-doc-honesty.sh
scripts/check-workflow-layout.sh
make -C src test
go test ./...
ok  	rpc_plugin_system/cmd/rpcpluginctl	(cached)
ok  	rpc_plugin_system/cmd/rpcplugind	(cached)
ok  	rpc_plugin_system/examples/rpcplugin-echo	(cached)
ok  	rpc_plugin_system/examples/rpcplugin-failure	(cached)
ok  	rpc_plugin_system/internal/adminrpc	(cached)
ok  	rpc_plugin_system/internal/auth	(cached)
ok  	rpc_plugin_system/internal/cli	(cached)
ok  	rpc_plugin_system/internal/eventlog	(cached)
ok  	rpc_plugin_system/internal/kernel	(cached)
ok  	rpc_plugin_system/internal/providerbundle	0.013s
ok  	rpc_plugin_system/internal/runtime	(cached)
ok  	rpc_plugin_system/pkg/plugin	(cached)
ok  	rpc_plugin_system/test/testpluginapi	(cached)
ok  	rpc_plugin_system/test/testroot	(cached)
PASS

cd src && go vet ./...
PASS

scripts/check-workflow-layout.sh
PASS

git diff --check
PASS
```

## Coverage

`go test ./internal/providerbundle -cover`: 90.8% statements after orchestrator-added coverage for malformed load identity failing closed before filesystem reads.

Covered behavior includes:

- valid load digest computation and identity/provenance binding;
- missing root, missing manifest, unreadable manifest stand-in, missing asset, unreadable asset stand-in;
- malformed plugin id, non-positive generation, missing observation/correlation, and stale expected generation fail-closed before filesystem reads;
- malformed manifest identity;
- path traversal, absolute outside-root asset, and symlink escape rejection;
- Lua content remains inert and descriptor/permission-looking manifest content is not interpreted.

## Risks / gaps

- Loader is intentionally not wired into kernel lifecycle, admin/status DTOs, RPC transport, or authority-use admission. Those are later slices.
- Manifest parsing is deliberately minimal: schema version plus optional plugin id/generation identity only. Descriptor semantics remain untouched.
- Failure metadata may lack digest fields when the digest cannot be computed; this preserves existing validation rules for valid metadata and keeps failures as closed observation facts, not admitted metadata.

## Next-step recommendation

Accepted by orchestrator after tightening load-identity validation and local verification. The next feature slice can expose admin/core inert metadata surfaces without expanding authority boundaries.
