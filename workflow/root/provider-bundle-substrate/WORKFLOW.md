# provider-bundle-substrate

Canonical state lives in `workflow.toml`.

## Scope

Implement provider bundle substrate support for `rpc-plugin-system`: inert discovery metadata, digest/provenance facts, generation binding, admin/core-facing inspection, and restart invalidation.

## Non-goals

- No broker admission in this repo.
- No executable authority-use refs.
- No authority-owner calls.
- No Lua execution.
- No raw secrets, handles, sockets, signers, sessions, or credential material in plugin/provider/admin/core surfaces.
- No protocol-specific authority friendship interfaces.
