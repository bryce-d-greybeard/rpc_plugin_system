# plugin package 100% coverage evidence — 2026-05-06

Scope: `plugin-authoring-sdk/go-runtime/plugin` after commit `9279896`.

## Changes

- Added a narrow unexported `registerRPCService` seam around `(*rpc.Server).RegisterName` so the existing `ServeWithConfig` registration failure branch can be tested without changing exported SDK behavior.
- Added a narrow unexported `serveExampleWithConfig` seam used only by `Example_minimal`, allowing the non-terminating serve call to be exercised under test while production/example behavior still delegates to `ServeWithConfig` by default.
- Added tests for:
  - `ServeWithConfig` RPC registration failure wrapping/log path.
  - `Example_minimal` happy path: config load, `NewTemplate`, and serve delegation.
  - `Example_minimal` config-load panic path.
  - `Example_minimal` serve-error panic path.

## Seam list

- `registerRPCService`: package-private variable defaulting to `s.RegisterName(name, rcvr)`. Test override only; preserves public API and normal runtime behavior.
- `serveExampleWithConfig`: package-private variable defaulting to `ServeWithConfig`. Test override only; prevents the example coverage test from entering the normal non-terminating accept loop.

## Verification

- `go test -coverprofile=/tmp/plugin-cover.out ./pkg/plugin`: PASS, package coverage `100.0%`.
- `go tool cover -func=/tmp/plugin-cover.out`: PASS, all functions in `plugin` package report `100.0%`.
- `git diff --check`: PASS.
- `make coverage`: PASS; package summary reports `rpc_plugin_system/pkg/plugin` at `100.0%`; repo total `63.4%`.

## HANDOFF

Plugin package coverage is closed to 100%. No push/PR performed. Remaining non-100 coverage in `make coverage` belongs to other packages and is outside this assignment.
