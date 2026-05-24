# provider-observability.event-schema

Canonical state lives in `workflow.toml`.

## Scope

Define the substrate event schema additions for provider-boundary and provider-owned diagnostics.

## Required implementation contract

- Add explicit event names/types for provider-boundary operations and provider-owned diagnostics.
- Events must carry plugin id, plugin generation, capability identity, operation identity, request/correlation id, status, duration where available, and error/degraded/unavailable class where applicable.
- Events must distinguish substrate-observed boundary facts from provider semantic facts.
- Events must not represent broker admission, permission, authority issuance, client-visible surface emission, or provider workflow meaning.
- Schema additions must be additive to the current JSONL event contract.

## Required verification

- Eventlog unit tests for new event names/fields.
- Negative tests or assertions proving raw payload/secret/authority-handle marker fields are not accepted in the structured event shape where this slice introduces typed helpers.
- `make doc-check`.
- `make test`.
- `git diff --check`.

## Rollback

Revert the event schema additions and this workflow feature. Existing lifecycle/RPC/auth/restart event names must remain unchanged.

## Interface notes

This feature owns only `src/internal/eventlog` schema/types and docs tied to those names. SDK and admin consumption are later slices.
