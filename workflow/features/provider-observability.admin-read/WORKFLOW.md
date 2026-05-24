# provider-observability.admin-read

Canonical state lives in `workflow.toml`.

## Scope

Make provider observability inspectable through local operator/admin log surfaces without changing authority semantics.

## Required implementation contract

- Existing log read paths must preserve provider event fields needed for operator diagnosis.
- CLI/admin output may filter provider events by plugin id, generation, capability, operation, and correlation id where practical.
- Output must distinguish kernel boundary events from provider-owned diagnostic events.
- Admin/CLI surfaces must not display raw secrets, payloads, raw authority refs, raw handles, provider-private paths, or response bodies.
- Reading logs must remain local/operator inspection, not broker admission or client-visible provider capability emission.

## Required verification

- Eventlog/admin/CLI tests for provider event reading or filtering introduced by this slice.
- Redaction assertions for rendered output.
- `make doc-check`.
- `make test`.
- `git diff --check`.

## Rollback

Revert admin/CLI read-surface changes. Existing lifecycle log inspection must remain intact.

## Interface notes

This slice may touch `src/internal/eventlog`, `src/internal/adminrpc`, and `src/internal/cli` only as needed for local operator inspection.
