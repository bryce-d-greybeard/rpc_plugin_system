# provider-observability.sdk-emission implementation evidence — 2026-05-23

## Changed files

- `src/pkg/plugin/logging.go`
  - Added public `ProviderDiagnostic` SDK input type.
  - Added exported provider status constants backed by the internal eventlog status vocabulary.
  - Added `(*Logger).ProviderDiagnostic`, binding plugin id, generation id, and pid from the SDK logger and routing through `eventlog.ProviderEvent` validation before durable write.
- `src/pkg/plugin/plugin_test.go`
  - Added SDK positive test for structured provider diagnostic emission and durable JSONL fields.
  - Added negative tests for missing required fields, unknown status, negative duration, unsafe top-level provider text, unsafe detail keys, and unsafe detail values.
- `src/pkg/plugin/provider_diagnostic_example_test.go`
  - Added external-package example proving callers can use the public SDK diagnostic surface without importing internal packages.

## Verification

Run from `/tank/development/linus/rpc-plugin-system` unless noted:

```text
git diff --check
make doc-check
make test
cd src && go vet ./...
cd src && go test ./pkg/plugin -cover
```

All passed. `pkg/plugin` coverage: `100.0%`.

## Coverage statement

Focused tests cover the relevant SDK emission behavior for this slice:

- valid diagnostic emission writes `provider_diagnostic` / `provider_diagnostic_reported` events;
- plugin id, generation id, and pid are bound by the SDK logger, not caller-provided diagnostic input;
- capability, operation, correlation, status, duration, error class, degraded reason, message, and safe details survive durable JSONL round-trip;
- nil logger remains backward-compatible and no-op safe;
- missing required provider ids are rejected;
- unknown statuses are rejected;
- negative durations are rejected;
- unsafe top-level provider text is rejected;
- unsafe detail keys and values are rejected;
- external-package example compiles using only `rpc_plugin_system/pkg/plugin`.

No accepted coverage gap for `provider-observability.sdk-emission`.

## Risks

- The SDK helper intentionally exposes only diagnostic reporting, not broker admission, authority issuance, Lua execution, or client-visible surface emission.
- `Logger.Event` remains unchanged for backward compatibility; provider diagnostics should use `ProviderDiagnostic` when callers want the provider-observability validation contract.

## Next recommendation

Orchestrator review accepted this slice. Proceed to the next active slice, `provider-observability.admin-read`.
