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
- missing required methods/capabilities
- generation mismatch on startup registration
- non-executable or invalid plugin path
- incompatible transport/runtime assumptions

## Soft-fail / degraded conditions

The kernel may degrade rather than hard-fail when:
- heartbeat becomes unhealthy after successful startup
- optional capabilities are missing
- plugin overload is reported
- restart budget is temporarily exhausted

## Versioning direction

For v0.1.0, versioning is simple:
- exact behavior matters more than negotiated compatibility
- explicit documented contract changes win over implicit compatibility assumptions

Later versions may add:
- feature negotiation
- capability versioning
- optional extension points

## Rule for development

Do not change plugin lifecycle semantics casually.

When changing API/ABI behavior:
1. update the spec
2. update the tests
3. update the implementation
4. rerun acceptance coverage
