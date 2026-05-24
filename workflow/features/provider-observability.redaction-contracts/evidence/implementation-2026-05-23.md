# provider-observability.redaction-contracts implementation evidence — 2026-05-23

## Changed files

- `src/internal/eventlog/provider.go`
  - Named the provider redaction sentinel as `ProviderRedactedValue`.
  - Exposed the deterministic provider unsafe-text matcher as `UnsafeProviderText` inside `internal/eventlog`.
  - Documented marker normalization so separator variants match consistently.
- `src/internal/eventlog/read.go`
  - Uses the shared provider unsafe-text matcher and redaction sentinel for display redaction.
- `src/internal/eventlog/redaction_contract_test.go`
  - Added deterministic marker-normalization tests.
  - Added cross-surface rejection/redaction contract tests covering durable provider writes and operator display redaction.
- `docs/implementation-spec.md`
  - Clarified deterministic marker normalization, write-time rejection, display-time redaction, and SDK helper requirement.
- `docs/downstream-provider-guidance.md`
  - Added downstream provider diagnostic logging guidance, including allowed safe facts and forbidden raw material.

## Verification

Run from `/tank/development/linus/rpc-plugin-system` unless noted:

```text
git diff --check
make doc-check
make test
cd src && go vet ./...
cd src && go test ./internal/eventlog ./pkg/plugin ./internal/cli ./cmd/rpcpluginctl -cover
```

All passed. Focused coverage:

```text
ok rpc_plugin_system/internal/eventlog coverage: 93.5% of statements
ok rpc_plugin_system/pkg/plugin       coverage: 100.0% of statements
ok rpc_plugin_system/internal/cli      coverage: 98.1% of statements
ok rpc_plugin_system/cmd/rpcpluginctl coverage: 100.0% of statements
```

## Coverage statement

Focused tests cover the relevant redaction-contract behavior for this slice:

- unsafe marker matching is deterministic across separator and case variants;
- safe provider diagnostic facts remain accepted;
- durable provider writes reject unsafe top-level capability/operation/correlation/error/degraded/message text;
- durable provider writes reject unsafe details;
- operator display redaction removes socket/private path, payload, response body, authority ref, bearer/token-like, and unsafe detail material;
- SDK, CLI, and admin-read tests from prior slices continue to exercise the shared helper path.

No accepted coverage gap for `provider-observability.redaction-contracts`.

## Risks

- The matcher is intentionally conservative. False positives should be handled by providers using coarser safe labels, not by weakening the substrate boundary.
- This slice does not add new provider workflow semantics, authority surfaces, broker admission, or client-visible observability surfaces.

## Next recommendation

Orchestrator review accepted this slice. The provider-observability root has no remaining planned child slices.
