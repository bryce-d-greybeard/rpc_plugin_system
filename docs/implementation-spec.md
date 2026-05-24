# rpc-plugin-system Implementation Spec

## Scope

This spec defines concrete implementation surfaces for the current executable plugin substrate and the active provider-observability work.

`rpc-plugin-system` implements local executable plugin supervision, generation-scoped trust, Unix socket `net/rpc` transport, provider bundle metadata, append-only JSONL event logs, public Go SDK helpers, and local admin/CLI inspection. It does not implement broker admission, authority-use issuance, provider workflow semantics, Lua execution, credential resolution, or provider-specific payload handling.

## Repository implementation layout

Source lives under `src/`:

- `src/cmd/rpcplugind` — daemon entrypoint.
- `src/cmd/rpcpluginctl` — local control CLI.
- `src/internal/kernel` — host/manager lifecycle, routing, monitoring, restart/failure isolation.
- `src/internal/auth` — one-time bootstrap token helpers.
- `src/internal/runtime` — runtime directories, Unix sockets, Linux peer credential adapter path.
- `src/internal/adminrpc` — local admin RPC surface.
- `src/internal/eventlog` — append-only JSONL event logging and reads.
- `src/internal/providerbundle` — inert provider bundle metadata loading/projection/redaction.
- `src/internal/cli` — operator output formatting.
- `src/pkg/plugin` — public Go plugin authoring SDK.
- `src/examples/*` — example plugins.
- `src/test/*` — shared test plugin API/root helpers.

Workflow state lives outside `src/` under `workflow/` and `workflow.toml`.

## Core substrate runtime contracts

### Plugin runtime identity

A trusted runtime identity is:

```text
plugin id + generation id + authenticated connection + live process instance
```

Implementation requirements:

- plugin id must be path-safe and stable for the supervised plugin;
- generation id must be positive and advance on restart;
- one trusted RPC client belongs to one generation;
- stale generations and poisoned RPC clients must not remain routable;
- restart invalidates cached capability and provider bundle metadata;
- state snapshots must attribute pid, socket, health, capabilities, bundle metadata, and last error to the correct plugin id/generation.

### Startup ABI

The daemon launches plugin executables with only the public substrate startup variables:

```text
RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET
RPC_PLUGIN_SYSTEM_PLUGIN_ID
RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION
RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE
```

Implementation requirements:

- missing or malformed startup variables fail explicitly in SDK config loading;
- general daemon environment must not be inherited as provider config;
- test-only env variables are not public ABI;
- configured plugin executables must be validated before launch according to current runtime hardening rules.

### Transport and auth

The v1 transport is Unix domain socket plus Go `net/rpc`/gob.

Implementation requirements:

- create one auth token per generation;
- write the token to the generation-scoped auth token file;
- require `Auth` before trusting non-auth RPC methods;
- reject token mismatch/replay;
- remove token files after successful bootstrap;
- verify Linux peer credentials on supported Linux paths;
- hard-fail plugin id/generation mismatch;
- poison the RPC client on timeout/transport break when required by the manager contract.

### Required plugin API

Every standard plugin exposes:

```text
Auth
Capabilities
Heartbeat
Shutdown
```

Implementation requirements:

- responses must carry expected plugin id and generation;
- capabilities are refreshed after restart;
- heartbeat drives visible health/degraded/unhealthy state;
- shutdown is graceful best effort and does not preserve generation trust;
- optional methods are capability-advertised and additive.

## Admin/control-plane implementation

Admin RPC methods are local operator/control-plane methods over a Unix socket:

```text
Admin.Status
Admin.Plugins
Admin.Plugin
Admin.Restart
Admin.Capabilities
Admin.Routes
Admin.Heartbeat
Admin.Echo
```

Implementation requirements:

- admin surfaces return substrate state, not broker admission;
- status/plugin views include generation-scoped plugin state;
- provider bundle metadata appears only as declared inert metadata projections;
- admin redaction must remove secret-like or host-private fields where configured;
- CLI output must preserve operator diagnosis while not leaking authority material.

## Eventlog implementation

### Existing event shape

`src/internal/eventlog.Event` is the durable JSONL shape:

```go
type Event struct {
    Time         time.Time      `json:"time"`
    Level        string         `json:"level"`
    Component    string         `json:"component"`
    Event        string         `json:"event"`
    PluginID     string         `json:"plugin_id,omitempty"`
    GenerationID uint64         `json:"generation_id,omitempty"`
    PID          int            `json:"pid,omitempty"`
    SocketPath   string         `json:"socket_path,omitempty"`
    Method       string         `json:"method,omitempty"`
    Message      string         `json:"message,omitempty"`
    Error        string         `json:"error,omitempty"`
    Reason       string         `json:"reason,omitempty"`
    Details      map[string]any `json:"details,omitempty"`
}
```

Existing components:

```text
kernel
rpc
auth
runtime
```

Existing lifecycle/RPC/auth/restart/cleanup event names must remain backward-compatible. Removals or semantic changes are compatibility breaks.

### Provider observability event schema

The `provider-observability.event-schema` slice implements this schema. Later provider-observability slices build on these additive eventlog fields and helpers.

Add provider observability as additive eventlog schema, not as a replacement for lifecycle logs.

Required new component names:

```text
provider_boundary
provider_diagnostic
```

Required new event names:

```text
provider_operation_started
provider_operation_succeeded
provider_operation_failed
provider_operation_degraded
provider_operation_unavailable
provider_diagnostic_reported
provider_diagnostic_rejected
```

The names intentionally distinguish substrate-observed provider-boundary operation facts from provider-owned semantic diagnostics.

### Provider event typed fields

Add typed top-level fields to `eventlog.Event` rather than hiding everything in `Details`:

```go
CapabilityID  string `json:"capability_id,omitempty"`
OperationID   string `json:"operation_id,omitempty"`
CorrelationID string `json:"correlation_id,omitempty"`
Status        string `json:"status,omitempty"`
DurationMS    int64  `json:"duration_ms,omitempty"`
ErrorClass    string `json:"error_class,omitempty"`
DegradedReason string `json:"degraded_reason,omitempty"`
```

Rules:

- `PluginID` and `GenerationID` remain the plugin runtime identity fields.
- `CapabilityID` is the advertised capability family/name, not permission.
- `OperationID` is the provider operation identity, not broker emission.
- `CorrelationID` is an audit/log correlation fact, not an authority ref.
- `Status` is a bounded diagnostic status, not lifecycle health by itself.
- `DurationMS` is non-negative and optional.
- `ErrorClass` and `DegradedReason` are coarse classes/reasons, not raw upstream responses.
- `Details` remains allowed only for explicitly safe additive facts after redaction/rejection.

Required status vocabulary for provider observability:

```text
started
succeeded
failed
degraded
unavailable
rejected
```

### Provider event validation/redaction helpers

Add internal helpers under `src/internal/eventlog` for provider observability event construction/validation.

Minimum shapes:

```go
type ProviderObservation struct {
    Level          string
    Component      string
    Event          string
    PluginID       string
    GenerationID   uint64
    CapabilityID   string
    OperationID    string
    CorrelationID  string
    Status         string
    DurationMS     int64
    ErrorClass     string
    DegradedReason string
    Message        string
    Details        map[string]any
}

func ProviderEvent(obs ProviderObservation) (Event, error)
```

`ProviderEvent` must reject malformed identity, unknown provider event names, unknown statuses, missing capability/operation/correlation fields where required, negative duration, unsafe top-level provider field text, and unsafe detail keys/values.

Unsafe detail keys or values include secret-like, payload-like, raw authority-ref, raw handle, raw credential, raw token, raw prompt, raw response body, raw socket, raw session, raw signer, and provider-private path material.

Use allowlisted structured top-level fields for normal diagnostics. Do not encourage arbitrary `Details` maps.

### Event read/filter additions

Provider observability fields must be readable without custom parsers.

Extend `eventlog.Filters` as needed:

```go
CapabilityID  string
OperationID   string
CorrelationID string
```

`ReadAll`, `match`, summaries, JSON output, and text formatting must preserve the fields. Text formatting may be concise but must include capability, operation, correlation, status, and error/degraded reason when present.

## Plugin SDK logging implementation

Current SDK logging lives in `src/pkg/plugin/logging.go` and wraps `eventlog.Event` through `Logger.Event`.

Provider observability SDK slice: `provider-observability.sdk-emission`.

Add public SDK helpers after the internal event schema exists.

Minimum public shapes:

```go
type ProviderDiagnostic struct {
    CapabilityID   string
    OperationID    string
    CorrelationID  string
    Status         string
    DurationMS     int64
    ErrorClass     string
    DegradedReason string
    Message        string
    Details        map[string]any
}

func (l *Logger) ProviderDiagnostic(d ProviderDiagnostic) error
```

Implementation requirements:

- bind `PluginID`, `GenerationID`, and `PID` from the SDK logger/config;
- use the shared internal provider event validation/redaction path;
- default component/event to provider diagnostic values;
- reject unsafe fields before writing durable JSONL;
- return validation errors to plugin code instead of silently writing unsafe events;
- keep `Logger.Event` backward-compatible.

The SDK must not expose broker admission, authority issuance, Lua execution, or client-visible surface emission.

## Provider bundle metadata implementation

Provider bundle metadata support remains a substrate feature and is not replaced by provider observability.

Required structs under `src/internal/providerbundle`:

```text
ProviderBundleMetadata
ProviderBundleAsset
ProviderBundleDigest
ProviderBundleStatus
DeclaredProviderBundleCoreDTO
ProviderBundleAdminDTO
ProviderBundleLoadOptions
```

Minimum metadata fields:

- plugin id;
- plugin generation;
- bundle root path;
- manifest path;
- Lua asset paths;
- manifest digest;
- per-asset digest;
- schema version;
- validation status;
- redacted error code/message;
- observed time;
- substrate event correlation id.

Required behavior:

- metadata is collected only for the authenticated plugin generation;
- restart invalidates prior metadata;
- metadata APIs return inert facts only;
- core-facing DTOs omit host-private paths and say `declared_provider_bundle_metadata`;
- admin DTOs redact host-private or secret-like fields when requested;
- malformed load identity must fail before filesystem reads;
- stale generation must fail before filesystem reads;
- bundle path resolution must reject traversal and symlink escape;
- manifest and Lua assets are read for digest/provenance only;
- Lua is never executed;
- descriptor permissions are never interpreted;
- authority owners are never called.

## Redaction contract

Redaction/rejection applies across provider bundle metadata, provider observability, SDK diagnostics, admin output, and CLI output.

Reject or redact by default:

- raw prompts;
- raw provider payloads;
- raw upstream response bodies;
- credentials;
- tokens;
- bearer strings;
- API keys;
- private keys;
- passwords;
- raw authority refs or authority-use refs;
- raw file/process/browser/socket/session handles;
- provider-private absolute paths;
- signer material;
- reusable external handles.

Allow safe diagnostic facts:

- plugin id;
- generation id;
- pid;
- capability id;
- operation id;
- correlation id when not secret-like;
- duration;
- bounded status;
- coarse error class;
- degraded/unavailable reason;
- digest algorithm/value where explicitly non-authoritative;
- counts/sizes when they do not reveal payload content.

Redaction must be deterministic and covered by tests. If a value is ambiguous, fail closed or redact.

## Required tests

### Existing substrate behavior

Continue to cover:

- startup config parsing;
- auth success/failure/replay;
- peer credential verification on supported Linux paths;
- capability fetch;
- heartbeat health/degraded/unhealthy behavior;
- timeout poisoning;
- restart generation advancement;
- stale client/fact rejection;
- runtime artifact cleanup;
- admin RPC state inspection;
- CLI formatting;
- log read/rotation/corrupt-line behavior;
- public SDK example build/use.

### Provider bundle metadata

Required tests:

- valid bundle metadata reports plugin id/generation/digests;
- stale generation metadata is rejected;
- malformed load identity fails before filesystem reads;
- malformed manifest identity fails closed;
- missing/unreadable manifest reports unavailable status;
- missing/unreadable Lua asset reports unavailable status;
- path traversal and symlink escape are rejected;
- plugin restart invalidates previous metadata;
- admin output does not leak secret-like or host-private fields;
- core-facing DTO distinguishes declared metadata from broker-emitted surfaces.

### Provider observability event schema

Required for `provider-observability.event-schema`:

- valid provider boundary events serialize to JSONL with plugin id, generation, capability, operation, correlation, status, duration, and error/degraded fields;
- valid provider diagnostic events serialize separately from boundary events;
- unknown provider event names are rejected;
- unknown statuses are rejected;
- missing plugin id/generation/capability/operation/correlation is rejected where required;
- negative duration is rejected;
- unsafe top-level provider field text and detail keys/values are rejected or redacted according to the helper contract;
- event reads can filter by capability, operation, and correlation once those filters are added;
- text and JSON output preserve safe provider fields.

### SDK provider diagnostics

Required for `provider-observability.sdk-emission`:

- SDK helper binds plugin id/generation/pid from config;
- valid diagnostic writes a provider diagnostic event without importing internal packages;
- unsafe diagnostic fields return errors and do not write events;
- existing `Logger.Event` remains backward-compatible.

### Admin/provider observability reads

Required for `provider-observability.admin-read`:

- admin/CLI log reads preserve provider observability fields;
- filtering by plugin/capability/operation/correlation works where implemented;
- text output distinguishes provider boundary from provider diagnostic events;
- redaction assertions cover rendered output.

## Verification

Default verification before committing substrate spec/code changes:

```sh
git diff --check
make doc-check
make test
```

When implementation code changes under `src/`, also run:

```sh
cd src && go vet ./...
```

There is currently no repo-root `make verify` target. Do not cite it as evidence unless it is added.

## Compatibility notes

Backward-compatible changes:

- additive event fields;
- additive provider event names;
- additive SDK helpers;
- additive CLI filters/output fields that preserve existing output modes;
- stricter rejection of unsafe provider observability details before durable logging.

Compatibility-sensitive changes:

- renaming existing event fields or event names;
- changing startup env names;
- changing required RPC method names;
- changing generation semantics;
- changing auth-before-RPC enforcement;
- weakening redaction or authority-boundary behavior;
- changing existing admin/CLI output in a way that breaks operator workflows without an explicit compatibility decision.
