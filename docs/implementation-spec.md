# rpc-plugin-system Implementation Spec: provider bundle metadata

## Scope

Implement substrate support for provider bundle metadata without changing the authority model.

## Required structs

```text
ProviderBundleMetadata
ProviderBundleAsset
ProviderBundleDigest
ProviderBundleStatus
```

Minimum fields:

- plugin id;
- plugin generation;
- bundle root path;
- manifest path;
- Lua asset paths;
- manifest digest;
- per-asset digest;
- schema version;
- validation status;
- error code/message with redaction;
- observed time;
- substrate event correlation id.

## Required behavior

- Bundle metadata is collected only for the authenticated plugin generation.
- Restart invalidates prior bundle metadata.
- Metadata APIs return inert facts only.
- Admin output redacts host-private paths when configured and never displays secrets.
- Core-facing metadata preserves enough digest/provenance information for core to perform admission.
- The substrate never runs Lua, never interprets descriptor permission, and never calls authority owners.

## Required tests

- valid bundle metadata is reported with plugin id/generation and digests;
- stale generation metadata is rejected;
- malformed manifest reports unavailable status without becoming authority;
- missing Lua asset reports unavailable status;
- plugin restart invalidates previous metadata;
- admin output does not leak raw secret-like fields;
- core-facing DTOs distinguish `declared` bundle metadata from broker-emitted surfaces.

## Verification

```sh
make test
cd src && go vet ./...
scripts/check-workflow-layout.sh
git diff --check
```
