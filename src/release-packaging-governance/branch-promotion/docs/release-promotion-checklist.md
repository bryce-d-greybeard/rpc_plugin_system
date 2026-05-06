# Release and branch promotion checklist

This is the concrete promotion path for `rpc_plugin_system`.

The branch rule is:
- feature branch -> `dev` -> `testing` -> `main`

Do not skip steps just because the tree feels close.

## Promotion: feature branch -> dev

Required before merge:
- the feature has a clear scope and name
- code, tests, and docs for the slice land together
- `gofmt` has been applied to touched Go files
- `go test ./...` passes locally
- any contract/API/ABI behavior changes are reflected in:
  - `README.md`
  - `docs/plugin-api.md`
  - `docs/plugin-abi.md`
  - `docs/compatibility.md`
  - `docs/plugin-standard-v0.md`
  - `docs/v1.0-checklist.md` when relevant
- internal-only planning/postmortem files are not exposed in the public repo surface

Merge intent:
- `dev` is the integration branch for real work
- `dev` may move, but should stay coherent and testable

## Promotion: dev -> testing

Required before promotion:
- `go test ./...` passes cleanly on `dev`
- the current README/tutorial still works against the real tree
- example plugin and failure plugin still build and run
- multi-plugin host control paths still pass local validation
- hardening tests still pass cleanly
- compatibility docs match implementation reality
- packaging/install guidance is current
- known release blockers are written down explicitly, not hand-waved

Recommended validation set:
- `go test ./...`
- `go test ./kernel-runtime-supervision/manager-lifecycle/kernel/...`
- `make build`
- one manual daemon + ctl smoke run using `.tmp-bin/rpcplugind` and `.tmp-bin/rpcpluginctl`

Promotion intent:
- `testing` is the stabilization branch
- only fixes, doc alignment, packaging, and release-readiness work should land here

## Promotion: testing -> main

Required before promotion:
- v1 checklist categories are honestly satisfied
- release docs describe the shipped behavior plainly
- compatibility policy/matrix is explicit
- installation/build guidance is explicit
- branch history is coherent enough to publish without apology
- no known blocker remains in the v1 path
- final local validation set has been rerun on `testing`

Required final validation set:
- `go test ./...`
- `go test ./kernel-runtime-supervision/manager-lifecycle/kernel/...`
- `make build`
- manual single-plugin smoke run
- manual multi-plugin smoke run
- manual CLI inspection pass for:
  - `status`
  - `plugins`
  - `plugin -plugin-id <id>`
  - `capabilities`
  - `routes`
  - `heartbeat -plugin-id <id>`
  - `echo -plugin-id <id> -message test`
  - `logs`

Promotion intent:
- `main` is the publishable branch
- `main` should describe a real substrate, not a thesis draft

## Tagging `v1.0.0`

Do not tag `v1.0.0` just because `main` exists.

Tag only when:
- `main` has passed the full final validation set
- docs and examples match exactly what shipped
- the branch promotion path was followed honestly
- the project is credible as a standalone plugin substrate another engineer could start from
- the release is source-only and the published source layout is stable

## Source release rule

The release rule is:
- release source only
- release only in a stable format
- do not attach unstable packaging experiments to a stable release
- do not imply binary/distribution support that the project is not ready to sustain

## Release notes minimum

A release note for a promoted version should summarize:
- the stable contract being claimed
- the supported authoring path
- the supported local transport/runtime assumptions
- the supported platform hardening path
- whether the release is backward compatible with prior v1 releases
- known intentional non-goals and post-v1 items
