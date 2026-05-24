# provider-observability.event-schema implementation evidence — 2026-05-23

## Changed files

- `src/internal/eventlog/eventlog.go`
  - Added provider observability top-level event fields: `capability_id`, `operation_id`, `correlation_id`, `status`, `duration_ms`, `error_class`, `degraded_reason`.
- `src/internal/eventlog/read.go`
  - Added filters for capability, operation, and correlation ids.
  - Included provider fields in `FormatText` when present.
- `src/internal/eventlog/provider.go`
  - Added provider component constants, provider event constants, provider status constants.
  - Added `ProviderObservation` and `ProviderEvent`.
  - Added fail-closed validation for plugin runtime identity, required provider ids, event/component/status compatibility, duration, and conservative unsafe `Details` keys/values.
- `src/internal/eventlog/provider_test.go`
  - Added focused positive and negative tests for this slice.

## Tests and verification run

From repo root unless noted:

- `git diff --check`
  - PASS: no output.
- `make doc-check`
  - BLOCKED by pre-existing/unrelated workflow layout failure after this feature's local evidence/review/postmortems directories existed:
  - Output:
    ```text
    scripts/check-protocol-doc-honesty.sh
    scripts/check-workflow-layout.sh
    feature provider-observability.sdk-emission: missing workflow/evidence directory
    make: *** [Makefile:11: doc-check] Error 1
    ```
- `make test`
  - BLOCKED because root `test` depends on `doc-check` and fails at the same unrelated workflow layout check:
    ```text
    scripts/check-protocol-doc-honesty.sh
    scripts/check-workflow-layout.sh
    feature provider-observability.sdk-emission: missing workflow/evidence directory
    make: *** [Makefile:11: doc-check] Error 1
    ```
- `cd src && make test`
  - PASS:
    ```text
    go test ./...
    ok  	rpc_plugin_system/cmd/rpcpluginctl	2.222s
    ok  	rpc_plugin_system/cmd/rpcplugind	2.633s
    ok  	rpc_plugin_system/examples/rpcplugin-echo	0.071s
    ok  	rpc_plugin_system/examples/rpcplugin-failure	0.040s
    ok  	rpc_plugin_system/internal/adminrpc	1.131s
    ok  	rpc_plugin_system/internal/auth	0.004s
    ok  	rpc_plugin_system/internal/cli	0.010s
    ok  	rpc_plugin_system/internal/eventlog	(cached)
    ok  	rpc_plugin_system/internal/kernel	18.670s
    ok  	rpc_plugin_system/internal/providerbundle	0.017s
    ok  	rpc_plugin_system/internal/runtime	0.012s
    ok  	rpc_plugin_system/pkg/plugin	0.018s
    ok  	rpc_plugin_system/test/testpluginapi	0.008s
    ok  	rpc_plugin_system/test/testroot	0.051s
    ```
- `cd src && go vet ./...`
  - PASS: no output.
- `cd src && go test ./internal/eventlog -cover`
  - PASS: `coverage: 92.9% of statements`.

## Coverage statement

Focused tests cover the relevant provider event schema behavior requested for this slice:

- valid provider boundary event construction, JSON serialization, logging, and read-back with new top-level fields;
- valid provider diagnostic event and boundary/diagnostic component distinction;
- rejection of unknown provider event names;
- rejection of unknown statuses;
- rejection of missing/malformed plugin id, missing generation, capability, operation, and correlation ids;
- rejection of negative duration;
- rejection of unsafe detail keys/values including payload, secret/token/API key, authority ref/use-ref, session, and provider-private absolute path markers;
- filtering by capability, operation, and correlation ids;
- text formatting of provider fields.

Known gap: root `make doc-check` and root `make test` cannot pass in this worktree without touching unrelated feature workflow directories (`provider-observability.sdk-emission`). I did not modify unrelated workflow records. Source-level tests and vet pass.

## Risks

- `Details` validation is intentionally conservative. It rejects arbitrary absolute paths and broad marker words such as payload/body/token/session/socket/handle/signer. This is deliberate fail-closed behavior for this slice; later redaction-contract work can narrow it.
- Provider diagnostic reported events accept any bounded provider status except only the separate rejected event is forced to `rejected`; this preserves a useful diagnostic surface without smuggling provider semantics into eventlog.

## Next recommendation

Review the eventlog-only schema surface, then unblock repository-level `doc-check` by adding the required workflow directories for unrelated manifest features in a separate workflow/layout cleanup if those records are intended to be active.
