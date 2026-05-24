# Provider bundle manifest inert compatibility check

Scope: check whether `rpc-plugin-system` needs changes for the generic provider-bundle manifest schema and semantic digest contract.

Finding:

No substrate mutation is required. `rpc-plugin-system` already stays at the right layer:

- `src/internal/providerbundle/loader.go` computes `sha256` over exact manifest/Lua file bytes.
- `src/internal/providerbundle/metadata.go` documents provider-bundle metadata as inert provenance, not broker-emitted surface, admission result, or permission.
- Tests cover byte digest preservation and malicious-looking descriptor content remaining metadata rather than substrate policy.
- Docs state descriptor admission, Lua execution, client-visible broker surfaces, provider workflow meaning, and authority-use admission belong above substrate, primarily in `agent-core-system` and authority/provider repos.

Conclusion:

Keep `rpc-plugin-system` unchanged for this slice. It should continue reporting byte provenance and authenticated plugin generation only. It must not parse `provider-bundle.v1` semantically, compute core semantic digests, interpret descriptor authority, admit broker surfaces, route authority, or create client-visible capabilities.

Verification:

- inspected `src/internal/providerbundle/loader.go`
- inspected `src/internal/providerbundle/metadata.go`
- inspected substrate architecture/spec/readme boundaries
- `scripts/check-workflow-layout.sh`
- `git diff --check`
