# Admin/Core Provider Bundle Surfaces Evidence — 2026-05-15

## Changed files
- `src/internal/providerbundle/metadata.go`
- `src/internal/providerbundle/metadata_test.go`
- `src/internal/kernel/manager.go`
- `src/internal/kernel/host.go`
- `src/internal/kernel/host_test.go`
- `src/internal/adminrpc/adminrpc_test.go`

## Summary
- Added generation-bound inert provider bundle projection plumbing onto kernel manager/admin `State` as optional `DeclaredProviderBundleMetadata`.
- Added separate kernel `CoreSnapshot`/`CoreState` so core-facing declared metadata is not mixed into admin status DTOs.
- Tightened core DTO to carry only declared metadata identity, plugin id/generation, manifest/Lua digests, schema/status, observation time, and redacted correlation. Host-private paths are not present in the core DTO.
- Added explicit declared metadata `Name` fields and preserved `declared_*` kind names.
- Expanded redaction to catch secret-like and raw-authority-like terms: authority refs, sockets, handles, signers, and sessions.
- Kept no-bundle status behavior unchanged by leaving the optional metadata fields nil.

## Verification
- `cd src && go test ./internal/providerbundle ./internal/adminrpc ./internal/cli ./internal/kernel`
  - PASS: `providerbundle`, `adminrpc`, `cli`, `kernel`
- `cd src && go test ./internal/providerbundle -cover`
  - PASS: coverage `91.0% of statements`
- `make test`
  - PASS: protocol honesty check, workflow layout check, and `go test ./...`
- `cd src && go vet ./...`
  - PASS: no output
- `scripts/check-workflow-layout.sh`
  - PASS: no output
- `git diff --check`
  - PASS: no output

## Coverage notes
- Added admin status coverage for declared bundle metadata presence, declared/inert naming, host-private path redaction, secret-like string redaction, and raw authority material non-exposure.
- Added no-bundle admin status coverage to preserve existing nil/empty behavior.
- Added core snapshot coverage for plugin id/generation, digest/schema/status/correlation facts, declared metadata identity, generation mismatch suppression, and absence of paths/authority-like material.
- Updated providerbundle DTO tests for digest-only core projection and no mutable slice leakage.

## Risks / gaps
- Projection input is currently supplied through host/manager config and is generation-bound; actual bundle loading remains separate substrate behavior.
- Admin path redaction defaults on; an explicit debug escape hatch exists via `DisableProviderBundleAdminPathRedaction`.
- Core snapshot is an internal kernel method only; no broker admission or Lua execution was added.
- Accepted by orchestrator after tightening core correlation redaction and adding stale-generation suppression coverage for admin/core projections.

## Next-step recommendation
- Wire the existing inert filesystem bundle loader into plugin startup/config discovery in a later feature, keeping this projection layer generation-bound and non-authoritative.
