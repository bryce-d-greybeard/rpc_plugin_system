# Orchestrator review — provider-observability.redaction-contracts — 2026-05-23

## Verdict

Accepted.

## Review notes

Reviewed the redaction-contract implementation in main. The change stays inside the active feature scope: provider observability redaction/rejection helpers, tests, and documentation.

The patch names the display redaction sentinel, exposes the deterministic internal provider unsafe-text matcher, documents marker normalization, and adds cross-surface contract tests. Durable provider writes through `ProviderEvent` reject unsafe provider top-level text/details. Operator display redacts unsafe provider material, including socket/private paths and unsafe details. Downstream guidance now tells providers to use `Logger.ProviderDiagnostic`, not raw `LogEvent`, for provider observability.

No provider workflow semantics, authority surfaces, broker admission, or client-visible observability surfaces were added.

## Verification

Run from `/tank/development/linus/rpc-plugin-system` after integration:

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

## Coverage

Relevant behavior covered:

- unsafe marker matching is deterministic across separator and case variants;
- safe provider diagnostic facts remain accepted;
- durable provider writes reject unsafe top-level capability/operation/correlation/error/degraded/message text;
- durable provider writes reject unsafe details;
- operator display redacts socket/private path, payload, response body, authority ref, bearer/token-like, and unsafe detail material;
- SDK, CLI, and admin-read tests from earlier slices continue to exercise the shared helper path.

No accepted coverage gap for this slice.

## Workflow transition

Marked `provider-observability.redaction-contracts` done. The provider-observability root has no remaining planned child slices.
