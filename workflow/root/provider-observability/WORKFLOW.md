# provider-observability

Canonical state lives in `workflow.toml`.

## Scope

Add substrate-owned provider observability without weakening authority boundaries.

This root covers provider-boundary event schema, provider-owned SDK diagnostic emission, admin/log read surfaces, and redaction contracts. It does not add provider workflow semantics, payload logging, credential exposure, authority refs, Lua execution, or broker admission logic.

## Current sequence

1. `provider-observability.event-schema`
2. `provider-observability.sdk-emission`
3. `provider-observability.admin-read`
4. `provider-observability.redaction-contracts`

## Acceptance

Provider observability is acceptable only if events remain generation-scoped, correlation-based, redacted by default, operator-readable, and clearly distinct from broker-admitted client surfaces.
