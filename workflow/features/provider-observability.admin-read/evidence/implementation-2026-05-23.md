# provider-observability.admin-read implementation evidence — 2026-05-23

## Changed files

- `src/internal/eventlog/read.go`
  - Added generation id filtering to `eventlog.Filters`.
  - Added operator-display redaction for provider observability events before text rendering.
  - Redacts unsafe provider top-level text, socket paths, and unsafe details from provider boundary/diagnostic events.
- `src/internal/cli/logs.go`
  - Redacts provider observability events before JSON log output.
- `src/cmd/rpcpluginctl/main.go`
  - Added `logs` filters for provider generation, capability, operation, and correlation ids.
- `src/cmd/rpcpluginctl/args.go`
  - Taught command splitting about the new log filter flags.
- Tests under `src/internal/eventlog`, `src/internal/cli`, and `src/cmd/rpcpluginctl` cover provider filtering and redacted rendered output.

## Verification

Run from `/tank/development/linus/rpc-plugin-system` unless noted:

```text
git diff --check
make doc-check
make test
cd src && go vet ./...
cd src && go test ./internal/eventlog ./internal/cli ./cmd/rpcpluginctl -cover
```

All passed. Focused coverage:

```text
ok rpc_plugin_system/internal/eventlog coverage: 93.1% of statements
ok rpc_plugin_system/internal/cli      coverage: 98.1% of statements
ok rpc_plugin_system/cmd/rpcpluginctl coverage: 100.0% of statements
```

## Coverage statement

Focused tests cover the relevant admin-read behavior for this slice:

- eventlog reads can filter by plugin id, generation id, capability id, operation id, and correlation id;
- text rendering distinguishes provider diagnostic events by component/event and includes provider fields when safe;
- provider display redaction removes unsafe socket paths and top-level capability/correlation/error/degraded/message text;
- provider display redaction replaces unsafe details with a redacted marker;
- CLI JSON log output redacts unsafe provider fields/details before display;
- `rpcpluginctl logs` accepts provider generation/capability/operation/correlation filters;
- `rpcpluginctl logs` can select provider diagnostics and preserve safe provider fields;
- `rpcpluginctl logs -format json` does not display raw unsafe provider socket paths, authority/payload/secret-like text, or unsafe details from provider events.

No accepted coverage gap for `provider-observability.admin-read`.

## Risks

- Redaction is display-time only for operator read surfaces. It does not mutate durable logs. This preserves audit evidence while preventing the CLI/admin read path from printing unsafe provider material.
- The admin-read slice uses the existing local log-read command path. It does not add broker admission, authority issuance, provider capability emission, or client-visible observability surfaces.

## Next recommendation

Orchestrator review accepted this slice. Proceed to the next active slice, `provider-observability.redaction-contracts`.
