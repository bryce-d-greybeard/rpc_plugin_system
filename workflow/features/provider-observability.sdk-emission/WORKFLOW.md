# provider-observability.sdk-emission

Canonical state lives in `workflow.toml`.

## Scope

Expose a public SDK path for plugins to emit provider-owned structured diagnostics through the substrate logging model.

## Required implementation contract

- Add SDK helpers under `src/pkg/plugin` for provider-owned diagnostic events.
- SDK output must bind diagnostics to the current plugin id and generation loaded from the startup config.
- Helpers must require operation/capability/correlation/status fields rather than accepting arbitrary unstructured maps as the primary API.
- SDK helpers must redact or reject secret-like, payload-like, raw authority-ref, raw handle, provider-private path, and raw response-body fields by default.
- The SDK must not expose broker admission, authority issuance, Lua execution, or client-visible surface emission.

## Required verification

- SDK unit tests for valid diagnostic emission.
- Negative tests for unsafe field names/values.
- Example or fixture proving a plugin can emit diagnostics without importing internal packages.
- `make doc-check`.
- `make test`.
- `git diff --check`.

## Rollback

Remove the SDK helpers and tests. Event schema from the prior slice may remain if unused and documented.

## Interface notes

This is a public SDK surface. Names and field meanings must be boring, explicit, and backward-compatible once admitted.
