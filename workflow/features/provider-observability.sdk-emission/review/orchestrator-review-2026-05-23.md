# Orchestrator review — provider-observability.sdk-emission — 2026-05-23

## Verdict

Accepted.

## Review notes

Reviewed the SDK emission implementation in main. The change stays inside the active feature scope: public SDK logging surface under `src/pkg/plugin` plus workflow evidence for `provider-observability.sdk-emission`.

The patch adds a narrow public `ProviderDiagnostic` input type, exported provider status constants, and `(*Logger).ProviderDiagnostic`. The helper binds plugin id, generation id, and pid from the SDK logger, routes through `eventlog.ProviderEvent`, and returns validation errors instead of silently writing unsafe diagnostics. `Logger.Event` remains unchanged for backward compatibility.

The external-package example compiles using only `rpc_plugin_system/pkg/plugin`; it does not import internal packages.

## Verification

Run from `/tank/development/linus/rpc-plugin-system` after integration:

```text
git diff --check
make doc-check
make test
cd src && go vet ./...
cd src && go test ./pkg/plugin -cover
```

All passed. `pkg/plugin` coverage: `100.0%`.

## Coverage

Relevant behavior covered:

- valid SDK provider diagnostic emission and durable JSONL fields;
- logger-bound plugin id, generation id, and pid;
- required capability, operation, correlation, status, duration, error class, degraded reason, message, and safe details;
- nil logger no-op compatibility;
- rejection of missing required fields;
- rejection of unknown status;
- rejection of negative duration;
- rejection of unsafe top-level provider text;
- rejection of unsafe detail keys and values;
- external-package example proving no internal imports are required.

No accepted coverage gap for this slice.

## Workflow transition

Marked `provider-observability.sdk-emission` done and activated `provider-observability.admin-read` as the next provider-observability slice.
