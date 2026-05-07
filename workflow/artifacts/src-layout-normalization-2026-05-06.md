# Source layout normalization — 2026-05-06

## Scope
Normalize `src/` from workflow-shaped feature paths into a conventional Go repository layout while preserving behavior and keeping workflow records under `workflow/`.

## Layout
- `src/cmd/rpcplugind` — daemon entrypoint.
- `src/cmd/rpcpluginctl` — CLI entrypoint.
- `src/internal/{adminrpc,auth,cli,eventlog,kernel,runtime}` — private host/runtime/control-plane implementation packages.
- `src/pkg/plugin` — public Go plugin SDK package.
- `src/examples/{rpcplugin-echo,rpcplugin-failure}` — example plugins.
- `src/test/{testpluginapi,testroot}` — shared test fixtures/helpers.

## Workflow mapping
Feature IDs and workflow records remain unchanged. `workflow.toml` `source_path` entries now point at the classic source/docs locations instead of workflow-shaped source directories.

## Verification
Passed:
- `make doc-check`
- `make test`
- `make build`
- `make coverage`
- `git diff --check`
- `scripts/check-workflow-layout.sh`
- `(cd src && go list ./...)`
- stale old workflow-shaped path/import grep after coverage regeneration

Coverage after regeneration: `total: (statements) 100.0%`.

## Notes
No push was performed. Local commit is expected after reviewable diff inspection.
