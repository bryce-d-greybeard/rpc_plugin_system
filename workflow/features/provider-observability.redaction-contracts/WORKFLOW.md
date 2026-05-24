# provider-observability.redaction-contracts

Canonical state lives in `workflow.toml`.

## Scope

Harden and document redaction/rejection rules for provider observability across schema, SDK emission, and admin read surfaces.

## Required implementation contract

- Centralize or document the denylist/allowlist rules used by provider observability fields.
- Reject or redact raw prompts, raw provider payloads, credentials, tokens, raw authority refs, raw file/process/browser/socket handles, provider-private paths, and raw response bodies by default.
- Preserve safe correlation and diagnosis fields: plugin id, generation, capability, operation, request/correlation id, duration, status, error class, degraded/unavailable reason, digest/ref facts when explicitly non-authoritative.
- Redaction behavior must be deterministic and testable.
- Documentation must tell downstream providers how to log diagnostics without leaking authority material.

## Required verification

- Cross-surface tests for redaction behavior where helper APIs exist.
- Documentation check through `make doc-check`.
- `make test`.
- `git diff --check`.

## Rollback

Revert redaction helper/doc changes while preserving prior admitted event schema and SDK/admin APIs if they remain safe.

## Interface notes

This slice is a hardening pass. It must not add new provider workflow semantics or extra authority surfaces.
