# Orchestrator review — provider-observability.admin-read — 2026-05-23

## Verdict

Accepted.

## Review notes

Reviewed the admin-read implementation in main. The change stays inside the active feature scope: eventlog read/filter/rendering, CLI log output, and `rpcpluginctl logs` flags for local operator inspection.

The patch adds generation/capability/operation/correlation filters for log reads, preserves provider component/event distinctions in rendered output, and redacts unsafe provider display material before text/JSON output. The socket path redaction blocker found during cold review was fixed before admission: provider `socket_path` values are redacted from display output along with unsafe provider text and details.

No broker admission, authority issuance, provider capability emission, or client-visible observability surface was added.

## Verification

Run from `/tank/development/linus/rpc-plugin-system` after integration:

```text
git diff --check
make doc-check
make test
cd src && go vet ./...
cd src && go test ./internal/eventlog ./internal/cli ./cmd/rpcpluginctl -cover
```

All passed. Focused coverage:

```text
ok rpc_plugin_system/internal/eventlog coverage: 93.2% of statements
ok rpc_plugin_system/internal/cli      coverage: 98.1% of statements
ok rpc_plugin_system/cmd/rpcpluginctl coverage: 100.0% of statements
```

## Coverage

Relevant behavior covered:

- eventlog reads filter by plugin id, generation id, capability id, operation id, and correlation id;
- text rendering keeps provider diagnostic/boundary component and event names visible;
- text rendering redacts unsafe provider socket paths, authority refs, payload markers, response-body markers, bearer/token-like text, provider-private paths, and unsafe details;
- CLI JSON output redacts unsafe provider socket paths and detail material before display;
- `rpcpluginctl logs` accepts generation/capability/operation/correlation filters;
- `rpcpluginctl logs` can select provider diagnostics and preserve safe provider fields;
- `rpcpluginctl logs -format json` does not display raw unsafe socket/authority/payload/secret-like provider material.

No accepted coverage gap for this slice.

## Workflow transition

Marked `provider-observability.admin-read` done and activated `provider-observability.redaction-contracts` as the next provider-observability slice.
