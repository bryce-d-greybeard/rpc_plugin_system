# Orchestrator review — provider-observability.event-schema — 2026-05-23

## Verdict

Accepted.

## Review notes

Reviewed worker changes in the feature worktree and integrated only the eventlog-scoped patch into main. The implementation stays inside the active feature scope: `src/internal/eventlog` plus feature evidence/workflow records.

The patch adds additive provider observability event fields, provider component/event/status constants, `ProviderObservation`, `ProviderEvent`, conservative unsafe detail rejection, event filters for capability/operation/correlation, and text formatting for provider fields.

## Verification

Run from `/tank/development/linus/rpc-plugin-system` after integration:

```text
git diff --check
make doc-check
make test
cd src && go vet ./...
cd src && go test ./internal/eventlog -cover
```

All passed. Eventlog package coverage: 92.9%.

## Coverage

Relevant behavior covered:

- valid boundary event serialization/readback with provider fields;
- valid provider diagnostic event distinction;
- unknown event/status rejection;
- missing plugin id/generation/capability/operation/correlation rejection;
- negative duration rejection;
- unsafe detail key/value rejection for payload/secret/authority/path markers;
- filtering by capability/operation/correlation;
- text formatting of provider fields.

No accepted coverage gap for this slice.

## Scope correction

Worker reported root `doc-check` blocked in its feature worktree because empty workflow evidence/review/postmortem directories are not tracked by Git. Main had local directories, but clone/worktree reproducibility was broken. Added `.gitkeep` files for provider-observability workflow directories so future worktrees pass layout checks.
