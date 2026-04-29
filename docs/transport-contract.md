# Secure RPC Transport Contract

## Purpose

This document defines the steady-state RPC transport contract that now wraps `TestPlugin.*` traffic after bootstrap completes.

This is transport contract, not application API. The application RPC namespace remains `TestPlugin.*` over Go `net/rpc` with gob payloads. This document describes the framing, key derivation inputs, nonce construction, direction rules, and failure behavior for the secure wrapper around that RPC stream.

## Scope

Applies only after:
- bootstrap record/response exchange succeeded
- both sides derived the shared secret and root key
- both sides performed the immediate post-bootstrap refresh
- a Unix socket connection was accepted/dialed for steady-state RPC

Does not apply to:
- FIFO bootstrap transport
- admin RPC
- raw plugin process startup env loading

## Connection prelude

Before secure framing starts, the manager writes one 8-byte connection identifier to the raw Unix socket stream.

Current wire shape:
- unsigned 64-bit integer
- big-endian
- substrate-generated
- one identifier per wrapped RPC connection attempt

The plugin must read exactly these 8 bytes before creating the secure transport wrapper.

## Base session keys

Before per-connection derivation, both sides already hold refreshed directional session keys from the bootstrap flow:
- substrate send key
- substrate receive key
- plugin send key
- plugin receive key

These refreshed session keys are not used directly for framing. They are used as per-connection derivation inputs.

## Per-connection transport key derivation

Per-connection transport keys are derived with HKDF-SHA256.

Inputs:
- base directional session key
- connection identifier
- direction label string

Current info strings:
- `rpc_plugin_system/transport/kernel-to-plugin/<connection-id>`
- `rpc_plugin_system/transport/plugin-to-kernel/<connection-id>`

Rules:
- manager write path uses label `kernel-to-plugin`
- plugin read path uses label `kernel-to-plugin`
- plugin write path uses label `plugin-to-kernel`
- manager read path uses label `plugin-to-kernel`
- the same base key plus same label plus same connection id must produce the same per-connection transport key on both ends
- changing connection id must produce a different transport key even if the base session key is reused

## Framing

Each wrapped RPC write becomes one sealed frame.

Current frame format:
1. 4-byte ciphertext length prefix, unsigned big-endian
2. ciphertext bytes

Ciphertext contents:
- AEAD-sealed plaintext RPC bytes for that write call
- no extra outer metadata bytes are currently appended beyond the ciphertext itself

Authenticated associated data is still supplied to AEAD.
Current associated-data binding includes:
- transport protocol string/version
- plugin id
- generation id
- session id
- connection id
- direction label

Current limits:
- maximum ciphertext frame size is `16 << 20` bytes
- zero-length frames are invalid

## Cipher suite

Current implementation:
- AES-256-GCM
- 32-byte keys
- 12-byte nonce

If the transport key size is not 32 bytes, wrapper creation must fail closed.

## Nonce construction

Current nonce construction is deterministic per connection and direction:
- 12 bytes total
- first 4 bytes are zero
- last 8 bytes are the unsigned big-endian frame sequence number

Sequence rules:
- each writer starts at sequence `0`
- sequence increments by 1 per sealed frame on that wrapped connection
- each reader starts at sequence `0`
- reader sequence must match the sender sequence exactly for decryption to succeed

Safety rule:
- nonce reuse is forbidden under the same key
- because sequence restarts at zero per wrapped connection, per-connection transport key derivation is required to keep nonce/key pairs unique across reconnects

## Direction rules

This transport is directional and metadata-bound.

Current live mapping:
- manager write key: derived from refreshed substrate send key with `kernel-to-plugin`
- plugin read key: derived from refreshed plugin receive key with `kernel-to-plugin`
- plugin write key: derived from refreshed plugin send key with `plugin-to-kernel`
- manager read key: derived from refreshed substrate receive key with `plugin-to-kernel`

If these direction rules are broken, or if plugin/session/generation metadata does not match the associated-data context expected by the peer, decryption fails and the RPC connection is unusable.

## Rekey trigger policy

Current live policy is intentionally simple.

- both sides perform an immediate post-bootstrap refresh before steady-state trust
- each wrapped connection may carry at most `1 << 32` encrypted frames per direction under one transport-key generation
- each wrapped connection may carry at most `32 GiB` of plaintext per direction under one transport-key generation
- each wrapped connection may live at most `60m` under one transport-key generation
- when any bound is reached, the transport fails closed with `secure transport rekey required: <reason>`
- the current implementation treats that as a reconnect/restart boundary rather than doing in-band seamless rekey negotiation

This is not fancy, but it is explicit and inspectable.

## Failure behavior

The transport must fail closed.

Hard failures include:
- missing or unreadable 8-byte connection prelude
- invalid transport key size
- invalid frame length prefix
- ciphertext larger than max frame size
- AEAD open failure
- age-limit, frame-limit, or byte-limit rekey boundary reached
- write failure on length prefix or ciphertext
- read shortfall while loading prefix or ciphertext

Required behavior on transport failure:
- tear down the affected RPC connection
- poison the current client/connection state
- do not continue using partially failed transport state
- require a fresh plugin start/reconnect path according to manager policy

## Observability expectations

At a minimum, operators should be able to infer:
- bootstrap success or failure
- dial/connect success or failure
- auth success or failure
- whether the RPC connection was poisoned after transport failure

The current implementation does not yet emit frame-level transport diagnostics, and that is intentional for now.

## Compatibility note

Any change to:
- connection prelude shape
- HKDF info strings
- direction labels
- cipher suite
- nonce construction
- frame length encoding
- failure semantics

is a transport compatibility change and must be documented as such.
