# Compatibility

## Purpose

This document defines compatibility expectations between the kernel and plugin executables.

## Compatibility axes

### API compatibility
Method names, request/response shapes, health values, and capability semantics.

### ABI compatibility
Startup environment, socket behavior, auth artifacts, transport expectations, teardown rules, and generation semantics.

## Hard-fail conditions

The kernel should hard-fail a plugin start when any of these occur:
- plugin id mismatch
- auth verification failure
- generation mismatch on startup registration
- non-executable or invalid plugin path
- incompatible transport/runtime assumptions
- capabilities fetch failure or capability sentinel failure during startup

## Soft-fail / degraded conditions

The kernel may degrade rather than hard-fail when:
- heartbeat becomes unhealthy after successful startup
- optional capabilities are missing
- plugin overload is reported
- restart budget is temporarily exhausted

## Compatibility matrix

| Axis | v1 expectation | Hard-fail when violated | Additive/flexible |
| --- | --- | --- | --- |
| Wire namespace | `TestPlugin.*` | yes | no |
| Required RPC methods | `Auth`, `Capabilities`, `Heartbeat`, `Shutdown` | yes | no |
| Generation semantics | one process and one trusted connection per generation | yes | no |
| Startup env contract | required env vars must be present and parseable | yes | no |
| Bootstrap auth | one-time token proof is required | yes | no |
| Peer credential hardening | Linux `SO_PEERCRED` path supported in v1 | yes on supported Linux path when verification is enabled by the runtime; unsupported platforms are not v1 hardening targets | backend expansion post-v1 |
| Optional capabilities | may be absent | no | yes |
| Optional response fields | safe to ignore when additive | no | yes |
| Logging schema | core event fields stay stable enough for operator tooling | yes for removals or semantic breakage | additive fields allowed |

## Versioning direction

For v1, versioning is still blunt:
- exact documented behavior matters more than hand-wavy compatibility claims
- explicit documented contract changes win over implicit compatibility assumptions
- the current frozen wire namespace remains `TestPlugin.*` even though the public SDK package is `sdk/go/plugin`
- additive capability growth is allowed when it does not break the required contract

Later versions may add:
- feature negotiation
- capability versioning
- optional extension points
- broader platform-specific hardening backends behind stable adapter surfaces

## Compatibility policy

The compatibility policy for v1 and later is:
- required lifecycle semantics are stable
- required startup env names are stable
- required wire method names are stable
- required core response shape/meaning is stable
- published source release layout is stable
- releases after `v1.0.0` should remain backward compatible by default
- optional capability additions are allowed
- optional response field additions are allowed
- breaking required behavior requires an intentional major version bump and doc/test updates

## Post-v1 rule

After `v1.0.0`, the default rule is backward compatibility.

That means:
- do not rename required env vars casually
- do not rename required RPC methods casually
- do not change required field meanings casually
- prefer additive capability growth over contract breakage
- keep older v1 plugins and operators working unless there is a clearly justified major-version break

## Rule for development

Do not change plugin lifecycle semantics casually.

When changing API/ABI behavior:
1. update the spec
2. update the tests
3. update the implementation
4. rerun acceptance coverage
5. update install/release docs if public behavior changed
6. state whether the change is backward compatible
